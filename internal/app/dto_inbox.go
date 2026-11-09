package app

// The inbox: Positron as a full WhatsApp client for the owner. Times are Unix milliseconds.

// Inbox events.
const (
	// EventMessage carries MessageEventDTO: a message arrived, was sent, or changed (reaction,
	// edit, deletion, status, media downloaded or transcribed).
	EventMessage = "wa:message"
	// EventUnread carries UnreadDTO: a chat's unread count (and the total) changed.
	EventUnread = "wa:unread"
	// EventReceipt carries ReceiptEventDTO: the owner's messages were delivered, read or played.
	EventReceipt = "wa:receipt"
	// EventPresence carries PresenceDTO: someone is typing or recording, or went online or offline.
	EventPresence = "wa:presence"
	// EventHistory carries HistoryEventDTO: older messages arrived for a chat (after LoadOlder or a
	// sync from the phone), or LoadOlder got no answer.
	EventHistory = "wa:history"
	// EventOpenChat carries a chat JID: the owner clicked a notification; show that chat.
	EventOpenChat = "open-chat"
)

// InboxChatDTO is one row of the inbox. The list also changes on EventChats ("chats").
type InboxChatDTO struct {
	JID      string `json:"jid"`
	Name     string `json:"name"`
	Initials string `json:"initials"` // one or two letters for an avatar placeholder
	IsGroup  bool   `json:"isGroup"`
	Members  int    `json:"members"`
	// AvatarURL is the cached picture ("" until GetAvatar fetched one, or when there is none).
	AvatarURL  string `json:"avatarUrl"`
	LastAt     int64  `json:"lastAt"`
	LastID     string `json:"lastId"`
	LastText   string `json:"lastText"`   // preview of the last message, without the sender
	LastKind   string `json:"lastKind"`   // "text" | "image" | "voice" | ... ("" when unknown)
	LastSender string `json:"lastSender"` // who sent it in a group, "" for the owner
	LastFromMe bool   `json:"lastFromMe"`
	LastStatus string `json:"lastStatus"` // the last message's status when it is the owner's
	Unread     int    `json:"unread"`
	Muted      bool   `json:"muted"`
	MutedUntil int64  `json:"mutedUntil"` // 0 = not muted; far future = forever
	Pinned     bool   `json:"pinned"`
	Archived   bool   `json:"archived"`
	AgentID    string `json:"agentId"` // the agent that answers this chat, "" for none
	AgentName  string `json:"agentName"`
	// Disappearing is the chat's disappearing-messages timer in seconds (86400 = 24 hours), 0 when
	// off or not known yet. Positron's messages carry it, as the phone's do.
	Disappearing int `json:"disappearing"`
}

// QuotedDTO is the message a message replies to.
type QuotedDTO struct {
	ID         string `json:"id"`
	SenderJID  string `json:"senderJid"`
	SenderName string `json:"senderName"` // "You" for the owner
	FromMe     bool   `json:"fromMe"`
	Kind       string `json:"kind"`
	Snippet    string `json:"snippet"`
	Found      bool   `json:"found"` // the quoted message is stored, so the UI can jump to it
}

// ReactionDTO is one emoji on a message and who put it there.
type ReactionDTO struct {
	Emoji   string   `json:"emoji"`
	Count   int      `json:"count"`
	Senders []string `json:"senders"` // names; "You" for the owner
	FromMe  bool     `json:"fromMe"`  // the owner reacted with this emoji
}

// MediaDTO is the file of a photo, video, voice note, audio, document or sticker.
type MediaDTO struct {
	Mime     string `json:"mime"`
	Seconds  int    `json:"seconds"`
	FileName string `json:"fileName"`
	Size     int64  `json:"size"`
	// URL serves the downloaded file ("" until GetMedia downloaded it).
	URL string `json:"url"`
	// ThumbURL serves a small JPEG preview WhatsApp sent with photos, videos and documents ("" without).
	ThumbURL string `json:"thumbUrl"`
	// Downloadable is true when GetMedia can still fetch the file from WhatsApp.
	Downloadable bool `json:"downloadable"`
}

// ChatMessageDTO is one message in a chat.
type ChatMessageDTO struct {
	ID         string        `json:"id"`
	ChatJID    string        `json:"chatJid"`
	SenderJID  string        `json:"senderJid"`
	SenderName string        `json:"senderName"` // "" for the owner
	FromMe     bool          `json:"fromMe"`
	FromAgent  bool          `json:"fromAgent"` // sent by one of Positron's agents
	AgentName  string        `json:"agentName"`
	FromUI     bool          `json:"fromUi"` // typed by the owner in Positron
	At         int64         `json:"at"`
	Kind       string        `json:"kind"` // "text" | "image" | "voice" | "audio" | "video" | "document" | "sticker" | "other"
	Text       string        `json:"text"` // body or caption ("" when deleted)
	Transcript string        `json:"transcript"`
	Mentions   []string      `json:"mentions"` // mentioned JIDs
	MentionsMe bool          `json:"mentionsMe"`
	Quoted     *QuotedDTO    `json:"quoted"` // null when it is not a reply
	Reactions  []ReactionDTO `json:"reactions"`
	Media      *MediaDTO     `json:"media"` // null for text
	// Status is how far the owner's message got: "pending" | "sent" | "delivered" | "read" |
	// "played" | "failed"; "" for others' messages. In groups it shows the furthest receipt.
	Status  string `json:"status"`
	Edited  bool   `json:"edited"`
	Revoked bool   `json:"revoked"` // deleted for everyone
	History bool   `json:"history"` // came from the phone's history, not live
	// ViewOnce is a view-once photo, video or voice note. Media.Downloadable says whether Positron
	// can show it (once, on request); otherwise it is only on the phone.
	ViewOnce bool `json:"viewOnce"`
}

// MessagePageDTO is one page of a chat, newest first. Pass Cursor back as before for the next page.
type MessagePageDTO struct {
	Messages []ChatMessageDTO `json:"messages"`
	Cursor   string           `json:"cursor"`  // "" when Positron has no older messages stored
	HasMore  bool             `json:"hasMore"` // false: call LoadOlder to ask the phone for more
}

// SearchHitDTO is a message that matched a search.
type SearchHitDTO struct {
	ChatJID  string         `json:"chatJid"`
	ChatName string         `json:"chatName"`
	IsGroup  bool           `json:"isGroup"`
	Message  ChatMessageDTO `json:"message"`
}

// ContactDTO is someone to start a chat with.
type ContactDTO struct {
	JID      string `json:"jid"`
	Name     string `json:"name"` // the best name to show
	PushName string `json:"pushName"`
	Phone    string `json:"phone"` // "+5551999991234", "" when unknown
	Initials string `json:"initials"`
	IsGroup  bool   `json:"isGroup"`
	HasChat  bool   `json:"hasChat"` // Positron already has this chat
}

// SendTextDTO is a text message from the owner.
type SendTextDTO struct {
	Chat     string   `json:"chat"`
	Text     string   `json:"text"`
	QuotedID string   `json:"quotedId"` // reply to this message, "" for none
	Mentions []string `json:"mentions"` // JIDs mentioned with @ in the text, optional
}

// SendFileDTO is a photo, video, audio file or document from the owner. Give either Path (a file
// the owner picked, e.g. with ChooseFiles) or DataBase64 (dropped or pasted bytes, with FileName).
type SendFileDTO struct {
	Chat       string `json:"chat"`
	Path       string `json:"path"`
	DataBase64 string `json:"dataBase64"`
	FileName   string `json:"fileName"`
	Mime       string `json:"mime"`    // "" = guess from the name and content
	Caption    string `json:"caption"` // photos, videos and documents
	QuotedID   string `json:"quotedId"`
	// Kind forces how it is sent: "image" | "video" | "audio" | "document"; "" picks from the type.
	Kind string `json:"kind"`
}

// SendVoiceDTO is a voice note the owner recorded in the window (any format the webview records;
// Positron converts it to Ogg Opus with ffmpeg when needed).
type SendVoiceDTO struct {
	Chat       string `json:"chat"`
	DataBase64 string `json:"dataBase64"`
	Mime       string `json:"mime"`    // e.g. "audio/webm;codecs=opus", "audio/mp4"
	Seconds    int    `json:"seconds"` // the recording's length, 0 = measure it
	QuotedID   string `json:"quotedId"`
}

// MessageEventDTO is the EventMessage payload.
type MessageEventDTO struct {
	Chat    string         `json:"chat"`
	Change  string         `json:"change"` // "new" | "update"
	Message ChatMessageDTO `json:"message"`
}

// UnreadDTO is the EventUnread payload.
type UnreadDTO struct {
	Chat   string `json:"chat"`   // "" when many chats changed: reload the inbox
	Unread int    `json:"unread"` // that chat's count
	Total  int    `json:"total"`  // all chats that are not muted: the Dock badge
}

// ReceiptEventDTO is the EventReceipt payload.
type ReceiptEventDTO struct {
	Chat   string   `json:"chat"`
	IDs    []string `json:"ids"`
	Status string   `json:"status"` // "delivered" | "read" | "played"
}

// PresenceDTO is the EventPresence payload.
type PresenceDTO struct {
	Chat     string `json:"chat"`     // the chat it shows in ("" for online/offline of a contact)
	JID      string `json:"jid"`      // who
	Name     string `json:"name"`     // their name, for "Ana is typing…" in groups
	State    string `json:"state"`    // "composing" | "recording" | "paused" | "online" | "offline"
	LastSeen int64  `json:"lastSeen"` // with "offline", when known
}

// HistoryEventDTO is the EventHistory payload.
type HistoryEventDTO struct {
	Chat  string `json:"chat"`
	Added int    `json:"added"` // messages stored
	Kind  string `json:"kind"`  // "on-demand" (after LoadOlder), "initial", "recent", "full"
	Error string `json:"error"` // "" or why no messages came, e.g. the phone did not answer
}

// MemberDTO is one participant of a group, for @mentions.
type MemberDTO struct {
	JID   string   `json:"jid"` // put in mentions, and "@"+its user part in the text
	Name  string   `json:"name"`
	Phone string   `json:"phone"` // "" when hidden
	Users []string `json:"users"` // every user part they go by, to show incoming @mentions by name
	Me    bool     `json:"me"`
	Admin bool     `json:"admin"`
}
