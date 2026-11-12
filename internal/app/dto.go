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
	Mode      string `json:"mode"`    // the agent's reply mode, "" without an agent
	ViaAll    bool   `json:"viaAll"`  // the agent has it through "all direct chats"
	Skipped   bool   `json:"skipped"` // a direct chat the all-direct agent was told to leave alone
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
	AllDirect       bool         `json:"allDirect"`    // also every direct chat no other agent has
	Except          []string     `json:"except"`       // direct chats AllDirect leaves alone
	AnswerMe        bool         `json:"answerMe"`     // the person's own messages start runs too
	MCP             []string     `json:"mcp"`          // MCP servers it may use, by claude mcp list name
	FolderAccess    string       `json:"folderAccess"` // "" | "read" | "edit"
	Folders         []string     `json:"folders"`      // folders on the Mac beyond its repo
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
	// Week is what the agent's runs used in the last 7 days. It is read-only.
	Week UsageTotalDTO `json:"week"`
}

// UsageDTO is what one run used. CostUSD is at API list prices; a subscription is not billed per run.
type UsageDTO struct {
	Model      string  `json:"model"`
	Input      int64   `json:"input"`
	CacheRead  int64   `json:"cacheRead"`
	CacheWrite int64   `json:"cacheWrite"`
	Output     int64   `json:"output"`
	Tokens     int64   `json:"tokens"` // all of the above
	CostUSD    float64 `json:"costUsd"`
	Turns      int     `json:"turns"`
	DurationMs int64   `json:"durationMs"`
}

// UsageTotalDTO adds up the usage of an agent's runs.
type UsageTotalDTO struct {
	Runs     int      `json:"runs"`
	Measured int      `json:"measured"` // runs that carry usage; older runs have none
	Usage    UsageDTO `json:"usage"`
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
	Request   string          `json:"request"` // what the owner asked the agent from Claude Code, "" for a normal run
	Commit    string          `json:"commit"`
	Files     []string        `json:"files"`
	Error     string          `json:"error"`
	SentAt    int64           `json:"sentAt"`
	// AnsweredAt is when the owner or an agent last wrote in this chat after the run; 0 if not yet.
	AnsweredAt int64 `json:"answeredAt"`
	// DismissedAt is when the owner dismissed it from "needs you" lists; 0 if not.
	DismissedAt int64 `json:"dismissedAt"`
	// Usage is what the Claude Code run used; nil when it was not recorded.
	Usage *UsageDTO `json:"usage"`
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
	MCPEnabled         bool        `json:"mcpEnabled"` // Claude Code in other projects can use Positron
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

// UpdateReleaseDTO is the newest release on GitHub.
type UpdateReleaseDTO struct {
	Tag   string `json:"tag"`
	Name  string `json:"name"`
	Notes string `json:"notes"`
	Date  int64  `json:"date"`
	URL   string `json:"url"`
}

// UpdateStatusDTO is the Updates card's state.
type UpdateStatusDTO struct {
	Version  string            `json:"version"`
	Repo     string            `json:"repo"`
	Latest   *UpdateReleaseDTO `json:"latest"` // null until a check succeeds
	UpToDate bool              `json:"upToDate"`
	HasToken bool              `json:"hasToken"`
	// State is "idle", "checking", "downloading", "verifying", "installing" or "failed".
	State        string   `json:"state"`
	Message      string   `json:"message"`
	Lines        []string `json:"lines"`
	Seq          int      `json:"seq"`
	InstalledApp string   `json:"installedApp"` // "" when Positron does not run from an app bundle
}

// MCPStatusDTO is the "Claude Code in other projects" card.
type MCPStatusDTO struct {
	Enabled bool   `json:"enabled"`
	Running bool   `json:"running"`
	URL     string `json:"url"`
	Error   string `json:"error"`
	// Command adds Positron to Claude Code by hand. It holds the token: show it only when asked.
	Command string `json:"command"`
}

// MCPServerDTO is one of the person's Claude Code MCP servers.
type MCPServerDTO struct {
	Name   string `json:"name"`
	Target string `json:"target"`
	Status string `json:"status"` // connected | failed | needs auth | pending | other text
}
