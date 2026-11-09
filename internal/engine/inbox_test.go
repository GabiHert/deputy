package engine

import (
	"context"
	"testing"
	"time"

	"github.com/GabiHert/positron/internal/claude"
	"github.com/GabiHert/positron/internal/domain"
	"github.com/GabiHert/positron/internal/whatsapp"
)

const loneChat = "7777@s.whatsapp.net" // no agent handles it

func answerMe(t *testing.T, mode domain.ReplyMode) (*Engine, *fakeWA, *fakeRunner) {
	t.Helper()
	e, st, wa, rn, a := setup(t, mode, domain.TriggerAll)
	a.AnswerMe = true
	if err := st.SaveAgent(a); err != nil {
		t.Fatal(err)
	}
	return e, wa, rn
}

func settle(e *Engine) {
	time.Sleep(300 * time.Millisecond)
	e.wg.Wait()
}

func TestHistoryNeverStartsRuns(t *testing.T) {
	e, wa, rn := answerMe(t, domain.ModeAuto)
	rn.res = claude.AgentResult{Reply: "should never be sent"}
	old := time.Now().Add(-48 * time.Hour)
	other := msg("group1@g.us", "H1", "old question @deputy", true, true)
	other.At = old
	own := msg("group1@g.us", "H2", "my old message", true, false)
	own.FromMe, own.At = true, old.Add(time.Minute)
	e.HandleUpdate(whatsapp.History{Type: "recent", Chats: []whatsapp.HistoryChat{{
		JID: "group1@g.us", Name: "Family", HasMeta: true, Unread: 3, Pinned: true,
		Messages: []whatsapp.Incoming{other, own},
	}}})
	// A single history message through the live path is only stored too.
	single := msg("5511@s.whatsapp.net", "H3", "older", false, false)
	single.History, single.At = true, old
	e.HandleIncoming(single)
	settle(e)
	if len(rn.prompts) != 0 || len(wa.all()) != 0 {
		t.Fatalf("history started a run: %d prompts, sent %+v", len(rn.prompts), wa.all())
	}
	m, err := e.d.Store.Message("group1@g.us", "H1")
	if err != nil || !m.History {
		t.Fatalf("history message not stored as history: %+v %v", m, err)
	}
	c, _ := e.d.Store.Chat("group1@g.us")
	if c.Unread != 3 || !c.Pinned || c.Name != "Family" || c.LastID != "H2" || !c.LastFromMe {
		t.Fatalf("chat from history: %+v", c)
	}
	// A history batch that resends a stored message leaves it alone.
	_, _, _ = e.d.Store.UpdateMessage("group1@g.us", "H1", func(m *domain.Message) bool { m.Transcript = "kept"; return true })
	e.HandleHistory(whatsapp.History{Type: "on-demand", Chats: []whatsapp.HistoryChat{{JID: "group1@g.us", Messages: []whatsapp.Incoming{other}}}})
	if m, _ := e.d.Store.Message("group1@g.us", "H1"); m.Transcript != "kept" {
		t.Fatalf("history overwrote a stored message: %+v", m)
	}
}

func TestUITypedMessageStartsAnswerMeAgentOnce(t *testing.T) {
	e, wa, rn := answerMe(t, domain.ModeAuto)
	rn.res = claude.AgentResult{Reply: "Hoje: dentista às 15h", Note: "answered"}
	m, err := e.SendOwner(context.Background(), "group1@g.us", whatsapp.Outgoing{Text: "deputy, what's on today?"})
	if err != nil {
		t.Fatal(err)
	}
	if !m.FromMe || !m.FromUI || m.Status != domain.StatusSent {
		t.Fatalf("stored %+v", m)
	}
	r := waitRun(t, e)
	if r.Status != domain.RunSent {
		t.Fatalf("run %s %s", r.Status, r.Error)
	}
	settle(e)
	if len(rn.prompts) != 1 {
		t.Fatalf("the agent's own reply started another run: %d prompts", len(rn.prompts))
	}
	got := wa.all()
	if len(got) != 2 || got[0].kind != "owner-text" || got[1].text != "🤖 Hoje: dentista às 15h" {
		t.Fatalf("sent %+v", got)
	}
}

func TestSendQuietAndSendNowNeverStartRuns(t *testing.T) {
	e, _, rn := answerMe(t, domain.ModeAuto)
	rn.res = claude.AgentResult{Reply: "no"}
	if _, err := e.SendQuiet(context.Background(), "group1@g.us", whatsapp.Outgoing{Text: "from Claude Code"}); err != nil {
		t.Fatal(err)
	}
	if err := e.SendNow("", "group1@g.us", "also from Claude Code"); err != nil {
		t.Fatal(err)
	}
	settle(e)
	if len(rn.prompts) != 0 {
		t.Fatalf("a Claude Code send started a run")
	}
}

func TestUITypedMessageWithoutAnswerMeIsJustSent(t *testing.T) {
	e, _, wa, rn, _ := setup(t, domain.ModeAuto, domain.TriggerAll)
	if _, err := e.SendOwner(context.Background(), "group1@g.us", whatsapp.Outgoing{Text: "oi"}); err != nil {
		t.Fatal(err)
	}
	settle(e)
	if len(rn.prompts) != 0 || len(wa.all()) != 1 {
		t.Fatalf("prompts %d sent %+v", len(rn.prompts), wa.all())
	}
}

func TestUnreadCounts(t *testing.T) {
	e, _, wa, _, _ := setup(t, domain.ModeAuto, domain.TriggerAll)
	var changes []string
	e.d.OnUnread = func(chat string) { changes = append(changes, chat) }
	unread := func() int { c, _ := e.d.Store.Chat(loneChat); return c.Unread }
	base := time.Now().Add(-time.Minute)
	for i, id := range []string{"U1", "U2"} {
		in := msg(loneChat, id, "hi", false, false)
		in.At = base.Add(time.Duration(i) * time.Second)
		e.HandleIncoming(in)
	}
	if unread() != 2 || len(changes) != 2 {
		t.Fatalf("unread %d, changes %v", unread(), changes)
	}
	if n := e.d.Store.TotalUnread(time.Now()); n != 2 {
		t.Fatalf("total %d", n)
	}
	// The owner wrote on the phone: they read the chat.
	own := msg(loneChat, "O1", "oi", false, false)
	own.FromMe, own.At = true, base.Add(3*time.Second)
	e.HandleIncoming(own)
	if unread() != 0 {
		t.Fatalf("unread after the owner's message: %d", unread())
	}
	in := msg(loneChat, "U3", "still there?", false, false)
	in.At = base.Add(4 * time.Second)
	e.HandleIncoming(in)
	in = msg(loneChat, "U4", "hello?", false, false)
	in.At = base.Add(5 * time.Second)
	e.HandleIncoming(in)
	// Read on the phone up to U3: U4 stays unread.
	e.HandleUpdate(whatsapp.Receipt{Chat: loneChat, IDs: []string{"U3"}, Status: "read", FromMe: true})
	if unread() != 1 {
		t.Fatalf("unread after reading on the phone: %d", unread())
	}
	if err := e.MarkChatRead(loneChat); err != nil {
		t.Fatal(err)
	}
	if unread() != 0 || len(wa.read["5599@s.whatsapp.net"]) != 1 || wa.read["5599@s.whatsapp.net"][0] != "U4" {
		t.Fatalf("unread %d, receipts %v", unread(), wa.read)
	}
	// Marked unread on the phone, then muted: the badge leaves it out.
	e.HandleUpdate(whatsapp.ChatRead{Chat: loneChat, Read: false})
	if unread() != 1 {
		t.Fatalf("mark as unread: %d", unread())
	}
	e.HandleUpdate(whatsapp.ChatMeta{Chat: loneChat, Field: "mute", On: true})
	if c, _ := e.d.Store.Chat(loneChat); !c.Muted(time.Now()) || e.d.Store.TotalUnread(time.Now()) != 0 {
		t.Fatalf("muted chat counted: %+v", c)
	}
	e.HandleUpdate(whatsapp.ChatRead{Chat: loneChat, Read: true, At: time.Now()})
	if unread() != 0 {
		t.Fatalf("marked read on the phone: %d", unread())
	}
}

func TestReactionsEditsAndDeletions(t *testing.T) {
	e, _, wa, _, _ := setup(t, domain.ModeAuto, domain.TriggerAll)
	var updates int
	e.d.OnMessage = func(_ domain.Message, change string) {
		if change == "update" {
			updates++
		}
	}
	e.HandleIncoming(msg(loneChat, "M1", "first", false, false))
	e.HandleIncoming(msg(loneChat, "M2", "second", false, false))
	get := func(id string) domain.Message { m, _ := e.d.Store.Message(loneChat, id); return m }

	e.HandleUpdate(whatsapp.Reaction{Chat: loneChat, TargetID: "M1", Sender: "5599@s.whatsapp.net", SenderName: "Ana", Emoji: "👍"})
	e.HandleUpdate(whatsapp.Reaction{Chat: loneChat, TargetID: "M1", Sender: "5599@s.whatsapp.net", SenderName: "Ana", Emoji: "❤️"})
	e.HandleUpdate(whatsapp.Reaction{Chat: loneChat, TargetID: "M1", FromMe: true, Emoji: "😂"})
	if r := get("M1").Reactions; len(r) != 2 || r[0].Emoji != "❤️" || !r[1].FromMe {
		t.Fatalf("reactions %+v", r)
	}
	e.HandleUpdate(whatsapp.Reaction{Chat: loneChat, TargetID: "M1", Sender: "5599@s.whatsapp.net", Emoji: ""})
	if r := get("M1").Reactions; len(r) != 1 || !r[0].FromMe {
		t.Fatalf("after removing Ana's: %+v", r)
	}
	// The owner's reaction from Deputy goes to WhatsApp with the sender of the message.
	if _, err := e.React(context.Background(), loneChat, "M2", "🙏"); err != nil {
		t.Fatal(err)
	}
	if len(wa.react) != 1 || wa.react[0] != "5599@s.whatsapp.net|M2|🙏" || len(get("M2").Reactions) != 1 {
		t.Fatalf("react %v %+v", wa.react, get("M2").Reactions)
	}

	e.HandleUpdate(whatsapp.Edit{Chat: loneChat, TargetID: "M2", Text: "second, fixed"})
	if m := get("M2"); m.Text != "second, fixed" || !m.Edited {
		t.Fatalf("edit %+v", m)
	}

	before, _ := e.d.Store.Chat(loneChat)
	e.HandleUpdate(whatsapp.Revoke{Chat: loneChat, TargetID: "M2", Sender: "5599@s.whatsapp.net"})
	m := get("M2")
	if !m.Revoked || m.Text != "" || len(m.Reactions) != 0 {
		t.Fatalf("revoke %+v", m)
	}
	after, _ := e.d.Store.Chat(loneChat)
	if after.Unread != before.Unread-1 || after.LastBody != "This message was deleted" {
		t.Fatalf("chat after revoke: unread %d -> %d, %q", before.Unread, after.Unread, after.LastBody)
	}
	// Changes to messages Deputy never saw are ignored.
	e.HandleUpdate(whatsapp.Edit{Chat: loneChat, TargetID: "NOPE", Text: "x"})
	if updates != 7 {
		t.Fatalf("updates reported: %d", updates)
	}
	// Only the owner's messages can be deleted for everyone.
	if _, err := e.Revoke(context.Background(), loneChat, "M1"); err == nil {
		t.Fatal("deleted someone else's message")
	}
}

func TestReceiptsOnlyMoveForward(t *testing.T) {
	e, _, _, _, _ := setup(t, domain.ModeAuto, domain.TriggerAll)
	var got []string
	e.d.OnReceipt = func(chat string, ids []string, status string) { got = append(got, status) }
	m, err := e.SendOwner(context.Background(), loneChat, whatsapp.Outgoing{Text: "oi"})
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range []string{"read", "delivered", "played"} {
		e.HandleUpdate(whatsapp.Receipt{Chat: "other-jid-for-the-same-chat@lid", IDs: []string{m.ID}, Status: st})
	}
	saved, _ := e.d.Store.Message(loneChat, m.ID)
	if saved.Status != domain.StatusPlayed || len(got) != 2 || got[0] != "read" || got[1] != "played" {
		t.Fatalf("status %s, events %v", saved.Status, got)
	}
}

func TestDisappearingTimerIsPersisted(t *testing.T) {
	e, _, _ := answerMe(t, domain.ModeAuto)
	at := time.Unix(1700000000, 0)
	e.HandleUpdate(whatsapp.Disappearing{Chat: loneChat, Seconds: 86400, SetAt: at})
	c, err := e.d.Store.Chat(loneChat)
	if err != nil || c.Disappearing != 86400 || !c.DisappearingKnown || !c.DisappearingAt.Equal(at) {
		t.Fatalf("chat %+v %v", c, err)
	}
	e.HandleUpdate(whatsapp.Disappearing{Chat: loneChat})
	if c, _ := e.d.Store.Chat(loneChat); c.Disappearing != 0 || !c.DisappearingKnown {
		t.Fatalf("turned off: %+v", c)
	}
}

func TestViewOncePlaceholderIsFilledInOnce(t *testing.T) {
	e, _, _ := answerMe(t, domain.ModeAuto)
	in := msg(loneChat, "V1", "", false, false)
	in.Kind, in.ViewOnce = "image", true
	e.HandleIncoming(in)
	m, err := e.d.Store.Message(loneChat, "V1")
	if err != nil || !m.ViewOnce || m.Kind != domain.KindImage || len(m.MediaRef) != 0 {
		t.Fatalf("placeholder %+v %v", m, err)
	}
	if c, _ := e.d.Store.Chat(loneChat); c.Unread != 1 || c.LastBody != "[view-once photo]" {
		t.Fatalf("chat %+v", c)
	}
	// The phone resends it with the media: filled in, not counted again.
	again := in
	again.Kind, again.MediaRef, again.Mime = "video", []byte{1}, "video/mp4"
	e.HandleIncoming(again)
	m, _ = e.d.Store.Message(loneChat, "V1")
	if m.Kind != domain.KindVideo || len(m.MediaRef) == 0 || !m.ViewOnce {
		t.Fatalf("not filled in: %+v", m)
	}
	if c, _ := e.d.Store.Chat(loneChat); c.Unread != 1 {
		t.Fatalf("counted twice: %+v", c)
	}
}

func TestViewOnceInPrompt(t *testing.T) {
	cases := []struct {
		m    domain.Message
		want string
	}{
		{domain.Message{Kind: domain.KindImage, ViewOnce: true}, "[view-once photo, not available]"},
		{domain.Message{Kind: domain.KindVoice, ViewOnce: true, Seconds: 4}, "[view-once voice note, not available]"},
		{domain.Message{Kind: domain.KindVideo, ViewOnce: true, Text: "olha"}, "[view-once video, not available] olha"},
		{domain.Message{ID: "P", Kind: domain.KindImage, ViewOnce: true, MediaRef: []byte{1}}, "[view-once photo: .deputy/media/p.jpg]"},
		{domain.Message{Kind: domain.KindImage}, "[photo, not available]"},
	}
	for _, c := range cases {
		if got := body(c.m, map[string]string{"P": ".deputy/media/p.jpg"}); got != c.want {
			t.Errorf("body(%+v) = %q, want %q", c.m, got, c.want)
		}
	}
}
