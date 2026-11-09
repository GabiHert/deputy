// Package engine routes WhatsApp messages to agents, runs them and delivers their replies.
package engine

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GabiHert/positron/internal/claude"
	"github.com/GabiHert/positron/internal/domain"
	"github.com/GabiHert/positron/internal/gitrepo"
	"github.com/GabiHert/positron/internal/media"
	"github.com/GabiHert/positron/internal/store"
	"github.com/GabiHert/positron/internal/whatsapp"
	"github.com/google/uuid"
)

// WhatsApp is what the engine needs from the link.
type WhatsApp interface {
	SendText(ctx context.Context, chat, text string) (string, error)
	SendImage(ctx context.Context, chat string, data []byte, mime, caption string) (string, error)
	SendVoice(ctx context.Context, chat string, ogg []byte, seconds int) (string, error)
	Typing(chat string, on, audio bool)
	ChatName(jid string) string
	State() whatsapp.State

	// The owner's own actions from Positron's inbox.
	Send(ctx context.Context, chat string, out whatsapp.Outgoing) (whatsapp.Sent, error)
	React(ctx context.Context, chat, sender, id, emoji string) error
	Revoke(ctx context.Context, chat, id string) error
	EditText(ctx context.Context, chat, id, text string) error
	MarkRead(chat, sender string, ids []string)
}

// Runner runs an agent.
type Runner interface {
	Run(ctx context.Context, r claude.AgentRun) (claude.AgentResult, error)
}

// Deps are the engine's collaborators.
type Deps struct {
	Store       *store.Store
	WA          WhatsApp
	Runner      Runner
	Media       media.Store
	Transcriber media.Transcriber
	Speaker     media.Speaker
	RepoEnv     func() []string
	OnRun       func(domain.Run)         // a run was created or changed
	OnChats     func()                   // the chat list changed
	Notify      func(title, body string) // a reply waits for approval

	// OnMessage reports a stored message: change is "new" for a message that just arrived or was
	// just sent, "update" for a reaction, edit, deletion, status or media change.
	OnMessage func(m domain.Message, change string)
	OnUnread  func(chat string)                              // a chat's unread count changed
	OnReceipt func(chat string, ids []string, status string) // the owner's messages got further
	OnHistory func(kind string, added map[string]int)        // a history sync stored messages, per chat
}

const (
	historySize   = 40
	maxConcurrent = 3
)

type chatState struct {
	pending []string // message IDs waiting for a run
	timer   *time.Timer
	running bool
}

// Engine is the router. Create it with New.
type Engine struct {
	d      Deps
	ctx    context.Context
	cancel context.CancelFunc
	sem    chan struct{}

	mu     sync.Mutex
	chats  map[string]*chatState
	wg     sync.WaitGroup
	histMu sync.Mutex // one history sync batch at a time
}

// New returns an engine. Call Close on shutdown.
func New(d Deps) *Engine {
	ctx, cancel := context.WithCancel(context.Background())
	return &Engine{d: d, ctx: ctx, cancel: cancel, sem: make(chan struct{}, maxConcurrent), chats: map[string]*chatState{}}
}

// Close stops pending timers and cancels running agents.
func (e *Engine) Close() {
	e.mu.Lock()
	for _, c := range e.chats {
		if c.timer != nil {
			c.timer.Stop()
		}
	}
	e.mu.Unlock()
	e.cancel()
	e.wg.Wait()
}

// AgentForChat returns the enabled agent that handles jid.
func (e *Engine) AgentForChat(jid string) (domain.Agent, bool) {
	a, _, ok := domain.NewRouter(e.d.Store.Agents()).Owner(jid)
	return a, ok && a.Enabled
}

// HandleIncoming stores a message and, when it is for an agent, schedules a run.
// It returns quickly; media download and transcription happen in the background.
// Messages from a history sync are only stored: they never start runs.
func (e *Engine) HandleIncoming(in whatsapp.Incoming) {
	m := toMessage(in)
	if in.History {
		e.storeHistory(whatsapp.HistoryChat{JID: in.Chat, Messages: []whatsapp.Incoming{in}})
		return
	}
	if old, err := e.d.Store.Message(m.ChatJID, m.ID); err == nil {
		if old.FromAgent || (old.FromMe && in.FromMe) {
			return // something Positron sent, echoed back
		}
		// The same message again: the phone resending a view-once Positron only had a placeholder
		// for, or a redelivery. Fill in the placeholder; never count or route it twice.
		if old.ViewOnce && len(old.MediaRef) == 0 && len(m.MediaRef) > 0 {
			e.updateMessage(m.ChatJID, m.ID, true, func(s *domain.Message) bool {
				s.Kind, s.Text, s.Mime, s.Seconds, s.FileSize, s.MediaRef = m.Kind, m.Text, m.Mime, m.Seconds, m.FileSize, m.MediaRef
				return true
			})
		}
		return
	}
	_ = e.d.Store.SaveMessage(m)
	e.touchChat(in, true)
	e.message(m, "new")
	e.route(m, in)
}

func toMessage(in whatsapp.Incoming) domain.Message {
	m := domain.Message{
		ID: in.ID, ChatJID: in.Chat, SenderJID: in.Sender, SenderName: in.SenderName, FromMe: in.FromMe, At: in.At,
		Kind: domain.MsgKind(in.Kind), Text: in.Text, Mime: in.Mime, Seconds: in.Seconds,
		MentionsMe: in.MentionsMe || in.QuotesMe, QuotedID: in.QuotedID,
		QuotedSender: in.QuotedSender, QuotedText: in.QuotedText, QuotedKind: domain.MsgKind(in.QuotedKind),
		Mentions: in.Mentions, FileName: in.FileName, FileSize: in.FileSize, MediaRef: in.MediaRef,
		History: in.History, Revoked: in.Revoked, Status: domain.MsgStatus(in.Status), ViewOnce: in.ViewOnce,
	}
	if in.Kind == "document" && m.Text == "" {
		m.Text = in.FileName
	}
	if m.FromMe && m.Status == "" {
		m.Status = domain.StatusSent
	}
	return m
}

func (e *Engine) message(m domain.Message, change string) {
	if e.d.OnMessage != nil {
		e.d.OnMessage(m, change)
	}
}

func (e *Engine) unread(chat string) {
	if e.d.OnUnread != nil {
		e.d.OnUnread(chat)
	}
}

func (e *Engine) chatsChanged() {
	if e.d.OnChats != nil {
		e.d.OnChats()
	}
}

// route hands a new message to its chat's agent, if any.
func (e *Engine) route(m domain.Message, in whatsapp.Incoming) {
	agent, ok := e.AgentForChat(in.Chat)
	if !ok {
		return
	}
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.fetchMedia(&m, in, agent)
		if e.triggers(agent, in) {
			e.schedule(agent, in.Chat, m.ID)
		}
	}()
}

// touchChat brings a chat's preview up to date with a message. A live message from someone else
// adds to the unread count; one from the owner means they have read the chat.
func (e *Engine) touchChat(in whatsapp.Incoming, live bool) {
	old, err := e.d.Store.Chat(in.Chat)
	name := ""
	if err != nil || old.Name == "" || old.Name == "Group" {
		name = e.d.WA.ChatName(in.Chat)
		if !in.IsGroup && !in.FromMe && in.SenderName != "" && strings.HasPrefix(name, "+") {
			name = in.SenderName
		}
	}
	c, _ := e.d.Store.UpdateChat(in.Chat, func(c *domain.Chat) {
		c.JID = in.Chat
		c.IsGroup = c.IsGroup || in.IsGroup
		if name != "" && (c.Name == "" || c.Name == "Group" || strings.HasPrefix(c.Name, "+")) {
			c.Name = name
		}
		if !in.At.Before(c.LastAt) {
			c.LastAt, c.LastText, c.LastID = in.At, preview(in), in.ID
			c.LastBody, c.LastKind, c.LastFromMe = previewBody(in), domain.MsgKind(in.Kind), in.FromMe
			c.LastSender = in.SenderName
			if in.FromMe {
				c.LastSender = ""
			}
		}
		if live {
			if in.FromMe {
				c.Unread = 0
				if in.At.After(c.ReadAt) {
					c.ReadAt = in.At
				}
			} else if !in.Revoked {
				c.Unread++
			}
		}
	})
	e.chatsChanged()
	if live && c.Unread != old.Unread {
		e.unread(in.Chat)
	}
}

func previewBody(in whatsapp.Incoming) string {
	if in.Revoked {
		return "This message was deleted"
	}
	in.IsGroup = false
	return preview(in)
}

func preview(in whatsapp.Incoming) string {
	var p string
	switch in.Kind {
	case "image":
		p = "[photo] "
	case "voice":
		p = fmt.Sprintf("[voice note %d:%02d] ", in.Seconds/60, in.Seconds%60)
	case "audio":
		p = "[audio] "
	case "video":
		p = "[video] "
	case "document":
		p = "[document] " + in.FileName + " "
	case "sticker":
		p = "[sticker]"
	}
	if in.ViewOnce && p != "" {
		p = "[view-once " + strings.TrimPrefix(p, "[")
	}
	text := strings.TrimSpace(p + in.Text)
	if in.IsGroup && in.SenderName != "" {
		who := in.SenderName
		if in.FromMe {
			who = "You"
		}
		text = who + ": " + text
	}
	return text
}

func (e *Engine) fetchMedia(m *domain.Message, in whatsapp.Incoming, a domain.Agent) {
	want := (in.Kind == "image" && a.ReadImages) || ((in.Kind == "voice" || in.Kind == "audio") && a.TranscribeVoice)
	// View-once media is meant to be seen once, by the owner: agents never get a copy.
	if !want || in.Download == nil || in.ViewOnce {
		return
	}
	ctx, cancel := context.WithTimeout(e.ctx, 2*time.Minute)
	defer cancel()
	data, err := in.Download(ctx)
	if err != nil {
		log.Printf("engine: download %s: %v", in.ID, err)
		return
	}
	p, err := e.d.Media.Save(in.Chat, in.ID, in.Mime, data)
	if err != nil {
		log.Printf("engine: save media: %v", err)
		return
	}
	m.MediaPath = p
	if in.Kind == "voice" || in.Kind == "audio" {
		text, err := e.d.Transcriber.Transcribe(ctx, p, a.Language)
		if err != nil {
			log.Printf("engine: transcribe %s: %v", in.ID, err)
			m.Transcript = ""
		} else {
			m.Transcript = text
		}
	}
	// Only these two fields: reactions or receipts may have changed the message meanwhile.
	if saved, ok, _ := e.d.Store.UpdateMessage(m.ChatJID, m.ID, func(s *domain.Message) bool {
		s.MediaPath, s.Transcript = m.MediaPath, m.Transcript
		return true
	}); ok {
		e.message(saved, "update")
	}
}

func (e *Engine) triggers(a domain.Agent, in whatsapp.Incoming) bool {
	if in.Kind == "sticker" {
		return false
	}
	if in.FromMe {
		// Only what the person types on their phone; never what Positron itself sent.
		return a.AnswerMe && !in.FromPositron
	}
	if a.Trigger == domain.TriggerAll || !in.IsGroup {
		return true
	}
	if in.MentionsMe || in.QuotesMe {
		return true
	}
	// A reply to one of the agent's own messages counts as talking to it.
	if in.QuotedID != "" {
		if q, err := e.d.Store.Message(in.Chat, in.QuotedID); err == nil && (q.FromAgent || q.FromMe) {
			return true
		}
	}
	return false
}

func (e *Engine) state(chat string) *chatState {
	c := e.chats[chat]
	if c == nil {
		c = &chatState{}
		e.chats[chat] = c
	}
	return c
}

func (e *Engine) schedule(a domain.Agent, chat, msgID string) {
	wait := time.Duration(max(a.WaitSeconds, 0)) * time.Second
	e.mu.Lock()
	defer e.mu.Unlock()
	c := e.state(chat)
	c.pending = append(c.pending, msgID)
	if c.running {
		return // picked up when the current run ends
	}
	if c.timer != nil {
		c.timer.Stop()
	}
	c.timer = time.AfterFunc(wait, func() { e.fire(chat) })
}

func (e *Engine) fire(chat string) {
	e.mu.Lock()
	c := e.state(chat)
	if c.running || len(c.pending) == 0 || e.ctx.Err() != nil {
		e.mu.Unlock()
		return
	}
	ids := c.pending
	c.pending = nil
	c.running = true
	e.mu.Unlock()

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer func() {
			e.mu.Lock()
			c.running = false
			more := len(c.pending) > 0
			e.mu.Unlock()
			if more {
				e.fire(chat)
			}
		}()
		agent, ok := e.AgentForChat(chat)
		if !ok {
			return
		}
		r := domain.Run{ID: uuid.NewString(), At: time.Now(), AgentID: agent.ID, ChatJID: chat, MessageIDs: ids, Status: domain.RunRunning}
		e.execute(agent, r)
	}()
}

func (e *Engine) save(r domain.Run) {
	_ = e.d.Store.SaveRun(r)
	if e.d.OnRun != nil {
		e.d.OnRun(r)
	}
}

// Retry runs a failed run again over the same messages.
func (e *Engine) Retry(runID string) error {
	r, err := e.d.Store.Run(runID)
	if err != nil {
		return err
	}
	if r.Status == domain.RunRunning {
		return errors.New("this run is still going")
	}
	a, err := e.d.Store.Agent(r.AgentID)
	if err != nil {
		return errors.New("the agent for this run was deleted")
	}
	r.Status, r.Error, r.Reply, r.Note, r.Images, r.Commit, r.Files = domain.RunRunning, "", "", "", nil, "", nil
	r.At = time.Now()
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.mu.Lock()
		c := e.state(r.ChatJID)
		busy := c.running
		c.running = true
		e.mu.Unlock()
		if busy {
			r.Status, r.Error = domain.RunFailed, "The agent is busy in this chat. Try again in a moment."
			e.save(r)
			return
		}
		defer func() {
			e.mu.Lock()
			c.running = false
			e.mu.Unlock()
		}()
		e.execute(a, r)
	}()
	return nil
}

// Request has an agent do what the owner asked, in one chat, and waits for the run to end (or ctx).
// It runs like any other run (same repo, knowledge, history and prompt) plus the owner's request.
// The reply is sent at once whatever the agent's mode and hourly limit, unless review is true:
// then it waits in Activity like a reply to approve. agentID "" means the chat's agent; chatJID
// "" means the agent's only chat.
func (e *Engine) Request(ctx context.Context, agentID, chatJID, request string, review bool) (domain.Run, error) {
	request = strings.TrimSpace(request)
	if request == "" {
		return domain.Run{}, errors.New("say what the agent should do")
	}
	var a domain.Agent
	switch {
	case agentID != "":
		var err error
		if a, err = e.d.Store.Agent(agentID); err != nil {
			return domain.Run{}, errors.New("that agent no longer exists")
		}
	case chatJID != "":
		var ok bool
		if a, _, ok = domain.NewRouter(e.d.Store.Agents()).Owner(chatJID); !ok {
			return domain.Run{}, errors.New("no agent handles this chat; name one with agent")
		}
	default:
		return domain.Run{}, errors.New("name the chat, the agent, or both")
	}
	if chatJID == "" {
		if len(a.Chats) != 1 {
			return domain.Run{}, fmt.Errorf("%s has %d chats; say which one", a.Name, len(a.Chats))
		}
		chatJID = a.Chats[0]
	}
	if e.ctx.Err() != nil {
		return domain.Run{}, errors.New("Positron is quitting")
	}
	r := domain.Run{ID: uuid.NewString(), At: time.Now(), AgentID: a.ID, ChatJID: chatJID, Status: domain.RunRunning,
		Request: request, Review: review}
	done := make(chan domain.Run, 1)
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		c := e.acquire(chatJID)
		defer e.release(chatJID, c)
		done <- e.execute(a, r)
	}()
	select {
	case r := <-done:
		return r, nil
	case <-ctx.Done():
		return r, nil // still running; it ends up in Activity
	}
}

// acquire waits until no run is going in chat, then marks it running.
func (e *Engine) acquire(chat string) *chatState {
	for {
		e.mu.Lock()
		c := e.state(chat)
		if !c.running || e.ctx.Err() != nil {
			c.running = true
			e.mu.Unlock()
			return c
		}
		e.mu.Unlock()
		select {
		case <-time.After(200 * time.Millisecond):
		case <-e.ctx.Done():
		}
	}
}

// release ends a run in chat and starts one for messages that came in meanwhile.
func (e *Engine) release(chat string, c *chatState) {
	e.mu.Lock()
	c.running = false
	more := len(c.pending) > 0
	e.mu.Unlock()
	if more {
		e.fire(chat)
	}
}

// execute runs the agent and saves where the run ended up, which it also returns.
func (e *Engine) execute(a domain.Agent, r domain.Run) domain.Run {
	e.save(r)
	select {
	case e.sem <- struct{}{}:
	case <-e.ctx.Done():
		r.Status, r.Error = domain.RunFailed, "Positron quit while this run was going."
		e.save(r)
		return r
	}
	defer func() { <-e.sem }()

	chat, _ := e.d.Store.Chat(r.ChatJID)
	history := e.d.Store.RecentMessages(r.ChatJID, historySize)
	repo := gitrepo.Repo{Dir: a.RepoPath, EnvFn: e.d.RepoEnv}
	if err := repo.Init(e.ctx); err != nil {
		r.Status, r.Error = domain.RunFailed, "The knowledge repo is not usable: "+err.Error()
		e.save(r)
		return r
	}
	mediaRel := stageMedia(a.RepoPath, history)

	mode := a.Mode
	requested := r.Request != ""
	typing := mode != domain.ModeNever || (requested && !r.Review)
	if typing {
		e.d.WA.Typing(r.ChatJID, true, false)
	}
	push := e.d.WA.State().PushName
	res, err := e.d.Runner.Run(e.ctx, claude.AgentRun{
		Dir: a.RepoPath, SystemPrompt: systemPrompt(a, chat, push), Prompt: prompt(chat, history, r.MessageIDs, mediaRel, push, r.Request),
		Model: a.Model, Web: a.Web, Bash: a.Bash, MCP: a.MCP,
		Folders: foldersOf(a), ReadOnly: a.FolderAccess == "read",
	})
	if typing {
		e.d.WA.Typing(r.ChatJID, false, false)
	}
	if err != nil {
		if e.ctx.Err() != nil {
			r.Status, r.Error = domain.RunFailed, "Positron quit while this run was going."
		} else {
			r.Status, r.Error = domain.RunFailed, err.Error()
		}
		e.save(r)
		return r
	}
	r.Reply, r.Note = res.Reply, res.Note
	r.Voice = res.Voice && a.VoiceReplies
	if a.SendImages {
		r.Images = validImages(a.RepoPath, res.Images)
	}

	msg := fmt.Sprintf("%s: learned from %q", a.Name, chat.Name)
	if sha, files, err := repo.Commit(e.ctx, msg); err != nil {
		log.Printf("engine: commit: %v", err)
	} else if sha != "" {
		r.Commit, r.Files = sha, files
		if a.Push {
			if err := repo.Push(e.ctx); err != nil {
				r.Note = strings.TrimSpace(r.Note + " (Push failed: " + err.Error() + ")")
			}
		}
	}

	// The owner's own request is sent whatever the agent's mode and hourly limit, unless they asked to review it.
	send := false
	switch {
	case mode == domain.ModeNever && !requested:
		r.Status = domain.RunLearned
	case r.Reply == "" && len(r.Images) == 0:
		r.Status = domain.RunQuiet
	case requested:
		r.Status = domain.RunWaiting
		send = !r.Review
	case mode == domain.ModeApprove:
		r.Status = domain.RunWaiting
	case e.overLimit(a):
		r.Status = domain.RunWaiting
		r.Note = strings.TrimSpace(fmt.Sprintf("Held for you: %s already sent %d replies in the last hour. %s", a.Name, a.MaxPerHour, r.Note))
	default:
		send = true
	}
	if send {
		if err := e.deliver(a, &r); err != nil {
			r.Status, r.Error = domain.RunFailed, "The reply could not be sent: "+err.Error()
		}
	}
	e.save(r)
	if r.Status == domain.RunWaiting && e.d.Notify != nil && !requested {
		e.d.Notify(a.Name+" wants to reply in "+chat.Name, r.Reply)
	}
	return r
}

func (e *Engine) overLimit(a domain.Agent) bool {
	if a.MaxPerHour <= 0 {
		return false
	}
	sent := e.d.Store.Runs(store.RunQuery{AgentID: a.ID, Status: domain.RunSent, Since: time.Now().Add(-time.Hour)})
	return len(sent) >= a.MaxPerHour
}

// stageMedia copies the history's photos into the repo's .positron/media so the agent can read them.
// It returns message ID -> repo-relative path.
func stageMedia(repoDir string, history []domain.Message) map[string]string {
	dir := filepath.Join(repoDir, ".positron", "media")
	_ = os.RemoveAll(dir)
	out := map[string]string{}
	for _, m := range history {
		if m.Kind != domain.KindImage || m.MediaPath == "" {
			continue
		}
		b, err := os.ReadFile(m.MediaPath)
		if err != nil {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return out
		}
		name := filepath.Base(m.MediaPath)
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err == nil {
			out[m.ID] = ".positron/media/" + name
		}
	}
	return out
}

var imageExt = map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".webp": "image/webp", ".gif": "image/gif"}

// validImages keeps paths that are image files inside the repo.
func validImages(repoDir string, paths []string) []string {
	var out []string
	root, _ := filepath.Abs(repoDir)
	for _, p := range paths {
		abs := p
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(root, p)
		}
		abs = filepath.Clean(abs)
		rel, err := filepath.Rel(root, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		if _, ok := imageExt[strings.ToLower(filepath.Ext(abs))]; !ok {
			continue
		}
		if st, err := os.Stat(abs); err != nil || st.IsDir() || st.Size() > 16<<20 {
			continue
		}
		out = append(out, filepath.ToSlash(rel))
	}
	return out
}

func withPrefix(prefix, text string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || strings.HasPrefix(text, prefix) {
		return text
	}
	return prefix + " " + text
}

// deliver sends a run's reply and marks it sent.
func (e *Engine) deliver(a domain.Agent, r *domain.Run) error {
	ctx, cancel := context.WithTimeout(e.ctx, 3*time.Minute)
	defer cancel()
	record := func(id string, kind domain.MsgKind, text string) {
		if id == "" {
			return
		}
		m := domain.Message{ID: id, ChatJID: r.ChatJID, SenderName: a.Name, FromMe: true, FromAgent: true,
			At: time.Now(), Kind: kind, Text: text, Status: domain.StatusSent}
		_ = e.d.Store.SaveMessage(m)
		e.touchChat(whatsapp.Incoming{ID: id, Chat: r.ChatJID, IsGroup: !domain.IsDirect(r.ChatJID), SenderName: a.Name,
			FromMe: true, At: m.At, Kind: string(kind), Text: text}, false)
		e.message(m, "new")
	}
	text := r.Reply
	if r.Voice && text != "" {
		e.d.WA.Typing(r.ChatJID, true, true)
		ogg, secs, err := e.d.Speaker.Speak(ctx, text, a.Language)
		e.d.WA.Typing(r.ChatJID, false, true)
		if err == nil {
			id, err := e.d.WA.SendVoice(ctx, r.ChatJID, ogg, secs)
			if err != nil {
				return err
			}
			record(id, domain.KindVoice, text)
			text = ""
		} else {
			r.Voice = false
			r.Note = strings.TrimSpace(r.Note + " (Sent as text: " + err.Error() + ")")
		}
	}
	for i, p := range r.Images {
		data, err := os.ReadFile(filepath.Join(a.RepoPath, filepath.FromSlash(p)))
		if err != nil {
			return err
		}
		caption := ""
		if i == 0 && text != "" {
			caption = withPrefix(a.Prefix, text)
			text = ""
		}
		id, err := e.d.WA.SendImage(ctx, r.ChatJID, data, imageExt[strings.ToLower(filepath.Ext(p))], caption)
		if err != nil {
			return err
		}
		record(id, domain.KindImage, caption)
	}
	if text != "" {
		out := withPrefix(a.Prefix, text)
		id, err := e.d.WA.SendText(ctx, r.ChatJID, out)
		if err != nil {
			return err
		}
		record(id, domain.KindText, out)
	}
	r.Status = domain.RunSent
	r.SentAt = time.Now()
	if e.d.OnChats != nil {
		e.d.OnChats()
	}
	return nil
}

// SendDraft sends a waiting reply, with the person's edits when text is not empty.
func (e *Engine) SendDraft(runID, text string) error {
	r, err := e.d.Store.Run(runID)
	if err != nil {
		return err
	}
	if r.Status != domain.RunWaiting {
		return errors.New("this reply is no longer waiting")
	}
	a, err := e.d.Store.Agent(r.AgentID)
	if err != nil {
		return errors.New("the agent for this reply was deleted")
	}
	if t := strings.TrimSpace(text); t != "" {
		r.Reply = t
	}
	if err := e.deliver(a, &r); err != nil {
		return fmt.Errorf("the reply could not be sent: %w", err)
	}
	e.save(r)
	return nil
}

// SendNow sends text to a chat right away, on the person's say-so. With an agent it goes out like
// one of that agent's replies (its prefix, a sent run in the log); without one it is a plain message.
func (e *Engine) SendNow(agentID, chatJID, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("the message is empty")
	}
	if agentID == "" {
		ctx, cancel := context.WithTimeout(e.ctx, time.Minute)
		defer cancel()
		id, err := e.d.WA.SendText(ctx, chatJID, text)
		if err != nil {
			return fmt.Errorf("the message could not be sent: %w", err)
		}
		if id != "" {
			m := domain.Message{ID: id, ChatJID: chatJID, FromMe: true, At: time.Now(), Kind: domain.KindText, Text: text, Status: domain.StatusSent}
			_ = e.d.Store.SaveMessage(m)
			e.touchChat(whatsapp.Incoming{ID: id, Chat: chatJID, IsGroup: !domain.IsDirect(chatJID), FromMe: true, At: m.At,
				Kind: "text", Text: text}, false)
			e.message(m, "new")
		}
		if e.d.OnChats != nil {
			e.d.OnChats()
		}
		return nil
	}
	a, err := e.d.Store.Agent(agentID)
	if err != nil {
		return errors.New("that agent no longer exists")
	}
	r := domain.Run{ID: uuid.NewString(), At: time.Now(), AgentID: a.ID, ChatJID: chatJID, Reply: text, Note: "Sent from a Positron chat with Claude."}
	if err := e.deliver(a, &r); err != nil {
		return fmt.Errorf("the message could not be sent: %w", err)
	}
	e.save(r)
	return nil
}

// DismissRun clears a run from the owner's "needs you" lists, or brings it back.
func (e *Engine) DismissRun(runID string, dismissed bool) error {
	r, err := e.d.Store.Run(runID)
	if err != nil {
		return errors.New("that run no longer exists")
	}
	if dismissed {
		r.DismissedAt = time.Now()
	} else {
		r.DismissedAt = time.Time{}
	}
	e.save(r)
	return nil
}

// DiscardDraft drops a waiting reply.
func (e *Engine) DiscardDraft(runID string) error {
	r, err := e.d.Store.Run(runID)
	if err != nil {
		return err
	}
	if r.Status != domain.RunWaiting {
		return errors.New("this reply is no longer waiting")
	}
	r.Status = domain.RunDiscarded
	e.save(r)
	return nil
}
