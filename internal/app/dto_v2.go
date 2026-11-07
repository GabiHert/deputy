package app

// Board and Claude chat DTOs (v2). Times are Unix milliseconds.

// Events added in v2.
const (
	EventBoard     = "board"     // no payload: call GetBoard (layout, data or a Claude block changed)
	EventAssistant = "assistant" // string chat ID: call AssistantMessages / ListAssistantChats
)

// BoardSourceDTO names the read-only query that feeds a block.
type BoardSourceDTO struct {
	Query  string         `json:"query"`  // e.g. "runs.count"
	Params map[string]any `json:"params"` // e.g. {"since": "7d", "by": "agent"}
}

// BoardColumnDTO is one column of a table block.
type BoardColumnDTO struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Numeric bool   `json:"numeric"`
}

// QueryResultDTO is what a query returns. Shape says how to read Items:
//   - "groups":   [{key, label, value}]                 (runs.count, media.usage)
//   - "runs":     [RunDTO]                              (runs.list)
//   - "messages": [{chatJid, chatName, ...MessageDTO}]  (messages.list, messages.search)
//   - "cells":    [{day, hour, count}] day 0=Sunday     (messages.byHour)
//   - "rows":     [object]                              (chats.list, agents.list, knowledge.changes)
type QueryResultDTO struct {
	Query string `json:"query"`
	Shape string `json:"shape"`
	Items any    `json:"items"`
}

// FormDTO is a form block or an ask card.
type FormDTO struct {
	Title     string        `json:"title"`
	Questions []QuestionDTO `json:"questions"`
}

// QuestionDTO is one question of a form.
type QuestionDTO struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Kind        string   `json:"kind"` // "choice" | "multi" | "input" | "confirm"
	Options     []string `json:"options"`
	Placeholder string   `json:"placeholder"`
}

// BoardBlockDTO is one block on the board, with its data resolved by the board engine.
//
// Types: "stats" and "bars" (groups), "replies" and "runs" (runs), "messages" (messages),
// "table" (any shape, with columns), "heatmap" (cells), "prose" (static markdown Text),
// "claude" (markdown Text written by Claude on Schedule), "agent" (AgentID),
// "form" (Form; answers go to the board chat), "custom" (HTML/JS in a sandboxed frame).
type BoardBlockDTO struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Title    string           `json:"title"`
	Span     int              `json:"span"` // 1 or 2 columns
	Source   *BoardSourceDTO  `json:"source"`
	Columns  []BoardColumnDTO `json:"columns"`
	Text     string           `json:"text"`
	HTML     string           `json:"html"`
	JS       string           `json:"js"`
	Height   int              `json:"height"`
	AgentID  string           `json:"agentId"`
	Prompt   string           `json:"prompt"`   // claude blocks: what to write
	Schedule string           `json:"schedule"` // claude blocks: "daily 08:00" | "hourly" | "manual"
	Form     *FormDTO         `json:"form"`

	Data       *QueryResultDTO `json:"data"`       // resolved Source, nil without one
	Agent      *AgentDTO       `json:"agent"`      // resolved AgentID
	Error      string          `json:"error"`      // the query or Claude failed
	TextAt     int64           `json:"textAt"`     // when a claude block was written
	Refreshing bool            `json:"refreshing"` // a claude block is being written now
}

// BoardDTO is the whole board.
type BoardDTO struct {
	Blocks   []BoardBlockDTO `json:"blocks"`
	LayoutAt int64           `json:"layoutAt"` // when Claude last changed the layout, 0 = the default board
	ChatID   string          `json:"chatId"`   // the board's own Claude chat
	Building bool            `json:"building"` // Claude is changing the layout now
}

// AssistantChatDTO is one saved Claude chat.
type AssistantChatDTO struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Purpose   string `json:"purpose"` // "general" | "new-agent" | "agent" | "board"
	AgentID   string `json:"agentId"` // for "agent" and "ask" chats
	ChatJID   string `json:"chatJid"` // for "ask" chats: the WhatsApp chat it was opened from
	Preview   string `json:"preview"` // last line, for the list
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
	Busy      bool   `json:"busy"` // Claude is answering
}

// AssistantMessageDTO is one item in a Claude chat.
type AssistantMessageDTO struct {
	ID     string   `json:"id"`
	ChatID string   `json:"chatId"`
	Role   string   `json:"role"` // "user" | "assistant" | "progress" | "card" | "note" | "error"
	Text   string   `json:"text"`
	At     int64    `json:"at"`
	Card   *CardDTO `json:"card"`
}

// CardDTO is an interactive card Claude put in the chat. Only the person's click acts on it.
type CardDTO struct {
	Kind    string              `json:"kind"`   // "ask" | "agent" | "change" | "message"
	Status  string              `json:"status"` // "open" | "done" | "dismissed"
	Result  string              `json:"result"` // what happened, once done
	Ask     *FormDTO            `json:"ask"`
	Answers map[string]string   `json:"answers"`
	Agent   *AgentProposalDTO   `json:"agent"`
	Change  *ChangeProposalDTO  `json:"change"`
	Message *MessageProposalDTO `json:"message"`
}

// ChatRefDTO names a WhatsApp chat.
type ChatRefDTO struct {
	JID     string `json:"jid"`
	Name    string `json:"name"`
	IsGroup bool   `json:"isGroup"`
	Members int    `json:"members"`
}

// FileDTO is a file Claude wants to write in an agent's repo.
type FileDTO struct {
	Path    string `json:"path"` // relative to the repo, under knowledge/
	Content string `json:"content"`
	Summary string `json:"summary"` // one line for the card
}

// AgentProposalDTO is a whole agent proposed by Claude, before it exists.
type AgentProposalDTO struct {
	Name            string       `json:"name"`
	Chats           []ChatRefDTO `json:"chats"`
	Mode            string       `json:"mode"`
	Trigger         string       `json:"trigger"`
	MaxPerHour      int          `json:"maxPerHour"`
	ReadImages      bool         `json:"readImages"`
	TranscribeVoice bool         `json:"transcribeVoice"`
	VoiceReplies    bool         `json:"voiceReplies"`
	SendImages      bool         `json:"sendImages"`
	Language        string       `json:"language"`
	RepoPath        string       `json:"repoPath"`
	Instructions    string       `json:"instructions"`
	Knowledge       []FileDTO    `json:"knowledge"`
	CreatedAgentID  string       `json:"createdAgentId"` // set once created
}

// SettingChangeDTO is one setting a change proposal flips.
type SettingChangeDTO struct {
	Field string `json:"field"` // AgentDTO json name, e.g. "mode"
	Label string `json:"label"`
	From  string `json:"from"`
	To    string `json:"to"`
	Value string `json:"value"` // the raw new value Deputy applies, like "auto" or "true"
}

// ChangeProposalDTO is a change to an existing agent.
type ChangeProposalDTO struct {
	AgentID         string             `json:"agentId"`
	AgentName       string             `json:"agentName"`
	Summary         string             `json:"summary"`
	OldInstructions string             `json:"oldInstructions"`
	Instructions    string             `json:"instructions"` // "" = unchanged
	Knowledge       []FileDTO          `json:"knowledge"`
	Settings        []SettingChangeDTO `json:"settings"`
	Commit          string             `json:"commit"` // set once applied
}

// MessageProposalDTO is a WhatsApp message Claude drafted; it is sent only when the person clicks Send.
type MessageProposalDTO struct {
	ChatJID   string `json:"chatJid"`
	ChatName  string `json:"chatName"`
	AgentID   string `json:"agentId"` // "" = sent as the person, with no agent prefix
	AgentName string `json:"agentName"`
	Prefix    string `json:"prefix"` // the agent's prefix, added when sent
	Text      string `json:"text"`
	Reason    string `json:"reason"`
}
