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

	"github.com/GabiHert/deputy/internal/claude"
	"github.com/GabiHert/deputy/internal/domain"
	"github.com/GabiHert/deputy/internal/gitrepo"
	"github.com/GabiHert/deputy/internal/media"
	"github.com/GabiHert/deputy/internal/store"
	"github.com/GabiHert/deputy/internal/whatsapp"
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

	mu    sync.Mutex
	chats map[string]*chatState
	wg    sync.WaitGroup
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
	for _, a := range e.d.Store.Agents() {
		if a.HasChat(jid) {
			return a, a.Enabled
		}
	}
	return domain.Agent{}, false
}

// HandleIncoming stores a message and, when it is for an agent, schedules a run.
// It returns quickly; media download and transcription happen in the background.
func (e *Engine) HandleIncoming(in whatsapp.Incoming) {
	m := domain.Message{
		ID: in.ID, ChatJID: in.Chat, SenderJID: in.Sender, SenderName: in.SenderName, FromMe: in.FromMe, At: in.At,
		Kind: domain.MsgKind(in.Kind), Text: in.Text, Mime: in.Mime, Seconds: in.Seconds,
		MentionsMe: in.MentionsMe || in.QuotesMe, QuotedID: in.QuotedID,
	}
	if in.Kind == "document" && m.Text == "" {
		m.Text = in.FileName
	}
	if old, err := e.d.Store.Message(m.ChatJID, m.ID); err == nil && old.FromAgent {
		return // our own reply, echoed back
	}
	_ = e.d.Store.SaveMessage(m)
	e.touchChat(in)

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

func (e *Engine) touchChat(in whatsapp.Incoming) {
	c, err := e.d.Store.Chat(in.Chat)
	if err != nil || c.Name == "" || c.Name == "Group" {
		c.JID = in.Chat
		c.IsGroup = in.IsGroup
		c.Name = e.d.WA.ChatName(in.Chat)
		if !in.IsGroup && in.FromMe == false && in.SenderName != "" && strings.HasPrefix(c.Name, "+") {
			c.Name = in.SenderName
		}
	}
	c.LastText = preview(in)
	if !in.At.Before(c.LastAt) {
		c.LastAt = in.At
	}
	_ = e.d.Store.SaveChat(c)
	if e.d.OnChats != nil {
		e.d.OnChats()
	}
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
	if !want || in.Download == nil {
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
	_ = e.d.Store.SaveMessage(*m)
}

func (e *Engine) triggers(a domain.Agent, in whatsapp.Incoming) bool {
	if in.FromMe || in.Kind == "sticker" {
		return false
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

func (e *Engine) execute(a domain.Agent, r domain.Run) {
	e.save(r)
	select {
	case e.sem <- struct{}{}:
	case <-e.ctx.Done():
		return
	}
	defer func() { <-e.sem }()

	chat, _ := e.d.Store.Chat(r.ChatJID)
	history := e.d.Store.RecentMessages(r.ChatJID, historySize)
	repo := gitrepo.Repo{Dir: a.RepoPath, EnvFn: e.d.RepoEnv}
	if err := repo.Init(e.ctx); err != nil {
		r.Status, r.Error = domain.RunFailed, "The knowledge repo is not usable: "+err.Error()
		e.save(r)
		return
	}
	mediaRel := stageMedia(a.RepoPath, history)

	mode := a.Mode
	if mode != domain.ModeNever {
		e.d.WA.Typing(r.ChatJID, true, false)
	}
	res, err := e.d.Runner.Run(e.ctx, claude.AgentRun{
		Dir: a.RepoPath, SystemPrompt: systemPrompt(a, chat, e.d.WA.State().PushName), Prompt: prompt(chat, history, r.MessageIDs, mediaRel, e.d.WA.State().PushName),
		Model: a.Model, Web: a.Web, Bash: a.Bash,
	})
	if mode != domain.ModeNever {
		e.d.WA.Typing(r.ChatJID, false, false)
	}
	if err != nil {
		if e.ctx.Err() != nil {
			r.Status, r.Error = domain.RunFailed, "Deputy quit while this run was going."
		} else {
			r.Status, r.Error = domain.RunFailed, err.Error()
		}
		e.save(r)
		return
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

	switch {
	case mode == domain.ModeNever:
		r.Status = domain.RunLearned
	case r.Reply == "" && len(r.Images) == 0:
		r.Status = domain.RunQuiet
	case mode == domain.ModeApprove:
		r.Status = domain.RunWaiting
	case e.overLimit(a):
		r.Status = domain.RunWaiting
		r.Note = strings.TrimSpace(fmt.Sprintf("Held for you: %s already sent %d replies in the last hour. %s", a.Name, a.MaxPerHour, r.Note))
	default:
		if err := e.deliver(a, &r); err != nil {
			r.Status, r.Error = domain.RunFailed, "The reply could not be sent: "+err.Error()
		}
	}
	e.save(r)
	if r.Status == domain.RunWaiting && e.d.Notify != nil {
		e.d.Notify(a.Name+" wants to reply in "+chat.Name, r.Reply)
	}
}

func (e *Engine) overLimit(a domain.Agent) bool {
	if a.MaxPerHour <= 0 {
		return false
	}
	sent := e.d.Store.Runs(store.RunQuery{AgentID: a.ID, Status: domain.RunSent, Since: time.Now().Add(-time.Hour)})
	return len(sent) >= a.MaxPerHour
}

// stageMedia copies the history's photos into the repo's .deputy/media so the agent can read them.
// It returns message ID -> repo-relative path.
func stageMedia(repoDir string, history []domain.Message) map[string]string {
	dir := filepath.Join(repoDir, ".deputy", "media")
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
			out[m.ID] = ".deputy/media/" + name
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
		_ = e.d.Store.SaveMessage(domain.Message{ID: id, ChatJID: r.ChatJID, SenderName: a.Name, FromMe: true, FromAgent: true,
			At: time.Now(), Kind: kind, Text: text})
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
