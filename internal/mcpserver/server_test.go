package mcpserver

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/GabiHert/positron/internal/app"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeBackend implements the few calls the tests make; the rest panic through the nil app.Backend.
type fakeBackend struct {
	app.Backend
	chats  []app.ChatDTO
	agents []app.AgentDTO
	sent   []string
	asked  []string
}

func (f *fakeBackend) ListChats() []app.ChatDTO   { return f.chats }
func (f *fakeBackend) ListAgents() []app.AgentDTO { return f.agents }
func (f *fakeBackend) SendNow(agentID, jid, text string) error {
	f.sent = append(f.sent, agentID+"|"+jid+"|"+text)
	return nil
}
func (f *fakeBackend) AskAgent(_ context.Context, agentID, jid, req string, review bool) (app.RunDTO, error) {
	f.asked = append(f.asked, agentID+"|"+jid+"|"+req)
	return app.RunDTO{ID: "r1", AgentID: agentID, AgentName: "Family", ChatJID: jid, Status: "sent", Reply: "Chego 20 min atrasado", Request: req}, nil
}

func (f *fakeBackend) ReplyFromClaude(_ context.Context, chat, quotedID, text string) (app.ChatMessageDTO, error) {
	f.sent = append(f.sent, "reply|"+chat+"|"+quotedID+"|"+text)
	return app.ChatMessageDTO{ID: "R1", ChatJID: chat, FromMe: true, Kind: "text", Text: text}, nil
}
func (f *fakeBackend) SendFileFromClaude(_ context.Context, in app.SendFileDTO) (app.ChatMessageDTO, error) {
	f.sent = append(f.sent, "file|"+in.Chat+"|"+in.Path+"|"+in.Kind)
	return app.ChatMessageDTO{ID: "F1", ChatJID: in.Chat, FromMe: true, Kind: "document"}, nil
}

func newFake() *fakeBackend {
	return &fakeBackend{
		chats: []app.ChatDTO{
			{JID: "111@g.us", Name: "Família", IsGroup: true, AgentID: "a1", AgentName: "Family"},
			{JID: "222@g.us", Name: "Família Herter", IsGroup: true},
			{JID: "5551999994412@s.whatsapp.net", Name: "Ana Souza"},
			{JID: "5551888881111@s.whatsapp.net", Name: "Ana Lima"},
		},
		agents: []app.AgentDTO{{ID: "a1", Name: "Family", Chats: []string{"111@g.us"}, Prefix: "🤖"}, {ID: "a2", Name: "Family work"}},
	}
}

func TestResolveChat(t *testing.T) {
	chats := newFake().chats
	cases := []struct{ q, jid, err string }{
		{"111@g.us", "111@g.us", ""},
		{"família", "111@g.us", ""}, // exact beats the longer name
		{"herter", "222@g.us", ""},
		{"Souza", "5551999994412@s.whatsapp.net", ""},
		{"+55 (51) 99999-4412", "5551999994412@s.whatsapp.net", ""},
		{"5511912345678", "5511912345678@s.whatsapp.net", ""}, // a number Positron hasn't seen
		{"ana", "", "matches 2 chats"},
		{"nobody", "", "no chat matches"},
	}
	for _, c := range cases {
		got, err := resolveChat(chats, c.q)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%q: want error %q, got %v %+v", c.q, c.err, err, got)
			}
			continue
		}
		if err != nil || got.JID != c.jid {
			t.Errorf("%q: got %q, %v", c.q, got.JID, err)
		}
	}
	if _, err := resolveChat(chats, "ana"); !strings.Contains(err.Error(), "Ana Lima (5551888881111@s.whatsapp.net)") {
		t.Errorf("ambiguous error should list candidates: %v", err)
	}
}

func TestResolveAgent(t *testing.T) {
	agents := newFake().agents
	for q, want := range map[string]string{"a2": "a2", "FAMILY": "a1", "work": "a2"} {
		if a, err := resolveAgent(agents, q); err != nil || a.ID != want {
			t.Errorf("%q: got %q, %v", q, a.ID, err)
		}
	}
	if _, err := resolveAgent(agents, "fam"); err == nil || !strings.Contains(err.Error(), "matches 2 agents") {
		t.Errorf("want ambiguous, got %v", err)
	}
}

func testServer(t *testing.T) (*Server, *fakeBackend, *httptest.Server) {
	t.Helper()
	fb := newFake()
	s := New(fb, Options{Home: t.TempDir(), Port: "0"})
	hs := httptest.NewServer(s.Handler())
	t.Cleanup(hs.Close)
	return s, fb, hs
}

func TestAuthAndHostChecks(t *testing.T) {
	s, _, hs := testServer(t)
	tok, err := s.Token()
	if err != nil || len(tok) != 64 {
		t.Fatalf("token %q %v", tok, err)
	}
	if st, _ := os.Stat(filepath.Join(s.opts.Home, "mcp-token")); st.Mode().Perm() != 0o600 {
		t.Fatalf("token file mode %v", st.Mode().Perm())
	}
	body := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	try := func(mut func(r *http.Request)) int {
		r, _ := http.NewRequest("POST", hs.URL+"/mcp", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		r.Header.Set("Authorization", "Bearer "+tok)
		mut(r)
		res, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if c := try(func(r *http.Request) { r.Header.Del("Authorization") }); c != 401 {
		t.Errorf("no token: %d", c)
	}
	if c := try(func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+strings.Repeat("0", 64)) }); c != 401 {
		t.Errorf("wrong token: %d", c)
	}
	if c := try(func(r *http.Request) { r.Host = "evil.example:47615" }); c != 403 {
		t.Errorf("foreign host: %d", c)
	}
	if c := try(func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }); c != 403 {
		t.Errorf("browser origin: %d", c)
	}
	if c := try(func(*http.Request) {}); c == 401 || c == 403 {
		t.Errorf("good request rejected: %d", c)
	}
	// A new token locks the old one out at once.
	if _, err := s.RotateToken(); err != nil {
		t.Fatal(err)
	}
	if c := try(func(*http.Request) {}); c != 401 {
		t.Errorf("old token after rotation: %d", c)
	}
}

type bearer struct{ tok string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.tok)
	return http.DefaultTransport.RoundTrip(r)
}

func connect(t *testing.T, s *Server, url string) *mcp.ClientSession {
	t.Helper()
	tok, _ := s.Token()
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := c.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: url + "/mcp",
		HTTPClient: &http.Client{Transport: bearer{tok}}, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	return res.Content[0].(*mcp.TextContent).Text, res.IsError
}

func TestToolsEndToEnd(t *testing.T) {
	s, fb, hs := testServer(t)
	cs := connect(t, s, hs.URL)

	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tl := range tools.Tools {
		byName[tl.Name] = tl
	}
	for _, n := range []string{"send_whatsapp_message", "ask_agent", "list_chats", "update_agent", "send_to_claude_chat", "apply_change_card", "reset_board"} {
		if byName[n] == nil {
			t.Fatalf("missing tool %s", n)
		}
	}
	if d := byName["delete_agent"].Annotations.DestructiveHint; d == nil || !*d {
		t.Error("delete_agent should be destructive")
	}
	// update_agent takes the agent fields flat, next to agent.
	schema, _ := json.Marshal(byName["update_agent"].InputSchema)
	if !strings.Contains(string(schema), `"mode"`) || !strings.Contains(string(schema), `"agent"`) {
		t.Errorf("update_agent schema: %s", schema)
	}

	out, isErr := call(t, cs, "send_whatsapp_message", map[string]any{"chat": "souza", "text": "Oi Ana", "as_agent": "family"})
	if isErr || len(fb.sent) != 1 || fb.sent[0] != "a1|5551999994412@s.whatsapp.net|Oi Ana" || !strings.Contains(out, "🤖 Oi Ana") {
		t.Fatalf("send: %s %v", out, fb.sent)
	}

	out, isErr = call(t, cs, "send_whatsapp_message", map[string]any{"chat": "ana", "text": "x"})
	if !isErr || !strings.Contains(out, "Ana Lima") || len(fb.sent) != 1 {
		t.Fatalf("ambiguous chat should fail and list candidates: %s", out)
	}

	for _, n := range []string{"read_whatsapp_chat", "search_whatsapp_messages", "list_unread_whatsapp_chats", "mark_whatsapp_chat_read",
		"reply_whatsapp_message", "react_whatsapp_message", "delete_whatsapp_message", "send_whatsapp_file"} {
		if byName[n] == nil {
			t.Fatalf("missing tool %s", n)
		}
	}
	if d := byName["delete_whatsapp_message"].Annotations.DestructiveHint; d == nil || !*d {
		t.Error("delete_whatsapp_message should be destructive")
	}
	out, isErr = call(t, cs, "reply_whatsapp_message", map[string]any{"chat": "souza", "message_id": "M9", "text": "Combinado"})
	if isErr || fb.sent[len(fb.sent)-1] != "reply|5551999994412@s.whatsapp.net|M9|Combinado" || !strings.Contains(out, "Combinado") {
		t.Fatalf("reply: %s %v", out, fb.sent)
	}
	out, isErr = call(t, cs, "send_whatsapp_file", map[string]any{"chat": "souza", "path": "/tmp/a.pdf", "as_document": true})
	if isErr || fb.sent[len(fb.sent)-1] != "file|5551999994412@s.whatsapp.net|/tmp/a.pdf|document" {
		t.Fatalf("send file: %s %v", out, fb.sent)
	}
	fb.sent = fb.sent[:1]

	out, isErr = call(t, cs, "ask_agent", map[string]any{"chat": "Família", "request": "tell them I'm 20 minutes late"})
	if isErr || len(fb.asked) != 1 || fb.asked[0] != "|111@g.us|tell them I'm 20 minutes late" || !strings.Contains(out, "Chego 20 min atrasado") {
		t.Fatalf("ask: %s %v", out, fb.asked)
	}
}

func TestSetEnabledStartsStopsAndReportsABusyPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	busy := New(newFake(), Options{Home: t.TempDir(), Port: port})
	if err := busy.SetEnabled(true); err == nil {
		t.Fatal("a taken port should fail")
	}
	if running, msg := busy.Status(); running || !strings.Contains(msg, port) {
		t.Fatalf("status %v %q", running, msg)
	}
	ln.Close()

	if err := busy.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	if running, msg := busy.Status(); !running || msg != "" {
		t.Fatalf("status %v %q", running, msg)
	}
	cs := connect(t, busy, strings.TrimSuffix(busy.URL(), "/mcp"))
	if out, isErr := call(t, cs, "find_chat", map[string]any{"chat": "herter"}); isErr || !strings.Contains(out, "222@g.us") {
		t.Fatalf("find_chat: %s", out)
	}
	cs.Close()
	if err := busy.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	if _, err := http.Get(busy.URL()); err == nil {
		t.Fatal("the server should be stopped")
	}
}

// claudeFake answers a Claude chat with a card after a few polls.
type claudeFake struct {
	*fakeBackend
	mu    sync.Mutex
	polls int
	msgs  []app.AssistantMessageDTO
}

func (f *claudeFake) ListAssistantChats() []app.AssistantChatDTO {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.polls++
	busy := f.polls < 3
	if !busy && len(f.msgs) == 1 {
		f.msgs = append(f.msgs,
			app.AssistantMessageDTO{ID: "m2", Role: "progress", Text: "Read 40 messages"},
			app.AssistantMessageDTO{ID: "m3", Role: "card", Card: &app.CardDTO{Kind: "message", Status: "open",
				Message: &app.MessageProposalDTO{ChatJID: "111@g.us", Text: "Oi família"}}})
	}
	return []app.AssistantChatDTO{{ID: "c1", Title: "Lunch plans", Busy: busy}}
}
func (f *claudeFake) AssistantMessages(string) []app.AssistantMessageDTO {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]app.AssistantMessageDTO(nil), f.msgs...)
}
func (f *claudeFake) SendAssistant(_, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.polls = 0
	f.msgs = append(f.msgs, app.AssistantMessageDTO{ID: "m1", Role: "user", Text: text})
	return nil
}

func TestSendToClaudeChatWaitsAndReturnsCards(t *testing.T) {
	cf := &claudeFake{fakeBackend: newFake()}
	s := New(cf, Options{Home: t.TempDir(), Port: "0"})
	hs := httptest.NewServer(s.Handler())
	defer hs.Close()
	out, isErr := call(t, connect(t, s, hs.URL), "send_to_claude_chat", map[string]any{"chat": "lunch", "text": "draft a hello to the family"})
	if isErr || !strings.Contains(out, `"id": "m3"`) || !strings.Contains(out, "Oi família") || strings.Contains(out, "m1") || strings.Contains(out, "Read 40") {
		t.Fatalf("got %s", out)
	}
}
