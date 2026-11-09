// Package domain holds Positron's core types.
package domain

import (
	"strings"
	"time"
)

// ReplyMode says what happens to a reply the agent writes.
type ReplyMode string

const (
	ModeAuto    ReplyMode = "auto"    // sent at once
	ModeApprove ReplyMode = "approve" // waits in Activity for the person
	ModeNever   ReplyMode = "never"   // never sent; the agent only learns
)

// Trigger says which messages start a run.
type Trigger string

const (
	TriggerAll     Trigger = "all"     // every message
	TriggerMention Trigger = "mention" // mentions of the person, or replies to them; every message in a direct chat
)

// Agent is a Claude Code agent bound to a knowledge repo and some chats.
type Agent struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Enabled         bool      `json:"enabled"`
	RepoPath        string    `json:"repoPath"`
	Chats           []string  `json:"chats"`
	AllDirect       bool      `json:"allDirect"`    // also handles every direct chat no other agent has
	Except          []string  `json:"except"`       // direct chats AllDirect leaves alone
	AnswerMe        bool      `json:"answerMe"`     // the person's own messages start runs too
	MCP             []string  `json:"mcp"`          // the person's MCP servers (claude mcp list names) it may use
	FolderAccess    string    `json:"folderAccess"` // "" (only its repo) | "read" | "edit": what it may do in Folders
	Folders         []string  `json:"folders"`      // folders on the Mac beyond its repo, e.g. a project's code
	Mode            ReplyMode `json:"mode"`
	Trigger         Trigger   `json:"trigger"`
	WaitSeconds     int       `json:"waitSeconds"`
	MaxPerHour      int       `json:"maxPerHour"`
	Prefix          string    `json:"prefix"`
	Push            bool      `json:"push"`
	ReadImages      bool      `json:"readImages"`
	TranscribeVoice bool      `json:"transcribeVoice"`
	SendImages      bool      `json:"sendImages"`
	VoiceReplies    bool      `json:"voiceReplies"`
	Web             bool      `json:"web"`
	Bash            bool      `json:"bash"`
	Model           string    `json:"model"` // "" uses the person's Claude Code default
	Language        Language  `json:"language"`
	CreatedAt       time.Time `json:"createdAt"`
}

// HasChat reports whether the agent handles jid.
func (a Agent) HasChat(jid string) bool {
	for _, c := range a.Chats {
		if c == jid {
			return true
		}
	}
	return false
}

// Skips reports whether jid is one of the direct chats the agent leaves alone.
func (a Agent) Skips(jid string) bool {
	for _, c := range a.Except {
		if c == jid {
			return true
		}
	}
	return false
}

// IsDirect reports whether jid is a one-to-one chat, not a group, broadcast or channel.
func IsDirect(jid string) bool {
	return strings.HasSuffix(jid, "@s.whatsapp.net") || strings.HasSuffix(jid, "@lid")
}

// Router finds the agent that handles a chat. An agent picked for the chat wins;
// otherwise a direct chat goes to the agent set to all direct chats, unless it skips it.
type Router struct {
	picked map[string]Agent
	direct *Agent
}

// NewRouter builds a Router over agents.
func NewRouter(agents []Agent) Router {
	r := Router{picked: map[string]Agent{}}
	for i, a := range agents {
		for _, c := range a.Chats {
			r.picked[c] = a
		}
		if a.AllDirect && r.direct == nil {
			r.direct = &agents[i]
		}
	}
	return r
}

// Owner returns the agent for jid and whether it came from "all direct chats".
func (r Router) Owner(jid string) (a Agent, viaAll bool, ok bool) {
	if a, ok := r.picked[jid]; ok {
		return a, false, true
	}
	if r.direct != nil && IsDirect(jid) && !r.direct.Skips(jid) {
		return *r.direct, true, true
	}
	return Agent{}, false, false
}

// AllDirect returns the agent set to all direct chats, if any.
func (r Router) AllDirect() (Agent, bool) {
	if r.direct == nil {
		return Agent{}, false
	}
	return *r.direct, true
}

// AgentDefaults are the settings a new agent starts with.
type AgentDefaults struct {
	Mode        ReplyMode `json:"mode"`
	MaxPerHour  int       `json:"maxPerHour"`
	WaitSeconds int       `json:"waitSeconds"`
	Prefix      string    `json:"prefix"`
}

// Settings are the app-wide settings.
type Settings struct {
	Defaults           AgentDefaults `json:"defaults"`
	MediaRetentionDays int           `json:"mediaRetentionDays"` // 0 keeps media forever
	OpenAtLogin        bool          `json:"openAtLogin"`
	Notify             bool          `json:"notify"`
	MCPEnabled         bool          `json:"mcpEnabled"` // Claude Code in other projects can use Positron
}

// DefaultSettings are the settings of a fresh install.
func DefaultSettings() Settings {
	return Settings{
		Defaults:           AgentDefaults{Mode: ModeApprove, MaxPerHour: 20, WaitSeconds: 8, Prefix: "🤖"},
		MediaRetentionDays: 30,
		Notify:             true,
	}
}

// NewAgent returns an agent with the defaults applied.
func NewAgent(d AgentDefaults) Agent {
	return Agent{
		Enabled: true, Mode: d.Mode, Trigger: TriggerMention, WaitSeconds: d.WaitSeconds, MaxPerHour: d.MaxPerHour,
		Prefix: d.Prefix, ReadImages: true, TranscribeVoice: true, SendImages: true, Web: true, Language: LangPTBR,
		Chats: []string{},
	}
}
