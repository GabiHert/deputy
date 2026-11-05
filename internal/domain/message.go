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
}

// Chat is a WhatsApp chat or group Deputy has seen.
type Chat struct {
	JID      string    `json:"jid"`
	Name     string    `json:"name"`
	IsGroup  bool      `json:"isGroup"`
	Members  int       `json:"members"`
	LastText string    `json:"lastText"`
	LastAt   time.Time `json:"lastAt"`
}

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
	Commit     string    `json:"commit"`
	Files      []string  `json:"files"`
	Error      string    `json:"error"`
	SentAt     time.Time `json:"sentAt"`
}
