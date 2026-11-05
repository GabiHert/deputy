// Package domain holds Deputy's core types.
package domain

import "time"

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
