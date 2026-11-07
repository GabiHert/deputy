package domain

import "time"

// MsgKind is what a WhatsApp message carries.
type MsgKind string

const (
	KindText     MsgKind = "text"
	KindImage    MsgKind = "image"
	KindVoice    MsgKind = "voice" // push-to-talk voice note
	KindAudio    MsgKind = "audio" // an audio file
	KindVideo    MsgKind = "video"
	KindDocument MsgKind = "document"
	KindSticker  MsgKind = "sticker"
	KindOther    MsgKind = "other"
)

// Message is one WhatsApp message in a chat's history.
type Message struct {
	ID         string    `json:"id"`
	ChatJID    string    `json:"chatJid"`
	SenderJID  string    `json:"senderJid"`
	SenderName string    `json:"senderName"`
	FromMe     bool      `json:"fromMe"`
	FromAgent  bool      `json:"fromAgent"` // sent by Deputy on the person's behalf
	At         time.Time `json:"at"`
	Kind       MsgKind   `json:"kind"`
	Text       string    `json:"text"` // body, or the media caption
	MediaPath  string    `json:"mediaPath,omitempty"`
	Mime       string    `json:"mime,omitempty"`
	Seconds    int       `json:"seconds,omitempty"`
	Transcript string    `json:"transcript,omitempty"`
	MentionsMe bool      `json:"mentionsMe"`
	QuotedID   string    `json:"quotedId,omitempty"`

	// Inbox fields: everything a full WhatsApp client shows.
	QuotedSender string     `json:"quotedSender,omitempty"` // JID of the quoted message's sender
	QuotedText   string     `json:"quotedText,omitempty"`   // snippet of the quoted message, as WhatsApp sent it
	QuotedKind   MsgKind    `json:"quotedKind,omitempty"`
	Mentions     []string   `json:"mentions,omitempty"` // mentioned JIDs
	FileName     string     `json:"fileName,omitempty"`
	FileSize     int64      `json:"fileSize,omitempty"`
	MediaRef     []byte     `json:"mediaRef,omitempty"` // the media message proto, to download the file later
	Reactions    []Reaction `json:"reactions,omitempty"`
	Status       MsgStatus  `json:"status,omitempty"` // own messages only
	Edited       bool       `json:"edited,omitempty"`
	Revoked      bool       `json:"revoked,omitempty"` // deleted for everyone
	History      bool       `json:"history,omitempty"` // came from a history sync, not live
	FromUI       bool       `json:"fromUi,omitempty"`  // typed by the owner in Deputy's own inbox
	// ViewOnce is a view-once photo, video or voice note. Without MediaRef it is a placeholder:
	// WhatsApp kept the media on the phone.
	ViewOnce bool `json:"viewOnce,omitempty"`
}

// Reaction is one person's emoji on a message.
type Reaction struct {
	Sender     string    `json:"sender"` // JID; "" for the owner
	SenderName string    `json:"senderName"`
	FromMe     bool      `json:"fromMe"`
	Emoji      string    `json:"emoji"`
	At         time.Time `json:"at"`
}

// MsgStatus is how far one of the owner's messages got.
type MsgStatus string

const (
	StatusPending   MsgStatus = "pending"
	StatusSent      MsgStatus = "sent"
	StatusDelivered MsgStatus = "delivered"
	StatusRead      MsgStatus = "read"
	StatusPlayed    MsgStatus = "played"
	StatusFailed    MsgStatus = "failed"
)

var statusRank = map[MsgStatus]int{StatusFailed: 0, StatusPending: 1, StatusSent: 2, StatusDelivered: 3, StatusRead: 4, StatusPlayed: 5}

// Upgrade returns the further of two statuses; receipts can arrive out of order.
func (s MsgStatus) Upgrade(to MsgStatus) MsgStatus {
	if statusRank[to] > statusRank[s] || s == "" {
		return to
	}
	return s
}

// SetReaction puts sender's emoji on m, or removes it when emoji is "". It reports whether m changed.
func (m *Message) SetReaction(r Reaction) bool {
	for i, old := range m.Reactions {
		if old.FromMe == r.FromMe && (r.FromMe || old.Sender == r.Sender) {
			if r.Emoji == "" {
				m.Reactions = append(m.Reactions[:i], m.Reactions[i+1:]...)
				return true
			}
			if old.Emoji == r.Emoji {
				return false
			}
			m.Reactions[i] = r
			return true
		}
	}
	if r.Emoji == "" {
		return false
	}
	m.Reactions = append(m.Reactions, r)
	return true
}

// Chat is a WhatsApp chat or group Deputy has seen.
type Chat struct {
	JID      string    `json:"jid"`
	Name     string    `json:"name"`
	IsGroup  bool      `json:"isGroup"`
	Members  int       `json:"members"`
	LastText string    `json:"lastText"`
	LastAt   time.Time `json:"lastAt"`

	// Inbox fields.
	LastID     string    `json:"lastId,omitempty"`
	LastBody   string    `json:"lastBody,omitempty"` // preview without the sender
	LastKind   MsgKind   `json:"lastKind,omitempty"`
	LastSender string    `json:"lastSender,omitempty"` // sender name, "" for the owner
	LastFromMe bool      `json:"lastFromMe,omitempty"`
	Unread     int       `json:"unread,omitempty"`
	ReadAt     time.Time `json:"readAt"`               // the owner read everything up to here
	MutedUntil time.Time `json:"mutedUntil,omitempty"` // zero = not muted
	Pinned     bool      `json:"pinned,omitempty"`
	Archived   bool      `json:"archived,omitempty"`

	// Disappearing messages, as last learned from WhatsApp: the timer in seconds (0 = off), when
	// it was set (zero when unknown), and whether Deputy knows it at all.
	Disappearing      int       `json:"disappearing,omitempty"`
	DisappearingAt    time.Time `json:"disappearingAt,omitempty"`
	DisappearingKnown bool      `json:"disappearingKnown,omitempty"`
}

// Forever is the MutedUntil of a chat muted with no end.
var Forever = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)

// Muted reports whether the chat is muted at t.
func (c Chat) Muted(t time.Time) bool { return !c.MutedUntil.IsZero() && c.MutedUntil.After(t) }

// RunStatus is where an agent run ended up.
type RunStatus string

const (
	RunRunning   RunStatus = "running"
	RunSent      RunStatus = "sent"
	RunWaiting   RunStatus = "waiting" // reply waits for the person
	RunDiscarded RunStatus = "discarded"
	RunQuiet     RunStatus = "quiet"   // the agent chose not to reply
	RunLearned   RunStatus = "learned" // "never" mode: no reply by design
	RunFailed    RunStatus = "failed"
)

// Run is one Claude Code run of an agent over new messages in a chat.
type Run struct {
	ID         string    `json:"id"`
	At         time.Time `json:"at"`
	AgentID    string    `json:"agentId"`
	ChatJID    string    `json:"chatJid"`
	MessageIDs []string  `json:"messageIds"`
	Status     RunStatus `json:"status"`
	Reply      string    `json:"reply"`
	Images     []string  `json:"images"` // paths relative to the agent's repo
	Voice      bool      `json:"voice"`  // send the reply as a voice note
	Note       string    `json:"note"`   // why it stayed quiet, or what it learned
	// Request is what the owner asked the agent to do directly (from Claude Code), "" for a normal run.
	Request string `json:"request,omitempty"`
	// Review holds a requested reply for the owner's OK instead of sending it at once.
	Review bool      `json:"review,omitempty"`
	Commit string    `json:"commit"`
	Files  []string  `json:"files"`
	Error  string    `json:"error"`
	SentAt time.Time `json:"sentAt"`
	// DismissedAt is when the owner cleared this run from "needs you" lists on the board.
	DismissedAt time.Time `json:"dismissedAt"`
}
