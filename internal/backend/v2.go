package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/GabiHert/positron/internal/app"
	"github.com/GabiHert/positron/internal/assistant"
	"github.com/GabiHert/positron/internal/board"
	"github.com/GabiHert/positron/internal/domain"
	"github.com/GabiHert/positron/internal/gitrepo"
	"github.com/GabiHert/positron/internal/paths"
	"github.com/GabiHert/positron/internal/query"
	"golang.org/x/text/unicode/norm"
)

// v2 holds the board and Claude chat state.
type v2 struct {
	asst       *assistant.Service
	query      *query.Engine
	mu         sync.Mutex
	refreshing map[string]bool
}

// SetV2 connects the Claude chats and the board engine. Call it once at startup.
func (b *Backend) SetV2(a *assistant.Service, q *query.Engine) {
	b.v2 = &v2{asst: a, query: q, refreshing: map[string]bool{}}
}

// QueryEngine builds the board engine over the backend's store.
func (b *Backend) QueryEngine() *query.Engine {
	return &query.Engine{
		Store:   b.d.Store,
		RepoEnv: b.d.Env,
		Runs: func(rs []domain.Run) any {
			agents := b.agentMap()
			out := make([]app.RunDTO, 0, len(rs))
			for _, r := range rs {
				out = append(out, b.runDTO(r, agents))
			}
			return out
		},
		Messages: func(ms []domain.Message) any {
			type msg struct {
				app.MessageDTO
				ChatJID  string `json:"chatJid"`
				ChatName string `json:"chatName"`
			}
			names := map[string]string{}
			out := make([]msg, 0, len(ms))
			for _, m := range ms {
				n, ok := names[m.ChatJID]
				if !ok {
					c, _ := b.d.Store.Chat(m.ChatJID)
					n = c.Name
					names[m.ChatJID] = n
				}
				out = append(out, msg{MessageDTO: messageDTO(m), ChatJID: m.ChatJID, ChatName: n})
			}
			return out
		},
		MediaUsage: func() (int, int64) { files, bytes := b.d.Media.Usage(); return files, bytes },
	}
}

func messageDTO(m domain.Message) app.MessageDTO {
	d := app.MessageDTO{ID: m.ID, At: ms(m.At), SenderName: m.SenderName, FromMe: m.FromMe, Kind: string(m.Kind), Text: m.Text,
		Seconds: m.Seconds, Transcript: m.Transcript, MentionsMe: m.MentionsMe}
	if m.FromAgent {
		d.SenderName = "Agent"
	}
	return d
}

// DefaultRepoPath suggests ~/Positron/agents/<slug> for a new agent, avoiding folders in use.
// With POSITRON_HOME set (a separate Positron, like a test one), agents go under that home instead.
func DefaultRepoPath(name string) string {
	root := filepath.Join(os.Getenv("HOME"), "Positron", "agents")
	if os.Getenv("POSITRON_HOME") != "" {
		root = filepath.Join(paths.Home(), "agents")
	}
	base := filepath.Join(root, slug(name))
	p := base
	for i := 2; ; i++ {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return p
		}
		p = fmt.Sprintf("%s-%d", base, i)
	}
}

func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "agent"
	}
	return out
}

func (b *Backend) ready() error {
	if b.v2 == nil {
		return errors.New("Positron is still starting")
	}
	return nil
}

// ---- board ----

type blockText struct {
	Text  string    `json:"text"`
	At    time.Time `json:"at"`
	Error string    `json:"error"`
}

// Layout returns the saved board (assistant.BoardHooks).
func (b *Backend) Layout() board.Layout {
	raw, _ := b.d.Store.KV("board.layout")
	return board.Parse(raw)
}

// SetLayout saves a board Claude designed (assistant.BoardHooks). Blocks are already validated.
func (b *Backend) SetLayout(blocks []app.BoardBlockDTO) error {
	l := board.Layout{Version: 1, Blocks: blocks, LayoutAt: time.Now()}
	raw, err := json.Marshal(l)
	if err != nil {
		return err
	}
	if err := b.d.Store.SetKV("board.layout", string(raw)); err != nil {
		return err
	}
	b.emit(app.EventBoard, nil)
	// New claude blocks get their first text right away.
	for _, bl := range blocks {
		if bl.Type == "claude" {
			if _, ok := b.blockText(bl.ID); !ok {
				_ = b.RefreshBoardBlock(bl.ID)
			}
		}
	}
	return nil
}

func (b *Backend) blockText(id string) (blockText, bool) {
	raw, ok := b.d.Store.KV("board.text." + id)
	if !ok {
		return blockText{}, false
	}
	var t blockText
	return t, json.Unmarshal([]byte(raw), &t) == nil
}

func (b *Backend) GetBoard() app.BoardDTO {
	l := b.Layout()
	out := app.BoardDTO{Blocks: make([]app.BoardBlockDTO, 0, len(l.Blocks))}
	if !l.LayoutAt.IsZero() {
		out.LayoutAt = l.LayoutAt.UnixMilli()
	}
	if b.v2 != nil {
		out.ChatID = b.v2.asst.BoardChat()
		out.Building = b.v2.asst.Busy(out.ChatID)
	}
	for _, bl := range l.Blocks {
		bl.Data, bl.Agent, bl.Error = nil, nil, ""
		switch {
		case bl.Source != nil && b.v2 != nil:
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			r, err := b.v2.query.Run(ctx, bl.Source.Query, bl.Source.Params)
			cancel()
			if err != nil {
				bl.Error = err.Error()
			} else {
				bl.Data = &app.QueryResultDTO{Query: r.Query, Shape: r.Shape, Items: r.Items}
			}
		case bl.Type == "agent":
			if a, err := b.d.Store.Agent(bl.AgentID); err == nil {
				d := b.toDTO(a, false)
				bl.Agent = &d
			} else {
				bl.Error = "That agent no longer exists."
			}
		case bl.Type == "claude":
			if t, ok := b.blockText(bl.ID); ok {
				bl.Text, bl.TextAt, bl.Error = t.Text, ms(t.At), t.Error
			}
			if b.v2 != nil {
				b.v2.mu.Lock()
				bl.Refreshing = b.v2.refreshing[bl.ID]
				b.v2.mu.Unlock()
			}
		}
		out.Blocks = append(out.Blocks, bl)
	}
	return out
}

func (b *Backend) RunBoardQuery(name string, params map[string]any) (app.QueryResultDTO, error) {
	if err := b.ready(); err != nil {
		return app.QueryResultDTO{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	r, err := b.v2.query.Run(ctx, name, params)
	if err != nil {
		return app.QueryResultDTO{}, err
	}
	return app.QueryResultDTO{Query: r.Query, Shape: r.Shape, Items: r.Items}, nil
}

// RefreshBoardBlock has Claude rewrite a claude block now. It returns at once.
func (b *Backend) RefreshBoardBlock(id string) error {
	if err := b.ready(); err != nil {
		return err
	}
	var block *app.BoardBlockDTO
	for _, bl := range b.Layout().Blocks {
		if bl.ID == id {
			block = &bl
			break
		}
	}
	if block == nil || block.Type != "claude" {
		return errors.New("that block isn't written by Claude")
	}
	b.v2.mu.Lock()
	if b.v2.refreshing[id] {
		b.v2.mu.Unlock()
		return nil
	}
	b.v2.refreshing[id] = true
	b.v2.mu.Unlock()
	b.emit(app.EventBoard, nil)
	go func(bl app.BoardBlockDTO) {
		text, err := b.v2.asst.WriteBlock(context.Background(), bl.ID, bl.Title, bl.Prompt)
		t, _ := b.blockText(bl.ID)
		t.At = time.Now()
		if err != nil {
			log.Printf("board block %s: %v", bl.ID, err)
			t.Error = err.Error()
		} else {
			t.Text, t.Error = text, ""
		}
		raw, _ := json.Marshal(t)
		_ = b.d.Store.SetKV("board.text."+bl.ID, string(raw))
		b.v2.mu.Lock()
		delete(b.v2.refreshing, bl.ID)
		b.v2.mu.Unlock()
		b.emit(app.EventBoard, nil)
	}(*block)
	return nil
}

// RunBoardSchedule rewrites claude blocks when their schedule says so, until ctx ends.
func (b *Backend) RunBoardSchedule(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			for _, bl := range b.Layout().Blocks {
				if bl.Type != "claude" {
					continue
				}
				s, err := board.ParseSchedule(bl.Schedule)
				if err != nil {
					continue
				}
				last, _ := b.blockText(bl.ID)
				if s.Due(last.At, now) {
					_ = b.RefreshBoardBlock(bl.ID)
				}
			}
		}
	}
}

func (b *Backend) ChangeBoard(text string) error {
	if err := b.ready(); err != nil {
		return err
	}
	return b.v2.asst.Send(b.v2.asst.BoardChat(), text)
}

func (b *Backend) ResetBoard() error {
	if err := b.d.Store.DeleteKV("board.layout"); err != nil {
		return err
	}
	b.emit(app.EventBoard, nil)
	return nil
}

// ---- Claude chats ----

func (b *Backend) ListAssistantChats() []app.AssistantChatDTO {
	if b.v2 == nil {
		return []app.AssistantChatDTO{}
	}
	return b.v2.asst.List()
}

// AskAgentChat opens (or reopens) the side chat with jid's agent.
func (b *Backend) AskAgentChat(jid string) (app.AssistantChatDTO, error) {
	if err := b.ready(); err != nil {
		return app.AssistantChatDTO{}, err
	}
	a, _, ok := domain.NewRouter(b.d.Store.Agents()).Owner(jid)
	if !ok {
		return app.AssistantChatDTO{}, errors.New("This chat has no agent. Pick one in the chat's header first.")
	}
	c, err := b.v2.asst.AskChat(a.ID, jid)
	if err == nil {
		b.emit(app.EventAssistant, c.ID)
	}
	return c, err
}

func (b *Backend) NewAssistantChat(purpose, agentID string) (app.AssistantChatDTO, error) {
	if err := b.ready(); err != nil {
		return app.AssistantChatDTO{}, err
	}
	c, err := b.v2.asst.Create(purpose, agentID)
	if err == nil {
		b.emit(app.EventAssistant, c.ID)
	}
	return c, err
}

func (b *Backend) AssistantMessages(chatID string) []app.AssistantMessageDTO {
	if b.v2 == nil {
		return []app.AssistantMessageDTO{}
	}
	return b.v2.asst.Messages(chatID)
}

func (b *Backend) SendAssistant(chatID, text string) error {
	if err := b.ready(); err != nil {
		return err
	}
	return b.v2.asst.Send(chatID, text)
}

func (b *Backend) StopAssistant(chatID string) {
	if b.v2 != nil {
		b.v2.asst.Stop(chatID)
	}
}

func (b *Backend) RenameAssistantChat(chatID, title string) error {
	if err := b.ready(); err != nil {
		return err
	}
	return b.v2.asst.Rename(chatID, title)
}

func (b *Backend) DeleteAssistantChat(chatID string) error {
	if err := b.ready(); err != nil {
		return err
	}
	return b.v2.asst.Delete(chatID)
}

func (b *Backend) AnswerCard(messageID string, answers map[string]string) error {
	if err := b.ready(); err != nil {
		return err
	}
	clean := map[string]string{}
	for k, v := range answers {
		clean[k] = strings.Join(strings.Fields(strings.ReplaceAll(v, "\n", ", ")), " ")
	}
	return b.v2.asst.Answer(messageID, clean)
}

func (b *Backend) DismissCard(messageID string) error {
	if err := b.ready(); err != nil {
		return err
	}
	return b.v2.asst.Dismiss(messageID)
}

// CreateAgentFromCard creates the agent Claude proposed, with the person's edits in p.
func (b *Backend) CreateAgentFromCard(messageID string, p app.AgentProposalDTO) (app.AgentDTO, error) {
	if err := b.ready(); err != nil {
		return app.AgentDTO{}, err
	}
	var created app.AgentDTO
	err := b.v2.asst.Complete(messageID, "agent", func(card *app.CardDTO) (string, string, error) {
		if card.Agent == nil {
			return "", "", errors.New("that card has no agent")
		}
		// Files and chats come from Claude's card; the person may edit the rest.
		p.Knowledge = card.Agent.Knowledge
		if len(p.Chats) == 0 {
			p.Chats = card.Agent.Chats
		}
		if strings.TrimSpace(p.Instructions) == "" {
			p.Instructions = card.Agent.Instructions
		}
		if strings.TrimSpace(p.RepoPath) == "" {
			p.RepoPath = DefaultRepoPath(p.Name)
		}
		owners := map[string]string{}
		for _, ag := range b.d.Store.Agents() {
			for _, c := range ag.Chats {
				owners[c] = ag.Name
			}
		}
		var chats []string
		for _, c := range p.Chats {
			if o := owners[c.JID]; o != "" {
				return "", "", fmt.Errorf("%s is already handled by %s. Remove it there first, or ask Claude for another chat.", c.Name, o)
			}
			chats = append(chats, c.JID)
		}
		d := b.NewAgent()
		d.Name, d.RepoPath, d.Instructions, d.Chats = p.Name, p.RepoPath, p.Instructions, chats
		d.Mode, d.Trigger, d.Language = p.Mode, p.Trigger, p.Language
		if p.MaxPerHour > 0 {
			d.MaxPerHour = p.MaxPerHour
		}
		d.ReadImages, d.TranscribeVoice, d.VoiceReplies, d.SendImages = p.ReadImages, p.TranscribeVoice, p.VoiceReplies, p.SendImages
		saved, err := b.SaveAgent(d)
		if err != nil {
			return "", "", err
		}
		ag, _ := b.d.Store.Agent(saved.ID)
		repo := gitrepo.Repo{Dir: ag.RepoPath, EnvFn: b.d.Env}
		for _, f := range p.Knowledge {
			if err := repo.WriteFile(f.Path, f.Content); err != nil {
				log.Printf("create agent: %v", err)
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		sha, _, _ := repo.Commit(ctx, ag.Name+": starting knowledge from the Positron chat")
		created, _ = b.GetAgent(saved.ID)
		p.CreatedAgentID = saved.ID
		card.Agent = &p
		result := "Created " + ag.Name
		if sha != "" {
			result += " · committed " + sha
		}
		note := fmt.Sprintf("Created %s. Its repo is %s; replies %s.", ag.Name, tilde(ag.RepoPath), modeNote[ag.Mode])
		return result, note, nil
	})
	if err != nil {
		return app.AgentDTO{}, err
	}
	b.v2.asst.RetitleAfterCreate(messageID, created.Name)
	b.emit(app.EventAgents, nil)
	b.emit(app.EventChats, nil)
	b.emit(app.EventBoard, nil)
	return created, nil
}

// SendMessageCard sends the WhatsApp message on a card, with the person's edits when text is not empty.
func (b *Backend) SendMessageCard(messageID, text string) error {
	if err := b.ready(); err != nil {
		return err
	}
	return b.v2.asst.Complete(messageID, "message", func(card *app.CardDTO) (string, string, error) {
		mp := card.Message
		if mp == nil {
			return "", "", errors.New("that card has no message")
		}
		if t := strings.TrimSpace(text); t != "" {
			mp.Text = t
		}
		if err := b.d.Engine.SendNow(mp.AgentID, mp.ChatJID, mp.Text); err != nil {
			return "", "", err
		}
		who := "you"
		if mp.AgentName != "" {
			who = mp.AgentName
		}
		return "Sent to " + mp.ChatName, fmt.Sprintf("Sent to %s as %s: %q", mp.ChatName, who, firstLineOf(mp.Text, 120)), nil
	})
}

func firstLineOf(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i] + "…"
	}
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + "…"
	}
	return s
}

// ApplyChangeCard applies a change Claude proposed to an agent.
func (b *Backend) ApplyChangeCard(messageID string) error {
	if err := b.ready(); err != nil {
		return err
	}
	err := b.v2.asst.Complete(messageID, "change", func(card *app.CardDTO) (string, string, error) {
		ch := card.Change
		if ch == nil {
			return "", "", errors.New("that card has no change")
		}
		ag, err := b.d.Store.Agent(ch.AgentID)
		if err != nil {
			return "", "", errors.New("that agent no longer exists")
		}
		repo := gitrepo.Repo{Dir: ag.RepoPath, EnvFn: b.d.Env}
		if ch.Instructions != "" {
			if strings.TrimSpace(repo.Instructions()) != strings.TrimSpace(ch.OldInstructions) {
				return "", "", errors.New("INSTRUCTIONS.md changed since Claude proposed this. Ask Claude to propose it again.")
			}
			if err := repo.SetInstructions(ch.Instructions); err != nil {
				return "", "", err
			}
		}
		for _, f := range ch.Knowledge {
			if err := repo.WriteFile(f.Path, f.Content); err != nil {
				return "", "", err
			}
		}
		for _, s := range ch.Settings {
			on := s.Value == "true"
			switch s.Field {
			case "mode":
				ag.Mode = domain.ReplyMode(s.Value)
			case "trigger":
				ag.Trigger = domain.Trigger(s.Value)
			case "maxPerHour":
				if n, err := strconv.Atoi(s.Value); err == nil {
					ag.MaxPerHour = n
				}
			case "readImages":
				ag.ReadImages = on
			case "transcribeVoice":
				ag.TranscribeVoice = on
			case "voiceReplies":
				ag.VoiceReplies = on
			case "sendImages":
				ag.SendImages = on
			}
		}
		if len(ch.Settings) > 0 {
			if err := b.d.Store.SaveAgent(ag); err != nil {
				return "", "", err
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		sha, _, err := repo.Commit(ctx, ag.Name+": "+ch.Summary)
		if err != nil {
			return "", "", fmt.Errorf("Could not commit the change: %v", err)
		}
		ch.Commit = sha
		result := "Applied"
		if sha != "" {
			result += " · committed " + sha
		}
		return result, fmt.Sprintf("Applied the change to %s: %s", ag.Name, ch.Summary), nil
	})
	if err != nil {
		return err
	}
	b.emit(app.EventAgents, nil)
	return nil
}

var modeNote = map[domain.ReplyMode]string{
	domain.ModeApprove: "wait for your OK in Activity",
	domain.ModeAuto:    "are sent on their own",
	domain.ModeNever:   "are never sent; it only learns",
}
