package whatsapp

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// Disappearing messages. A chat with a timer expects every message to carry it in its
// ContextInfo; a message without it shows "This message won't disappear. The sender may be using
// an older version of WhatsApp" on the other side. WhatsApp keeps the timer per chat on each
// device, so Positron learns it from what arrives and remembers it (domain.Chat).

// Disappearing is an update (through OnUpdate): a chat's disappearing-messages timer changed.
type Disappearing struct {
	Chat    string
	Seconds uint32    // 0 = off
	SetAt   time.Time // when the setting changed; zero when unknown
}

// Timer is what Positron knows of a chat's disappearing-messages timer.
type Timer struct {
	Known   bool
	Seconds uint32 // 0 = off
	SetAt   int64  // Unix seconds of the setting change, 0 when unknown
}

// timerSeen is one sighting of a chat's timer.
type timerSeen struct {
	Seconds uint32
	SetAt   int64
	// Authoritative: a setting change or the server's group info, not a timer read off a message.
	Authoritative bool
}

// acceptTimer decides whether a sighting replaces what is known. Setting timestamps win when both
// have one; otherwise setting changes and group info always count, while a plain message only
// counts when it carries a timer. A message without one never turns the timer off: that is
// exactly what an old WhatsApp (or a bot) sends into a chat that has one.
func acceptTimer(cur Timer, seen timerSeen) bool {
	if !cur.Known {
		return seen.Authoritative || seen.SetAt > 0 || seen.Seconds > 0
	}
	if seen.SetAt > 0 && cur.SetAt > 0 {
		return seen.SetAt >= cur.SetAt
	}
	if seen.Authoritative || seen.SetAt > 0 {
		return true
	}
	return seen.Seconds > 0
}

// SetTimerSource gives the client the timers Positron remembered, read on first use of each chat.
// Set it before Start.
func (c *Client) SetTimerSource(f func(chat string) (Timer, bool)) { c.timerSource = f }

// timer returns what is known of a chat's timer.
func (c *Client) timer(chat string) Timer {
	c.timersMu.Lock()
	defer c.timersMu.Unlock()
	return c.timerLocked(chat)
}

func (c *Client) timerLocked(chat string) Timer {
	if t, ok := c.timers[chat]; ok {
		return t
	}
	var t Timer
	if c.timerSource != nil {
		if s, ok := c.timerSource(chat); ok {
			t = s
			t.Known = true
		}
	}
	if c.timers == nil {
		c.timers = map[string]Timer{}
	}
	c.timers[chat] = t
	return t
}

// learnTimer records a sighting of chat's timer and reports a change through OnUpdate.
func (c *Client) learnTimer(chat string, seen timerSeen) Timer {
	if chat == "" {
		return Timer{}
	}
	c.timersMu.Lock()
	cur := c.timerLocked(chat)
	if !acceptTimer(cur, seen) {
		c.timersMu.Unlock()
		return cur
	}
	next := Timer{Known: true, Seconds: seen.Seconds, SetAt: seen.SetAt}
	if next.SetAt == 0 && cur.Seconds == next.Seconds {
		next.SetAt = cur.SetAt // same timer, seen again without a timestamp
	}
	c.timers[chat] = next
	c.timersMu.Unlock()
	if next != cur {
		u := Disappearing{Chat: chat, Seconds: next.Seconds}
		if next.SetAt > 0 {
			u.SetAt = time.Unix(next.SetAt, 0)
		}
		c.update(u)
	}
	return next
}

// learnGroupTimer records a group's timer from its info, which comes from the server.
func (c *Client) learnGroupTimer(jid types.JID, e types.GroupEphemeral, at time.Time) Timer {
	seen := timerSeen{Authoritative: true}
	if e.IsEphemeral {
		seen.Seconds = e.DisappearingTimer
	}
	if !at.IsZero() {
		seen.SetAt = at.Unix()
	}
	return c.learnTimer(jid.String(), seen)
}

// ephemeralSetting reads a protocol message that changed a chat's disappearing messages (sent by
// someone in the chat, or by the owner on the phone), sent at at.
func ephemeralSetting(chat string, pm *waE2E.ProtocolMessage, at time.Time) Disappearing {
	d := Disappearing{Chat: chat, Seconds: pm.GetEphemeralExpiration(), SetAt: at}
	if ts := pm.GetEphemeralSettingTimestamp(); ts > 0 {
		d.SetAt = time.Unix(ts, 0)
	}
	return d
}

// learnFromMessage reads the timer off a live message: a setting change (Disappearing from
// classify, including the owner's change on the phone) or the timer an ordinary message carries.
// Positron's own messages are skipped; they carry whatever Positron put there.
func (c *Client) learnFromMessage(e *events.Message, in Incoming, upd any) {
	if d, ok := upd.(Disappearing); ok {
		seen := timerSeen{Seconds: d.Seconds, Authoritative: true}
		if !d.SetAt.IsZero() {
			seen.SetAt = d.SetAt.Unix()
		}
		c.learnTimer(d.Chat, seen)
		return
	}
	if in.ID == "" || in.FromPositron {
		return
	}
	ci := contextInfoOf(e.Message)
	if ci == nil {
		return
	}
	if exp, ts := ci.GetExpiration(), ci.GetEphemeralSettingTimestamp(); exp > 0 || ts > 0 {
		c.learnTimer(in.Chat, timerSeen{Seconds: exp, SetAt: ts})
	}
}

// timerFor finds the timer to put on a message to j: what is known for the chat, else for the
// same contact under its other JID (phone number or LID), else, for a group, the server's info.
func (c *Client) timerFor(ctx context.Context, cli *whatsmeow.Client, j types.JID) Timer {
	if t := c.timer(j.String()); t.Known {
		return t
	}
	switch j.Server {
	case types.GroupServer:
		gctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if g, err := cli.GetGroupInfo(gctx, j); err == nil {
			c.groupNames.Store(j.String(), g.Name)
			return c.learnGroupTimer(j, g.GroupEphemeral, time.Time{})
		}
	case types.DefaultUserServer, types.HiddenUserServer:
		var alt types.JID
		var err error
		if j.Server == types.HiddenUserServer {
			alt, err = cli.Store.LIDs.GetPNForLID(ctx, j)
		} else {
			alt, err = cli.Store.LIDs.GetLIDForPN(ctx, j)
		}
		if err == nil && !alt.IsEmpty() {
			if t := c.timer(alt.ToNonAD().String()); t.Known {
				return t
			}
		}
	}
	return Timer{}
}

// contextInfoOf returns the ContextInfo of a message's content, nil when it has none.
func contextInfoOf(m *waE2E.Message) *waE2E.ContextInfo {
	switch {
	case m == nil:
		return nil
	case m.GetExtendedTextMessage() != nil:
		return m.GetExtendedTextMessage().GetContextInfo()
	case m.GetImageMessage() != nil:
		return m.GetImageMessage().GetContextInfo()
	case m.GetVideoMessage() != nil:
		return m.GetVideoMessage().GetContextInfo()
	case m.GetAudioMessage() != nil:
		return m.GetAudioMessage().GetContextInfo()
	case m.GetDocumentMessage() != nil:
		return m.GetDocumentMessage().GetContextInfo()
	case m.GetDocumentWithCaptionMessage().GetMessage() != nil:
		return contextInfoOf(m.GetDocumentWithCaptionMessage().GetMessage())
	case m.GetStickerMessage() != nil:
		return m.GetStickerMessage().GetContextInfo()
	case m.GetContactMessage() != nil:
		return m.GetContactMessage().GetContextInfo()
	case m.GetLocationMessage() != nil:
		return m.GetLocationMessage().GetContextInfo()
	}
	return nil
}

// withTimer stamps a message for a chat with timer t, the way WhatsApp's own apps do: the
// timer (and when it was set) in the content's ContextInfo. A plain Conversation has no
// ContextInfo, so it becomes an ExtendedTextMessage. Without a timer m is left alone.
func withTimer(m *waE2E.Message, t Timer) *waE2E.Message {
	if m == nil || t.Seconds == 0 {
		return m
	}
	if m.Conversation != nil {
		m.ExtendedTextMessage = &waE2E.ExtendedTextMessage{Text: m.Conversation}
		m.Conversation = nil
	}
	var ci **waE2E.ContextInfo
	switch {
	case m.ExtendedTextMessage != nil:
		ci = &m.ExtendedTextMessage.ContextInfo
	case m.ImageMessage != nil:
		ci = &m.ImageMessage.ContextInfo
	case m.VideoMessage != nil:
		ci = &m.VideoMessage.ContextInfo
	case m.AudioMessage != nil:
		ci = &m.AudioMessage.ContextInfo
	case m.DocumentMessage != nil:
		ci = &m.DocumentMessage.ContextInfo
	case m.DocumentWithCaptionMessage.GetMessage().GetDocumentMessage() != nil:
		ci = &m.DocumentWithCaptionMessage.Message.DocumentMessage.ContextInfo
	case m.StickerMessage != nil:
		ci = &m.StickerMessage.ContextInfo
	default:
		return m
	}
	if *ci == nil {
		*ci = &waE2E.ContextInfo{}
	}
	secs := t.Seconds
	(*ci).Expiration = &secs
	if t.SetAt > 0 {
		at := t.SetAt
		(*ci).EphemeralSettingTimestamp = &at
	}
	return m
}

// sendMessage sends a new message to j with the chat's disappearing timer. Every new message
// Positron sends goes through here; reactions, edits and deletions do not need the timer.
func (c *Client) sendMessage(ctx context.Context, cli *whatsmeow.Client, j types.JID, m *waE2E.Message) (whatsmeow.SendResponse, error) {
	return cli.SendMessage(ctx, j, withTimer(m, c.timerFor(ctx, cli, j)))
}
