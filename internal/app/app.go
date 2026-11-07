// Package app is the surface the Wails frontend calls.
package app

import (
	"context"
	"errors"
)

// Events the frontend listens to.
const (
	EventStatus   = "status"   // StatusDTO
	EventChats    = "chats"    // no payload: call ListChats
	EventAgents   = "agents"   // no payload: call ListAgents
	EventActivity = "activity" // RunDTO, a new or changed card
	EventSpeech   = "speech"   // []SpeechModelDTO
	EventNavigate = "navigate" // string view name, from the menu bar: "activity"

	EventUpdateOutput = "update:output" // {seq, line}: one line of the update's output
	EventUpdateState  = "update:state"  // {state, message}
	// EventUpdateAvailable says a newer release is out: {tag}.
	EventUpdateAvailable = "update:available"
)

// Backend does the work behind the App methods.
type Backend interface {
	Status() StatusDTO
	LinkWithPhone(ctx context.Context, phone string) (string, error)
	Logout(ctx context.Context) error
	ListChats() []ChatDTO
	AssignChat(jid, agentID string) error
	ListAgents() []AgentDTO
	GetAgent(id string) (AgentDTO, error)
	NewAgent() AgentDTO
	SaveAgent(a AgentDTO) (AgentDTO, error)
	DeleteAgent(id string) error
	OpenRepoInClaude(id string) error
	ListActivity(filter string) []RunDTO
	TodayStats() StatsDTO
	SendDraft(runID, text string) error
	DiscardDraft(runID string) error
	DismissRun(runID string, dismissed bool) error
	RetryRun(runID string) error
	GetSettings() SettingsDTO
	SaveSettings(s SettingsDTO) error
	ListSpeechModels() []SpeechModelDTO
	DownloadSpeechModel(lang string) error
	CancelSpeechDownload(lang string)
	ShowMediaFolder() error
	RecheckClaude() StatusDTO
	ListMCPServers(ctx context.Context, refresh bool) ([]MCPServerDTO, error)
	GetUpdateStatus() UpdateStatusDTO
	CheckForUpdate(ctx context.Context) (UpdateStatusDTO, error)
	SetUpdateToken(ctx context.Context, token string) (UpdateStatusDTO, error)
	StartUpdate() error

	// Claude Code in other projects (the MCP server)
	GetMCPStatus() MCPStatusDTO
	SetMCPEnabled(on bool) (MCPStatusDTO, error)
	AddMCPToClaude() (string, error)
	RotateMCPToken() (MCPStatusDTO, error)

	// v2: board
	GetBoard() BoardDTO
	RunBoardQuery(query string, params map[string]any) (QueryResultDTO, error)
	RefreshBoardBlock(id string) error
	ChangeBoard(text string) error
	ResetBoard() error

	// v2: Claude chats
	ListAssistantChats() []AssistantChatDTO
	NewAssistantChat(purpose, agentID string) (AssistantChatDTO, error)
	AskAgentChat(jid string) (AssistantChatDTO, error)
	AssistantMessages(chatID string) []AssistantMessageDTO
	SendAssistant(chatID, text string) error
	StopAssistant(chatID string)
	RenameAssistantChat(chatID, title string) error
	DeleteAssistantChat(chatID string) error
	AnswerCard(messageID string, answers map[string]string) error
	DismissCard(messageID string) error
	CreateAgentFromCard(messageID string, p AgentProposalDTO) (AgentDTO, error)
	ApplyChangeCard(messageID string) error
	SendMessageCard(messageID, text string) error

	// inbox: the owner's own WhatsApp client
	ListInbox(filter string) []InboxChatDTO
	GetInboxChat(jid string) (InboxChatDTO, error)
	ListMessages(chat, before string, limit int) (MessagePageDTO, error)
	GetMessage(chat, id string) (ChatMessageDTO, error)
	MarkChatRead(chat string) error
	SetActiveChat(jid string)
	GetUnreadTotal() int
	GetMedia(ctx context.Context, chat, id string) (string, error)
	GroupMembers(ctx context.Context, chat string) ([]MemberDTO, error)
	OpenMedia(ctx context.Context, chat, id string) error
	SendText(ctx context.Context, in SendTextDTO) (ChatMessageDTO, error)
	SendFile(ctx context.Context, in SendFileDTO) (ChatMessageDTO, error)
	SendVoice(ctx context.Context, in SendVoiceDTO) (ChatMessageDTO, error)
	React(ctx context.Context, chat, id, emoji string) (ChatMessageDTO, error)
	DeleteForEveryone(ctx context.Context, chat, id string) (ChatMessageDTO, error)
	EditMessage(ctx context.Context, chat, id, text string) (ChatMessageDTO, error)
	ListContacts(query string) []ContactDTO
	StartChat(ctx context.Context, phone string) (InboxChatDTO, error)
	SearchMessages(query, chat string, limit int) []SearchHitDTO
	LoadOlder(chat string) error
	SetTyping(chat string, on bool)
	GetAvatar(ctx context.Context, jid string) (string, error)
}

// App is bound to the frontend. Each method is one call from the UI.
type App struct {
	ctx         context.Context
	b           Backend
	chooseDir   func(ctx context.Context, title string) (string, error)
	chooseFiles func(ctx context.Context, title string) ([]string, error)
}

// SetFilePicker sets what ChooseFiles opens. It is a function, not a method, so it is not bound to the UI.
func SetFilePicker(a *App, f func(ctx context.Context, title string) ([]string, error)) {
	a.chooseFiles = f
}

// New binds b; chooseDir opens the folder picker. Call the returned function with the Wails context on startup.
func New(b Backend, chooseDir func(ctx context.Context, title string) (string, error)) (*App, func(context.Context)) {
	a := &App{b: b, chooseDir: chooseDir, ctx: context.Background()}
	return a, func(ctx context.Context) { a.ctx = ctx }
}

func (a *App) GetStatus() StatusDTO                       { return a.b.Status() }
func (a *App) LinkWithPhone(phone string) (string, error) { return a.b.LinkWithPhone(a.ctx, phone) }
func (a *App) Logout() error                              { return a.b.Logout(a.ctx) }
func (a *App) ListChats() []ChatDTO                       { return a.b.ListChats() }
func (a *App) AssignChat(jid, agentID string) error       { return a.b.AssignChat(jid, agentID) }
func (a *App) ListAgents() []AgentDTO                     { return a.b.ListAgents() }
func (a *App) GetAgent(id string) (AgentDTO, error)       { return a.b.GetAgent(id) }
func (a *App) NewAgent() AgentDTO                         { return a.b.NewAgent() }
func (a *App) SaveAgent(ag AgentDTO) (AgentDTO, error)    { return a.b.SaveAgent(ag) }
func (a *App) DeleteAgent(id string) error                { return a.b.DeleteAgent(id) }
func (a *App) OpenRepoInClaude(id string) error           { return a.b.OpenRepoInClaude(id) }
func (a *App) ListActivity(filter string) []RunDTO        { return a.b.ListActivity(filter) }
func (a *App) TodayStats() StatsDTO                       { return a.b.TodayStats() }
func (a *App) SendDraft(runID, text string) error         { return a.b.SendDraft(runID, text) }
func (a *App) DiscardDraft(runID string) error            { return a.b.DiscardDraft(runID) }
func (a *App) DismissRun(runID string, dismissed bool) error {
	return a.b.DismissRun(runID, dismissed)
}
func (a *App) RetryRun(runID string) error           { return a.b.RetryRun(runID) }
func (a *App) GetSettings() SettingsDTO              { return a.b.GetSettings() }
func (a *App) SaveSettings(s SettingsDTO) error      { return a.b.SaveSettings(s) }
func (a *App) ListSpeechModels() []SpeechModelDTO    { return a.b.ListSpeechModels() }
func (a *App) DownloadSpeechModel(lang string) error { return a.b.DownloadSpeechModel(lang) }
func (a *App) CancelSpeechDownload(lang string)      { a.b.CancelSpeechDownload(lang) }

// ListMCPServers lists the person's Claude Code MCP servers, for an agent's tool access.
func (a *App) ListMCPServers(refresh bool) ([]MCPServerDTO, error) {
	return a.b.ListMCPServers(a.ctx, refresh)
}

// GroupMembers lists a group's participants, for @mentions in the composer.
func (a *App) GroupMembers(chat string) ([]MemberDTO, error) { return a.b.GroupMembers(a.ctx, chat) }

func (a *App) ShowMediaFolder() error   { return a.b.ShowMediaFolder() }
func (a *App) RecheckClaude() StatusDTO { return a.b.RecheckClaude() }

// GetUpdateStatus returns the version, the last update check and the current update.
func (a *App) GetUpdateStatus() UpdateStatusDTO { return a.b.GetUpdateStatus() }

// CheckForUpdate asks GitHub for the newest release.
func (a *App) CheckForUpdate() (UpdateStatusDTO, error) { return a.b.CheckForUpdate(a.ctx) }

// StartUpdate downloads, checks and installs the newest release, then quits Deputy. Output arrives
// as EventUpdateOutput events and states as EventUpdateState events.
func (a *App) StartUpdate() error { return a.b.StartUpdate() }

// SetUpdateToken saves the GitHub token that reads a private repo's releases in the keychain;
// "" removes it.
func (a *App) SetUpdateToken(token string) (UpdateStatusDTO, error) {
	return a.b.SetUpdateToken(a.ctx, token)
}

// GetMCPStatus says whether Claude Code in other projects can reach Deputy.
func (a *App) GetMCPStatus() MCPStatusDTO { return a.b.GetMCPStatus() }

// SetMCPEnabled turns the server for Claude Code in other projects on or off.
func (a *App) SetMCPEnabled(on bool) (MCPStatusDTO, error) { return a.b.SetMCPEnabled(on) }

// AddMCPToClaude runs claude mcp add for the person and returns its output.
func (a *App) AddMCPToClaude() (string, error) { return a.b.AddMCPToClaude() }

// RotateMCPToken makes a new token; Claude Code needs to be given it again.
func (a *App) RotateMCPToken() (MCPStatusDTO, error) { return a.b.RotateMCPToken() }

func (a *App) GetBoard() BoardDTO { return a.b.GetBoard() }
func (a *App) RunBoardQuery(query string, params map[string]any) (QueryResultDTO, error) {
	return a.b.RunBoardQuery(query, params)
}
func (a *App) RefreshBoardBlock(id string) error      { return a.b.RefreshBoardBlock(id) }
func (a *App) ChangeBoard(text string) error          { return a.b.ChangeBoard(text) }
func (a *App) ResetBoard() error                      { return a.b.ResetBoard() }
func (a *App) ListAssistantChats() []AssistantChatDTO { return a.b.ListAssistantChats() }

// AskAgentChat opens a private Claude chat with the agent of WhatsApp chat jid, about that chat.
func (a *App) AskAgentChat(jid string) (AssistantChatDTO, error) { return a.b.AskAgentChat(jid) }

func (a *App) NewAssistantChat(purpose, agentID string) (AssistantChatDTO, error) {
	return a.b.NewAssistantChat(purpose, agentID)
}
func (a *App) AssistantMessages(chatID string) []AssistantMessageDTO {
	return a.b.AssistantMessages(chatID)
}
func (a *App) SendAssistant(chatID, text string) error { return a.b.SendAssistant(chatID, text) }
func (a *App) StopAssistant(chatID string)             { a.b.StopAssistant(chatID) }
func (a *App) RenameAssistantChat(chatID, title string) error {
	return a.b.RenameAssistantChat(chatID, title)
}
func (a *App) DeleteAssistantChat(chatID string) error { return a.b.DeleteAssistantChat(chatID) }
func (a *App) AnswerCard(messageID string, answers map[string]string) error {
	return a.b.AnswerCard(messageID, answers)
}
func (a *App) DismissCard(messageID string) error { return a.b.DismissCard(messageID) }
func (a *App) CreateAgentFromCard(messageID string, p AgentProposalDTO) (AgentDTO, error) {
	return a.b.CreateAgentFromCard(messageID, p)
}
func (a *App) ApplyChangeCard(messageID string) error { return a.b.ApplyChangeCard(messageID) }
func (a *App) SendMessageCard(messageID, text string) error {
	return a.b.SendMessageCard(messageID, text)
}

// ChooseFolder opens the folder picker and returns the chosen path, "" when cancelled.
func (a *App) ChooseFolder() (string, error) {
	if a.chooseDir == nil {
		return "", errors.New("the folder picker is not available")
	}
	return a.chooseDir(a.ctx, "Choose the agent's knowledge repo")
}

// ---- inbox ----

// ListInbox lists chats for the inbox, pinned first, then most recent: filter "" (not archived),
// "archived", "unread", "groups", "direct" or "all".
func (a *App) ListInbox(filter string) []InboxChatDTO { return a.b.ListInbox(filter) }

// GetInboxChat returns one inbox row.
func (a *App) GetInboxChat(jid string) (InboxChatDTO, error) { return a.b.GetInboxChat(jid) }

// ListMessages pages back through a chat, newest first: before "" for the newest page, then the
// returned cursor. limit 0 means 50.
func (a *App) ListMessages(chat, before string, limit int) (MessagePageDTO, error) {
	return a.b.ListMessages(chat, before, limit)
}

// GetMessage returns one stored message.
func (a *App) GetMessage(chat, id string) (ChatMessageDTO, error) { return a.b.GetMessage(chat, id) }

// MarkChatRead sends read receipts for the chat's unread messages and clears its unread count.
func (a *App) MarkChatRead(chat string) error { return a.b.MarkChatRead(chat) }

// SetActiveChat tells Deputy which chat is open and visible ("" when none, or the window is
// hidden): it gets no notifications, its contact's presence is followed and the owner shows online.
func (a *App) SetActiveChat(jid string) { a.b.SetActiveChat(jid) }

// GetUnreadTotal is the unread count of all chats that are not muted (the Dock badge).
func (a *App) GetUnreadTotal() int { return a.b.GetUnreadTotal() }

// GetMedia downloads a message's file if needed and returns its URL.
func (a *App) GetMedia(chat, id string) (string, error) { return a.b.GetMedia(a.ctx, chat, id) }

// OpenMedia downloads a message's file if needed and opens it in its macOS app.
func (a *App) OpenMedia(chat, id string) error { return a.b.OpenMedia(a.ctx, chat, id) }

// SendText sends a text message as the owner, optionally replying to a message.
func (a *App) SendText(in SendTextDTO) (ChatMessageDTO, error) { return a.b.SendText(a.ctx, in) }

// SendFile sends a photo, video, audio file or document as the owner.
func (a *App) SendFile(in SendFileDTO) (ChatMessageDTO, error) { return a.b.SendFile(a.ctx, in) }

// SendVoice sends a voice note the owner recorded.
func (a *App) SendVoice(in SendVoiceDTO) (ChatMessageDTO, error) { return a.b.SendVoice(a.ctx, in) }

// React puts the owner's emoji on a message; emoji "" takes the reaction away.
func (a *App) React(chat, id, emoji string) (ChatMessageDTO, error) {
	return a.b.React(a.ctx, chat, id, emoji)
}

// DeleteForEveryone deletes one of the owner's messages for everyone.
func (a *App) DeleteForEveryone(chat, id string) (ChatMessageDTO, error) {
	return a.b.DeleteForEveryone(a.ctx, chat, id)
}

// EditMessage changes the text of one of the owner's text messages (first 15 minutes only).
func (a *App) EditMessage(chat, id, text string) (ChatMessageDTO, error) {
	return a.b.EditMessage(a.ctx, chat, id, text)
}

// ListContacts finds people and groups to message by name or number; "" lists them all.
func (a *App) ListContacts(query string) []ContactDTO { return a.b.ListContacts(query) }

// StartChat checks a phone number is on WhatsApp and returns its (possibly new, empty) chat.
func (a *App) StartChat(phone string) (InboxChatDTO, error) { return a.b.StartChat(a.ctx, phone) }

// SearchMessages finds messages containing query, in one chat or ("" chat) all of them.
func (a *App) SearchMessages(query, chat string, limit int) []SearchHitDTO {
	return a.b.SearchMessages(query, chat, limit)
}

// LoadOlder asks the phone for messages older than the oldest stored one. It returns at once;
// EventHistory says when they arrived, or that the phone did not answer.
func (a *App) LoadOlder(chat string) error { return a.b.LoadOlder(chat) }

// SetTyping shows or clears "typing…" for the owner in a chat.
func (a *App) SetTyping(chat string, on bool) { a.b.SetTyping(chat, on) }

// GetAvatar returns the URL of a chat's or contact's picture, "" when there is none.
func (a *App) GetAvatar(jid string) (string, error) { return a.b.GetAvatar(a.ctx, jid) }

// ChooseFiles opens the file picker for attachments and returns the chosen paths (none when cancelled).
func (a *App) ChooseFiles() ([]string, error) {
	if a.chooseFiles == nil {
		return nil, errors.New("the file picker is not available")
	}
	return a.chooseFiles(a.ctx, "Choose files to send")
}
