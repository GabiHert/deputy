// Package assistant runs Deputy's Claude chats: the chats in Activity, the New agent chat,
// an agent's "Change with Claude" chat and the board's own chat. Each runs the person's
// claude command with only Deputy's tools, served over MCP from this process. Claude can
// read Deputy's data; anything that changes it is a card the person clicks.
package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/GabiHert/deputy/internal/app"
	"github.com/GabiHert/deputy/internal/board"
	"github.com/GabiHert/deputy/internal/query"
	"github.com/GabiHert/deputy/internal/store"
	"github.com/google/uuid"
)

// Chat purposes.
const (
	PurposeGeneral  = "general"
	PurposeNewAgent = "new-agent"
	PurposeAgent    = "agent"
	PurposeBoard    = "board"
)

// Chat is a saved Claude chat.
type Chat struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Titled    bool      `json:"titled"` // the title was set by the person or from the first message
	Purpose   string    `json:"purpose"`
	AgentID   string    `json:"agentId"`
	SessionID string    `json:"sessionId"` // Claude Code session to resume
	Started   bool      `json:"started"`   // the session exists in Claude Code
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Message is one item in a chat.
type Message struct {
	ID      string       `json:"id"`
	ChatID  string       `json:"chatId"`
	Role    string       `json:"role"` // user | assistant | progress | card | note | error
	Text    string       `json:"text"`
	At      time.Time    `json:"at"`
	Card    *app.CardDTO `json:"card,omitempty"`
	Pending bool         `json:"pending,omitempty"` // a note Claude hasn't seen yet
}

// BoardHooks lets the board chat read and replace the board layout.
type BoardHooks interface {
	Layout() board.Layout
	SetLayout(blocks []app.BoardBlockDTO) error
}

// Deps are the service's collaborators.
type Deps struct {
	Store     *store.Store
	Query     *query.Engine
	Env       func() []string // the login shell environment
	RepoEnv   func() []string // git's environment
	Dir       string          // working folder for Claude runs
	Emit      func(event string, data any)
	OwnerName func() string
	Board     BoardHooks
	// DefaultRepoPath suggests where a new agent's repo goes.
	DefaultRepoPath func(name string) string
	Model           string        // "" = the person's default
	Timeout         time.Duration // per run; default 15 minutes
}

// Service runs the chats.
type Service struct {
	d     Deps
	tools *toolServer

	mu   sync.Mutex
	busy map[string]context.CancelFunc
}

// New starts the tool server.
func New(d Deps) (*Service, error) {
	if d.Timeout == 0 {
		d.Timeout = 15 * time.Minute
	}
	if d.Emit == nil {
		d.Emit = func(string, any) {}
	}
	if d.OwnerName == nil {
		d.OwnerName = func() string { return "" }
	}
	s := &Service{d: d, busy: map[string]context.CancelFunc{}}
	ts, err := startToolServer(s)
	if err != nil {
		return nil, err
	}
	s.tools = ts
	return s, nil
}

// Close stops every run and the tool server.
func (s *Service) Close() {
	s.mu.Lock()
	for _, c := range s.busy {
		c()
	}
	s.mu.Unlock()
	s.tools.close()
}

// ---- storage ----

func (s *Service) chat(id string) (Chat, error) {
	raw, err := s.d.Store.AssistantChat(id)
	if err != nil {
		return Chat{}, errors.New("that chat no longer exists")
	}
	var c Chat
	err = json.Unmarshal(raw, &c)
	return c, err
}

func (s *Service) saveChat(c Chat) error {
	b, _ := json.Marshal(c)
	return s.d.Store.SaveAssistantChat(c.ID, c.UpdatedAt, b)
}

func (s *Service) messages(chatID string) []Message {
	var out []Message
	for _, raw := range s.d.Store.AssistantMessages(chatID) {
		var m Message
		if json.Unmarshal(raw, &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

func (s *Service) message(id string) (Message, error) {
	raw, err := s.d.Store.AssistantMessage(id)
	if err != nil {
		return Message{}, errors.New("that message no longer exists")
	}
	var m Message
	err = json.Unmarshal(raw, &m)
	return m, err
}

func (s *Service) saveMessage(m Message) error {
	b, _ := json.Marshal(m)
	return s.d.Store.SaveAssistantMessage(m.ID, m.ChatID, b)
}

// add appends a message to a chat, touches the chat and tells the frontend.
func (s *Service) add(chatID, role, text string, card *app.CardDTO) Message {
	m := Message{ID: uuid.NewString(), ChatID: chatID, Role: role, Text: text, At: time.Now(), Card: card}
	if role == "note" {
		m.Pending = true
	}
	if err := s.saveMessage(m); err != nil {
		log.Printf("assistant: save message: %v", err)
	}
	if c, err := s.chat(chatID); err == nil {
		c.UpdatedAt = m.At
		_ = s.saveChat(c)
	}
	s.d.Emit(app.EventAssistant, chatID)
	return m
}

// ---- chats ----

// List returns the chats, most recent first. Board and block chats are left out.
func (s *Service) List() []app.AssistantChatDTO {
	out := []app.AssistantChatDTO{}
	for _, raw := range s.d.Store.AssistantChats() {
		var c Chat
		if json.Unmarshal(raw, &c) != nil || c.Purpose == PurposeBoard {
			continue
		}
		out = append(out, s.chatDTO(c))
	}
	return out
}

func (s *Service) chatDTO(c Chat) app.AssistantChatDTO {
	d := app.AssistantChatDTO{ID: c.ID, Title: c.Title, Purpose: c.Purpose, AgentID: c.AgentID,
		CreatedAt: c.CreatedAt.UnixMilli(), UpdatedAt: c.UpdatedAt.UnixMilli(), Busy: s.Busy(c.ID)}
	ms := s.messages(c.ID)
	for i := len(ms) - 1; i >= 0; i-- {
		if ms[i].Role == "assistant" || ms[i].Role == "user" || ms[i].Role == "note" {
			d.Preview = firstLine(ms[i].Text, 80)
			break
		}
		if ms[i].Role == "card" && ms[i].Card != nil {
			d.Preview = cardPreview(*ms[i].Card)
			break
		}
	}
	return d
}

// Get returns one chat.
func (s *Service) Get(id string) (app.AssistantChatDTO, error) {
	c, err := s.chat(id)
	if err != nil {
		return app.AssistantChatDTO{}, err
	}
	return s.chatDTO(c), nil
}

// Create makes a new chat.
func (s *Service) Create(purpose, agentID string) (app.AssistantChatDTO, error) {
	title := "New chat"
	switch purpose {
	case "", PurposeGeneral:
		purpose = PurposeGeneral
	case PurposeNewAgent:
		title = "New agent"
	case PurposeAgent:
		ag, err := s.d.Store.Agent(agentID)
		if err != nil {
			return app.AssistantChatDTO{}, errors.New("that agent no longer exists")
		}
		title = "Changing " + ag.Name
	case PurposeBoard:
		title = "Board"
	default:
		return app.AssistantChatDTO{}, fmt.Errorf("unknown chat purpose %q", purpose)
	}
	now := time.Now()
	c := Chat{ID: uuid.NewString(), Title: title, Purpose: purpose, AgentID: agentID, SessionID: uuid.NewString(),
		CreatedAt: now, UpdatedAt: now}
	if err := s.saveChat(c); err != nil {
		return app.AssistantChatDTO{}, err
	}
	return s.chatDTO(c), nil
}

// BoardChat returns the board's own chat, creating it once.
func (s *Service) BoardChat() string {
	if id, ok := s.d.Store.KV("board.chat"); ok {
		if _, err := s.chat(id); err == nil {
			return id
		}
	}
	c, err := s.Create(PurposeBoard, "")
	if err != nil {
		return ""
	}
	_ = s.d.Store.SetKV("board.chat", c.ID)
	return c.ID
}

// Messages returns a chat's messages for the frontend.
func (s *Service) Messages(chatID string) []app.AssistantMessageDTO {
	out := []app.AssistantMessageDTO{}
	for _, m := range s.messages(chatID) {
		out = append(out, app.AssistantMessageDTO{ID: m.ID, ChatID: m.ChatID, Role: m.Role, Text: m.Text, At: m.At.UnixMilli(), Card: m.Card})
	}
	return out
}

// Rename sets a chat's title.
func (s *Service) Rename(chatID, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("a chat needs a name")
	}
	if len([]rune(title)) > 80 {
		title = string([]rune(title)[:80])
	}
	c, err := s.chat(chatID)
	if err != nil {
		return err
	}
	c.Title, c.Titled = title, true
	if err := s.saveChat(c); err != nil {
		return err
	}
	s.d.Emit(app.EventAssistant, chatID)
	return nil
}

// Delete stops and removes a chat.
func (s *Service) Delete(chatID string) error {
	s.Stop(chatID)
	if err := s.d.Store.DeleteAssistantChat(chatID); err != nil {
		return err
	}
	s.d.Emit(app.EventAssistant, chatID)
	return nil
}

// Busy reports whether Claude is answering in a chat.
func (s *Service) Busy(chatID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.busy[chatID]
	return ok
}

// Stop cancels a chat's run.
func (s *Service) Stop(chatID string) {
	s.mu.Lock()
	cancel := s.busy[chatID]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Send adds the person's message and starts Claude. It returns once the run has started.
func (s *Service) Send(chatID, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("write a message first")
	}
	c, err := s.chat(chatID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.d.Timeout)
	s.mu.Lock()
	if _, ok := s.busy[chatID]; ok {
		s.mu.Unlock()
		cancel()
		return errors.New("Claude is still answering in this chat")
	}
	s.busy[chatID] = cancel
	s.mu.Unlock()

	if !c.Titled && c.Purpose == PurposeGeneral {
		c.Title, c.Titled = firstLine(text, 48), true
		_ = s.saveChat(c)
	}
	notes := s.takeNotes(chatID)
	s.add(chatID, "user", text, nil)
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.busy, chatID)
			s.mu.Unlock()
			cancel()
			s.d.Emit(app.EventAssistant, chatID)
			if c.Purpose == PurposeBoard {
				s.d.Emit(app.EventBoard, nil)
			}
		}()
		if c.Purpose == PurposeBoard {
			s.d.Emit(app.EventBoard, nil)
		}
		s.run(ctx, chatID, composePrompt(notes, text))
	}()
	return nil
}

// takeNotes returns notes Claude hasn't seen (things the person did with cards) and marks them seen.
func (s *Service) takeNotes(chatID string) []string {
	var notes []string
	for _, m := range s.messages(chatID) {
		if m.Role == "note" && m.Pending {
			notes = append(notes, m.Text)
			m.Pending = false
			_ = s.saveMessage(m)
		}
	}
	return notes
}

func composePrompt(notes []string, text string) string {
	if len(notes) == 0 {
		return text
	}
	return "[Deputy, what happened in this chat since your last answer: " + strings.Join(notes, " ") + "]\n\n" + text
}

// ---- cards ----

// Answer records the answers to an ask card and sends them to Claude.
func (s *Service) Answer(messageID string, answers map[string]string) error {
	m, err := s.openCard(messageID, "ask")
	if err != nil {
		return err
	}
	var lines []string
	for _, q := range m.Card.Ask.Questions {
		a := strings.TrimSpace(answers[q.ID])
		if a == "" {
			a = "(no answer, you choose)"
		}
		lines = append(lines, fmt.Sprintf("%s: %s", q.Label, a))
	}
	m.Card.Status, m.Card.Answers, m.Card.Result = "done", answers, "Answered"
	if err := s.saveMessage(m); err != nil {
		return err
	}
	return s.Send(m.ChatID, strings.Join(lines, "\n"))
}

// Dismiss closes a card without acting on it.
func (s *Service) Dismiss(messageID string) error {
	m, err := s.message(messageID)
	if err != nil || m.Card == nil {
		return errors.New("that card no longer exists")
	}
	if m.Card.Status != "open" {
		return nil
	}
	m.Card.Status, m.Card.Result = "dismissed", "Not now"
	if err := s.saveMessage(m); err != nil {
		return err
	}
	s.add(m.ChatID, "note", "Dismissed the "+cardNoun(m.Card.Kind)+".", nil)
	return nil
}

// Complete acts on an open card: act does the work and returns the result line and a note
// for Claude. The card is marked done only if act succeeds.
func (s *Service) Complete(messageID, kind string, act func(c *app.CardDTO) (result, note string, err error)) error {
	m, err := s.openCard(messageID, kind)
	if err != nil {
		return err
	}
	result, note, err := act(m.Card)
	if err != nil {
		return err
	}
	m.Card.Status, m.Card.Result = "done", result
	if err := s.saveMessage(m); err != nil {
		return err
	}
	if note != "" {
		s.add(m.ChatID, "note", note, nil)
	} else {
		s.d.Emit(app.EventAssistant, m.ChatID)
	}
	return nil
}

// RetitleAfterCreate names a New agent chat after the agent it made, unless the person renamed it.
func (s *Service) RetitleAfterCreate(messageID, agentName string) {
	m, err := s.message(messageID)
	if err != nil {
		return
	}
	c, err := s.chat(m.ChatID)
	if err != nil || c.Titled {
		return
	}
	c.Title, c.Titled = "New agent: "+agentName, true
	_ = s.saveChat(c)
	s.d.Emit(app.EventAssistant, c.ID)
}

// Card returns a card message's chat and card.
func (s *Service) Card(messageID string) (string, *app.CardDTO, error) {
	m, err := s.message(messageID)
	if err != nil || m.Card == nil {
		return "", nil, errors.New("that card no longer exists")
	}
	return m.ChatID, m.Card, nil
}

func (s *Service) openCard(messageID, kind string) (Message, error) {
	m, err := s.message(messageID)
	if err != nil || m.Card == nil {
		return Message{}, errors.New("that card no longer exists")
	}
	if m.Card.Kind != kind {
		return Message{}, fmt.Errorf("that card is not a %s", cardNoun(kind))
	}
	if m.Card.Status != "open" {
		return Message{}, errors.New("that card was already used")
	}
	return m, nil
}

func cardNoun(kind string) string {
	switch kind {
	case "ask":
		return "questions"
	case "agent":
		return "agent proposal"
	case "change":
		return "proposed change"
	}
	return "card"
}

func cardPreview(c app.CardDTO) string {
	switch {
	case c.Agent != nil:
		return "Proposed agent " + c.Agent.Name
	case c.Change != nil:
		return "Proposed a change to " + c.Change.AgentName
	case c.Ask != nil:
		return "Asked " + fmt.Sprint(len(c.Ask.Questions)) + " questions"
	}
	return ""
}

func firstLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	r := []rune(strings.TrimSpace(strings.Trim(s, "#*_> ")))
	if len(r) <= max {
		return string(r)
	}
	cut := string(r[:max])
	if i := strings.LastIndexByte(cut, ' '); i > max/2 {
		cut = cut[:i]
	}
	return cut + "…"
}

func ensureDir(dir string) string {
	if dir == "" {
		dir = os.TempDir()
	}
	_ = os.MkdirAll(dir, 0o700)
	return dir
}
