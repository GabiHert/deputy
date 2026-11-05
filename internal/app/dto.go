package app

// DTOs are what the frontend sees. Times are Unix milliseconds (0 = never).

// WhatsAppDTO is the WhatsApp connection state.
type WhatsAppDTO struct {
	// State is "starting", "qr" (waiting for a scan), "pairing" (scanned, syncing),
	// "connected", "disconnected" (linked but offline, reconnecting) or "error".
	State       string `json:"state"`
	QR          string `json:"qr"`          // PNG data URL while State is "qr"
	QRExpiresAt int64  `json:"qrExpiresAt"` // when the code is replaced
	Phone       string `json:"phone"`       // "+55 51 9•••• 4412" once linked
	PushName    string `json:"pushName"`
	Since       int64  `json:"since"`
	Error       string `json:"error"`
}

// ClaudeDTO is the state of the person's Claude Code.
type ClaudeDTO struct {
	Found   bool   `json:"found"`
	Path    string `json:"path"`
	Version string `json:"version"`
	Error   string `json:"error"`
}

// StatusDTO is the app's overall state.
type StatusDTO struct {
	WhatsApp WhatsAppDTO `json:"whatsapp"`
	Claude   ClaudeDTO   `json:"claude"`
	Version  string      `json:"version"`
	Waiting  int         `json:"waiting"` // replies waiting for approval
}

// ChatDTO is one row of the Chats view.
type ChatDTO struct {
	JID       string `json:"jid"`
	Name      string `json:"name"`
	IsGroup   bool   `json:"isGroup"`
	Members   int    `json:"members"`
	LastText  string `json:"lastText"`
	LastAt    int64  `json:"lastAt"`
	AgentID   string `json:"agentId"` // "" = no agent
	AgentName string `json:"agentName"`
	Mode      string `json:"mode"` // the agent's reply mode, "" without an agent
}

// RepoStatsDTO summarizes an agent's knowledge repo.
type RepoStatsDTO struct {
	Exists      bool   `json:"exists"`
	Files       int    `json:"files"` // files under knowledge/
	Commits     int    `json:"commits"`
	LastLearned int64  `json:"lastLearned"`
	Remote      string `json:"remote"` // e.g. "github.com/GabiHert/checagem-kb", "" without one
}

// AgentDTO is an agent as edited in the Agent view.
type AgentDTO struct {
	ID              string       `json:"id"` // "" for a new agent
	Name            string       `json:"name"`
	Enabled         bool         `json:"enabled"`
	RepoPath        string       `json:"repoPath"`
	Instructions    string       `json:"instructions"` // INSTRUCTIONS.md in the repo
	Chats           []string     `json:"chats"`        // chat JIDs
	Mode            string       `json:"mode"`         // "auto" | "approve" | "never"
	Trigger         string       `json:"trigger"`      // "all" | "mention"
	WaitSeconds     int          `json:"waitSeconds"`
	MaxPerHour      int          `json:"maxPerHour"`
	Prefix          string       `json:"prefix"`
	Push            bool         `json:"push"`
	ReadImages      bool         `json:"readImages"`
	TranscribeVoice bool         `json:"transcribeVoice"`
	SendImages      bool         `json:"sendImages"`
	VoiceReplies    bool         `json:"voiceReplies"`
	Web             bool         `json:"web"`
	Bash            bool         `json:"bash"`
	Model           string       `json:"model"`    // "" | "sonnet" | "opus" | "haiku"
	Language        string       `json:"language"` // "pt-BR" | "en": voice notes and spoken replies
	Stats           RepoStatsDTO `json:"stats"`
}

// MessageDTO is a WhatsApp message shown in an activity card.
type MessageDTO struct {
	ID         string `json:"id"`
	At         int64  `json:"at"`
	SenderName string `json:"senderName"`
	FromMe     bool   `json:"fromMe"`
	Kind       string `json:"kind"` // "text" | "image" | "voice" | "audio" | "video" | "document" | "sticker" | "other"
	Text       string `json:"text"`
	MediaURL   string `json:"mediaUrl"` // served by the app, "" without media
	Seconds    int    `json:"seconds"`
	Transcript string `json:"transcript"`
	MentionsMe bool   `json:"mentionsMe"`
}

// AttachmentDTO is an image the agent sent or wants to send.
type AttachmentDTO struct {
	Path  string `json:"path"` // relative to the agent's repo
	URL   string `json:"url"`
	Bytes int64  `json:"bytes"`
}

// RunDTO is one card in Activity.
type RunDTO struct {
	ID        string          `json:"id"`
	At        int64           `json:"at"`
	AgentID   string          `json:"agentId"`
	AgentName string          `json:"agentName"`
	ChatJID   string          `json:"chatJid"`
	ChatName  string          `json:"chatName"`
	IsGroup   bool            `json:"isGroup"`
	Status    string          `json:"status"` // "running" | "sent" | "waiting" | "discarded" | "quiet" | "learned" | "failed"
	Incoming  []MessageDTO    `json:"incoming"`
	Reply     string          `json:"reply"`
	Images    []AttachmentDTO `json:"images"`
	Voice     bool            `json:"voice"`
	Note      string          `json:"note"`
	Commit    string          `json:"commit"`
	Files     []string        `json:"files"`
	Error     string          `json:"error"`
	SentAt    int64           `json:"sentAt"`
}

// StatsDTO is the Today panel in Activity.
type StatsDTO struct {
	Sent       int `json:"sent"`
	Waiting    int `json:"waiting"`
	Quiet      int `json:"quiet"`
	Learned    int `json:"learned"` // runs that committed knowledge
	Failed     int `json:"failed"`
	Photos     int `json:"photos"` // photos agents looked at
	Voice      int `json:"voice"`  // voice notes transcribed
	ImagesSent int `json:"imagesSent"`
}

// DefaultsDTO are the settings a new agent starts with.
type DefaultsDTO struct {
	Mode        string `json:"mode"`
	MaxPerHour  int    `json:"maxPerHour"`
	WaitSeconds int    `json:"waitSeconds"`
	Prefix      string `json:"prefix"`
}

// SettingsDTO is the Settings view.
type SettingsDTO struct {
	Defaults           DefaultsDTO `json:"defaults"`
	MediaRetentionDays int         `json:"mediaRetentionDays"` // 0 keeps media
	OpenAtLogin        bool        `json:"openAtLogin"`
	Notify             bool        `json:"notify"`
	MediaDir           string      `json:"mediaDir"`   // read only
	MediaBytes         int64       `json:"mediaBytes"` // read only
	MediaFiles         int         `json:"mediaFiles"` // read only
}

// SpeechModelDTO is one transcription language in Settings.
type SpeechModelDTO struct {
	Language    string `json:"language"` // "pt-BR" | "en"
	Label       string `json:"label"`
	Installed   bool   `json:"installed"`
	SizeBytes   int64  `json:"sizeBytes"` // download size when not installed
	Downloading bool   `json:"downloading"`
	Done        int64  `json:"done"`
	Total       int64  `json:"total"`
	Phase       string `json:"phase"` // "download" | "verify" | "extract" | ""
	Error       string `json:"error"`
}
