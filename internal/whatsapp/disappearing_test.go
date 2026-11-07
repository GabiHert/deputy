package whatsapp

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

const day = 86400

func TestWithTimerStampsEveryKind(t *testing.T) {
	timer := Timer{Known: true, Seconds: day, SetAt: 1700000000}
	kinds := map[string]*waE2E.Message{
		"text":     {Conversation: proto.String("oi")},
		"extended": {ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("oi"), ContextInfo: &waE2E.ContextInfo{StanzaID: proto.String("Q1")}}},
		"image":    {ImageMessage: &waE2E.ImageMessage{Caption: proto.String("c")}},
		"video":    {VideoMessage: &waE2E.VideoMessage{}},
		"audio":    {AudioMessage: &waE2E.AudioMessage{PTT: proto.Bool(true)}},
		"document": {DocumentMessage: &waE2E.DocumentMessage{FileName: proto.String("a.pdf")}},
		"captioned": {DocumentWithCaptionMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{
			DocumentMessage: &waE2E.DocumentMessage{Caption: proto.String("c")}}}},
		"sticker": {StickerMessage: &waE2E.StickerMessage{}},
	}
	for name, m := range kinds {
		out := withTimer(m, timer)
		ci := contextInfoOf(out)
		if ci.GetExpiration() != day || ci.GetEphemeralSettingTimestamp() != 1700000000 {
			t.Errorf("%s: context info %v", name, ci)
		}
	}
	text := withTimer(&waE2E.Message{Conversation: proto.String("oi")}, timer)
	if text.Conversation != nil || text.GetExtendedTextMessage().GetText() != "oi" {
		t.Errorf("conversation not turned into extended text: %v", text)
	}
	if q := withTimer(kinds["extended"], timer).GetExtendedTextMessage().GetContextInfo().GetStanzaID(); q != "Q1" {
		t.Errorf("the quote was lost: %q", q)
	}
}

func TestWithTimerLeavesChatsWithoutOne(t *testing.T) {
	m := withTimer(&waE2E.Message{Conversation: proto.String("oi")}, Timer{Known: true})
	if m.GetConversation() != "oi" || m.ExtendedTextMessage != nil {
		t.Errorf("changed a message for a chat without a timer: %v", m)
	}
	// No setting time known: the timer alone.
	ci := contextInfoOf(withTimer(&waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}, Timer{Known: true, Seconds: 7 * day}))
	if ci.GetExpiration() != 7*day || ci.EphemeralSettingTimestamp != nil {
		t.Errorf("context info %v", ci)
	}
}

func TestAcceptTimer(t *testing.T) {
	on := Timer{Known: true, Seconds: day, SetAt: 100}
	cases := []struct {
		name string
		cur  Timer
		seen timerSeen
		want bool
	}{
		{"unknown, message with a timer", Timer{}, timerSeen{Seconds: day}, true},
		{"unknown, message without one", Timer{}, timerSeen{}, false},
		{"unknown, setting turned off", Timer{}, timerSeen{Authoritative: true}, true},
		{"message without a timer never turns it off", on, timerSeen{}, false},
		{"older setting loses", on, timerSeen{Seconds: 0, SetAt: 50, Authoritative: true}, false},
		{"newer setting wins", on, timerSeen{Seconds: 0, SetAt: 200, Authoritative: true}, true},
		{"message from after the change", on, timerSeen{Seconds: 0, SetAt: 200}, true},
		{"server group info without a time", on, timerSeen{Authoritative: true}, true},
		{"message timer without a time", Timer{Known: true}, timerSeen{Seconds: 7 * day}, true},
	}
	for _, c := range cases {
		if got := acceptTimer(c.cur, c.seen); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

func newTestClient(stored map[string]Timer) (*Client, *[]Disappearing) {
	c := &Client{}
	var ups []Disappearing
	c.OnUpdate(func(u any) {
		if d, ok := u.(Disappearing); ok {
			ups = append(ups, d)
		}
	})
	c.SetTimerSource(func(chat string) (Timer, bool) {
		t, ok := stored[chat]
		return t, ok
	})
	return c, &ups
}

func TestTimerFromProtocolMessage(t *testing.T) {
	c, ups := newTestClient(nil)
	chat := "5511@s.whatsapp.net"
	at := time.Unix(1700000000, 0)
	pm := &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_EPHEMERAL_SETTING.Enum(), EphemeralExpiration: proto.Uint32(7 * day)}
	d := ephemeralSetting(chat, pm, at)
	if d.Seconds != 7*day || !d.SetAt.Equal(at) {
		t.Fatalf("setting %+v", d)
	}
	// The owner's own change on the phone counts the same.
	c.learnFromMessage(&events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{IsFromMe: true}}}, Incoming{}, d)
	if got := c.timer(chat); got.Seconds != 7*day || got.SetAt != at.Unix() {
		t.Fatalf("timer %+v", got)
	}
	if len(*ups) != 1 || (*ups)[0].Seconds != 7*day {
		t.Fatalf("updates %+v", *ups)
	}
	// Turned off later.
	off := ephemeralSetting(chat, &waE2E.ProtocolMessage{EphemeralExpiration: proto.Uint32(0)}, at.Add(time.Hour))
	c.learnFromMessage(&events.Message{}, Incoming{}, off)
	if got := c.timer(chat); got.Seconds != 0 || !got.Known {
		t.Fatalf("not turned off: %+v", got)
	}
}

func TestTimerFromIncomingMessage(t *testing.T) {
	chat := "5511@s.whatsapp.net"
	c, ups := newTestClient(map[string]Timer{chat: {Seconds: 0}})
	ephemeral := &events.Message{IsEphemeral: true, Message: &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
		Text: proto.String("oi"), ContextInfo: &waE2E.ContextInfo{Expiration: proto.Uint32(day)}}}}
	c.learnFromMessage(ephemeral, Incoming{ID: "M1", Chat: chat}, nil)
	if got := c.timer(chat); got.Seconds != day || !got.Known {
		t.Fatalf("timer %+v", got)
	}
	// Seeing it again changes nothing; a message from an old app without a timer neither.
	c.learnFromMessage(ephemeral, Incoming{ID: "M2", Chat: chat}, nil)
	c.learnFromMessage(&events.Message{Message: &waE2E.Message{Conversation: proto.String("oi")}}, Incoming{ID: "M3", Chat: chat}, nil)
	if got := c.timer(chat); got.Seconds != day || len(*ups) != 1 {
		t.Fatalf("timer %+v after %d updates", got, len(*ups))
	}
	// Deputy's own messages are never read for it.
	c.learnFromMessage(&events.Message{Message: &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
		ContextInfo: &waE2E.ContextInfo{Expiration: proto.Uint32(7 * day)}}}}, Incoming{ID: "M4", Chat: chat, FromDeputy: true}, nil)
	if got := c.timer(chat); got.Seconds != day {
		t.Fatalf("learned from Deputy's own message: %+v", got)
	}
}

func TestTimerSourceIsRemembered(t *testing.T) {
	chat := "group@g.us"
	c, ups := newTestClient(map[string]Timer{chat: {Seconds: 90 * day, SetAt: 10}})
	if got := c.timer(chat); !got.Known || got.Seconds != 90*day {
		t.Fatalf("stored timer not read: %+v", got)
	}
	c.learnGroupTimer(types.NewJID("group", types.GroupServer), types.GroupEphemeral{IsEphemeral: true, DisappearingTimer: 90 * day}, time.Time{})
	if len(*ups) != 0 {
		t.Fatalf("an unchanged timer was reported: %+v", *ups)
	}
	c.learnGroupTimer(types.NewJID("group", types.GroupServer), types.GroupEphemeral{IsEphemeral: false}, time.Unix(20, 0))
	if got := c.timer(chat); got.Seconds != 0 || len(*ups) != 1 {
		t.Fatalf("group turned off: %+v, %+v", got, *ups)
	}
}
