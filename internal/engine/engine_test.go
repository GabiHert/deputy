package engine

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GabiHert/positron/internal/claude"
	"github.com/GabiHert/positron/internal/domain"
	"github.com/GabiHert/positron/internal/store"
	"github.com/GabiHert/positron/internal/whatsapp"
)

type sent struct{ chat, kind, text string }

type fakeWA struct {
	mu    sync.Mutex
	sent  []sent
	read  map[string][]string // sender -> message IDs marked read
	seq   int
	react []string
}

func (f *fakeWA) Send(_ context.Context, chat string, out whatsapp.Outgoing) (whatsapp.Sent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	f.sent = append(f.sent, sent{chat, "owner-" + out.Kind, out.Text})
	return whatsapp.Sent{ID: "UI" + strconv.Itoa(f.seq), At: time.Now()}, nil
}
func (f *fakeWA) React(_ context.Context, chat, sender, id, emoji string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.react = append(f.react, sender+"|"+id+"|"+emoji)
	return nil
}
func (f *fakeWA) Revoke(context.Context, string, string) error           { return nil }
func (f *fakeWA) EditText(context.Context, string, string, string) error { return nil }
func (f *fakeWA) MarkRead(chat, sender string, ids []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.read == nil {
		f.read = map[string][]string{}
	}
	f.read[sender] = append(f.read[sender], ids...)
}

func (f *fakeWA) SendText(_ context.Context, chat, text string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sent{chat, "text", text})
	return "OUT" + time.Now().Format("150405.000000"), nil
}
func (f *fakeWA) SendImage(_ context.Context, chat string, _ []byte, _, caption string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sent{chat, "image", caption})
	return "IMG" + time.Now().Format("150405.000000"), nil
}
func (f *fakeWA) SendVoice(_ context.Context, chat string, _ []byte, _ int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sent{chat, "voice", ""})
	return "VOI", nil
}
func (f *fakeWA) Typing(string, bool, bool)  {}
func (f *fakeWA) ChatName(jid string) string { return "Chat " + jid[:4] }
func (f *fakeWA) State() whatsapp.State {
	return whatsapp.State{State: whatsapp.StateConnected, PushName: "Gabriel"}
}
func (f *fakeWA) all() []sent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sent(nil), f.sent...)
}

type fakeRunner struct {
	mu      sync.Mutex
	res     claude.AgentResult
	prompts []string
	write   string // file to create in the repo, to test commits
}

func (f *fakeRunner) Run(_ context.Context, r claude.AgentRun) (claude.AgentResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prompts = append(f.prompts, r.Prompt)
	if f.write != "" {
		_ = os.WriteFile(filepath.Join(r.Dir, f.write), []byte("learned\n"), 0o644)
	}
	return f.res, nil
}

func setup(t *testing.T, mode domain.ReplyMode, trig domain.Trigger) (*Engine, *store.Store, *fakeWA, *fakeRunner, domain.Agent) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	a := domain.NewAgent(domain.DefaultSettings().Defaults)
	a.ID, a.Name, a.RepoPath, a.Mode, a.Trigger, a.WaitSeconds = "a1", "Tester", filepath.Join(dir, "kb"), mode, trig, 0
	a.Chats = []string{"group1@g.us", "5511@s.whatsapp.net"}
	if err := st.SaveAgent(a); err != nil {
		t.Fatal(err)
	}
	wa, rn := &fakeWA{}, &fakeRunner{}
	runs := make(chan domain.Run, 20)
	e := New(Deps{Store: st, WA: wa, Runner: rn, OnRun: func(r domain.Run) {
		if r.Status != domain.RunRunning {
			runs <- r
		}
	}})
	t.Cleanup(e.Close)
	runCh[e] = runs
	return e, st, wa, rn, a
}

var runCh = map[*Engine]chan domain.Run{}

func waitRun(t *testing.T, e *Engine) domain.Run {
	t.Helper()
	select {
	case r := <-runCh[e]:
		return r
	case <-time.After(10 * time.Second):
		t.Fatal("no run finished")
	}
	return domain.Run{}
}

func msg(chat, id, text string, group, mention bool) whatsapp.Incoming {
	return whatsapp.Incoming{ID: id, Chat: chat, IsGroup: group, Sender: "5599@s.whatsapp.net", SenderName: "Ana", At: time.Now(),
		Kind: "text", Text: text, MentionsMe: mention}
}

func TestAutoReplyIsSentWithPrefixAndCommitted(t *testing.T) {
	e, st, wa, rn, _ := setup(t, domain.ModeAuto, domain.TriggerMention)
	rn.res = claude.AgentResult{Reply: "Oi Ana!", Note: "greeted"}
	rn.write = "knowledge/ana.md"
	e.HandleIncoming(msg("group1@g.us", "M1", "@positron oi", true, true))
	r := waitRun(t, e)
	if r.Status != domain.RunSent {
		t.Fatalf("status %s, error %q", r.Status, r.Error)
	}
	got := wa.all()
	if len(got) != 1 || got[0].text != "🤖 Oi Ana!" {
		t.Fatalf("sent %+v", got)
	}
	if r.Commit == "" || len(r.Files) == 0 {
		t.Fatalf("expected a commit, got %+v", r)
	}
	if !strings.Contains(rn.prompts[0], "NEW [") || !strings.Contains(rn.prompts[0], "Ana: (mentions or replies to the owner) @positron oi") {
		t.Fatalf("prompt:\n%s", rn.prompts[0])
	}
	// The reply is in the history as the agent's own message.
	h := st.RecentMessages("group1@g.us", 10)
	if len(h) != 2 || !h[1].FromAgent {
		t.Fatalf("history %+v", h)
	}
}

func TestGroupMessageWithoutMentionIsIgnored(t *testing.T) {
	e, _, _, rn, _ := setup(t, domain.ModeAuto, domain.TriggerMention)
	e.HandleIncoming(msg("group1@g.us", "M1", "bom dia", true, false))
	time.Sleep(300 * time.Millisecond)
	e.wg.Wait()
	if len(rn.prompts) != 0 {
		t.Fatal("agent ran without a mention")
	}
}

func TestDirectChatAlwaysTriggersAndApproveWaits(t *testing.T) {
	e, _, wa, rn, _ := setup(t, domain.ModeApprove, domain.TriggerMention)
	rn.res = claude.AgentResult{Reply: "Vou sim", Note: "answered"}
	e.HandleIncoming(msg("5511@s.whatsapp.net", "D1", "vem domingo?", false, false))
	r := waitRun(t, e)
	if r.Status != domain.RunWaiting || len(wa.all()) != 0 {
		t.Fatalf("status %s sent %+v", r.Status, wa.all())
	}
	if err := e.SendDraft(r.ID, "Vou sim, chego meio-dia"); err != nil {
		t.Fatal(err)
	}
	if got := wa.all(); len(got) != 1 || got[0].text != "🤖 Vou sim, chego meio-dia" {
		t.Fatalf("sent %+v", got)
	}
	if err := e.SendDraft(r.ID, ""); err == nil {
		t.Fatal("sending twice should fail")
	}
}

func TestQuietAndOwnMessages(t *testing.T) {
	e, _, wa, rn, _ := setup(t, domain.ModeAuto, domain.TriggerAll)
	rn.res = claude.AgentResult{Reply: "", Note: "small talk"}
	own := msg("group1@g.us", "O1", "hello", true, false)
	own.FromMe = true
	e.HandleIncoming(own)
	time.Sleep(200 * time.Millisecond)
	if len(rn.prompts) != 0 {
		t.Fatal("the owner's own message started a run")
	}
	e.HandleIncoming(msg("group1@g.us", "M2", "kkk", true, false))
	r := waitRun(t, e)
	if r.Status != domain.RunQuiet || len(wa.all()) != 0 {
		t.Fatalf("status %s sent %+v", r.Status, wa.all())
	}
	if !strings.Contains(rn.prompts[0], "Gabriel (owner): hello") {
		t.Fatalf("prompt lacks the owner's message:\n%s", rn.prompts[0])
	}
}

func TestAnswerMe(t *testing.T) {
	e, st, _, rn, a := setup(t, domain.ModeAuto, domain.TriggerMention)
	a.AnswerMe = true
	if err := st.SaveAgent(a); err != nil {
		t.Fatal(err)
	}
	rn.res = claude.AgentResult{Reply: "oi", Note: "answered"}
	fromPositron := msg("group1@g.us", "D1", "sent by Positron", true, false)
	fromPositron.FromMe, fromPositron.FromPositron = true, true
	e.HandleIncoming(fromPositron)
	time.Sleep(200 * time.Millisecond)
	if len(rn.prompts) != 0 {
		t.Fatal("a message Positron sent started a run")
	}
	own := msg("group1@g.us", "O1", "positron, what's on today?", true, false)
	own.FromMe = true
	e.HandleIncoming(own)
	waitRun(t, e)
	if len(rn.prompts) != 1 || !strings.Contains(rn.prompts[0], "NEW") {
		t.Fatalf("the owner's message should start one run, got %d", len(rn.prompts))
	}
}

func TestHourlyLimitHoldsReplies(t *testing.T) {
	e, st, wa, rn, a := setup(t, domain.ModeAuto, domain.TriggerAll)
	a.MaxPerHour = 1
	_ = st.SaveAgent(a)
	rn.res = claude.AgentResult{Reply: "ok", Note: ""}
	e.HandleIncoming(msg("group1@g.us", "L1", "one", true, false))
	if r := waitRun(t, e); r.Status != domain.RunSent {
		t.Fatalf("first: %s %s", r.Status, r.Error)
	}
	e.HandleIncoming(msg("group1@g.us", "L2", "two", true, false))
	r := waitRun(t, e)
	if r.Status != domain.RunWaiting || len(wa.all()) != 1 {
		t.Fatalf("second: %s, sent %d", r.Status, len(wa.all()))
	}
}

func TestImagesMustBeInsideTheRepo(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "charts"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "charts", "a.png"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "notes.md"), []byte("x"), 0o644)
	got := validImages(dir, []string{"charts/a.png", "../etc/passwd.png", "notes.md", "missing.png", filepath.Join(dir, "charts", "a.png")})
	if len(got) != 2 || got[0] != "charts/a.png" || got[1] != "charts/a.png" {
		t.Fatalf("got %v", got)
	}
}

// TestRealClaude runs the agent through the person's real Claude Code. Opt in with POSITRON_E2E=1.
func TestRealClaude(t *testing.T) {
	if os.Getenv("POSITRON_E2E") != "1" {
		t.Skip("set POSITRON_E2E=1 to run Claude Code for real")
	}
	e, _, wa, _, a := setup(t, domain.ModeAuto, domain.TriggerMention)
	e.d.Runner = claude.Runner{}
	a.Model = "haiku"
	_ = e.d.Store.SaveAgent(a)
	e.HandleIncoming(msg("5511@s.whatsapp.net", "R1", "Hi! I'm Ana, I just adopted a cat called Miso. What should I feed a kitten?", false, false))
	select {
	case r := <-runCh[e]:
		t.Logf("status=%s reply=%q note=%q commit=%s files=%v error=%q", r.Status, r.Reply, r.Note, r.Commit, r.Files, r.Error)
		if r.Status != domain.RunSent || len(wa.all()) != 1 {
			t.Fatalf("expected a sent reply")
		}
	case <-time.After(5 * time.Minute):
		t.Fatal("timeout")
	}
}

func TestRequestSendsAtOnceEvenInApproveMode(t *testing.T) {
	e, st, wa, rn, a := setup(t, domain.ModeApprove, domain.TriggerMention)
	a.MaxPerHour = 1
	_ = st.SaveAgent(a)
	_ = st.SaveRun(domain.Run{ID: "old", At: time.Now(), AgentID: a.ID, ChatJID: "group1@g.us", Status: domain.RunSent})
	rn.res = claude.AgentResult{Reply: "Gente, o almoço é às 13h", Note: "told the group"}
	r, err := e.Request(context.Background(), "", "group1@g.us", "tell the group lunch moved to 1pm", false)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != domain.RunSent || r.Request != "tell the group lunch moved to 1pm" {
		t.Fatalf("run %+v", r)
	}
	if got := wa.all(); len(got) != 1 || got[0].text != "🤖 Gente, o almoço é às 13h" {
		t.Fatalf("sent %+v", got)
	}
	p := rn.prompts[0]
	if !strings.Contains(p, "Gabriel asked you directly") || !strings.Contains(p, "tell the group lunch moved to 1pm") || strings.Contains(p, "Decide whether to reply") {
		t.Fatalf("prompt:\n%s", p)
	}
	if saved, _ := st.Run(r.ID); saved.Request == "" || saved.Status != domain.RunSent {
		t.Fatalf("saved %+v", saved)
	}
}

func TestRequestWithReviewWaits(t *testing.T) {
	e, _, wa, rn, _ := setup(t, domain.ModeAuto, domain.TriggerMention)
	rn.res = claude.AgentResult{Reply: "Oi Ana, tudo certo pra domingo?", Note: "asked Ana"}
	r, err := e.Request(context.Background(), "a1", "5511@s.whatsapp.net", "ask Ana if Sunday is still on", true)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != domain.RunWaiting || len(wa.all()) != 0 {
		t.Fatalf("status %s sent %+v", r.Status, wa.all())
	}
	if err := e.SendDraft(r.ID, ""); err != nil || len(wa.all()) != 1 {
		t.Fatalf("send draft: %v %+v", err, wa.all())
	}
}

func TestRequestNeedsAChat(t *testing.T) {
	e, _, _, _, _ := setup(t, domain.ModeAuto, domain.TriggerMention)
	if _, err := e.Request(context.Background(), "a1", "", "do it", false); err == nil {
		t.Fatal("an agent with two chats needs the chat named")
	}
	if _, err := e.Request(context.Background(), "", "999@g.us", "do it", false); err == nil {
		t.Fatal("a chat without an agent needs the agent named")
	}
}
