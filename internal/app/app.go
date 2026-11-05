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
	RetryRun(runID string) error
	GetSettings() SettingsDTO
	SaveSettings(s SettingsDTO) error
	ListSpeechModels() []SpeechModelDTO
	DownloadSpeechModel(lang string) error
	CancelSpeechDownload(lang string)
	ShowMediaFolder() error
	RecheckClaude() StatusDTO

	// v2: board
	GetBoard() BoardDTO
	RunBoardQuery(query string, params map[string]any) (QueryResultDTO, error)
	RefreshBoardBlock(id string) error
	ChangeBoard(text string) error
	ResetBoard() error

	// v2: Claude chats
	ListAssistantChats() []AssistantChatDTO
	NewAssistantChat(purpose, agentID string) (AssistantChatDTO, error)
	AssistantMessages(chatID string) []AssistantMessageDTO
	SendAssistant(chatID, text string) error
	StopAssistant(chatID string)
	RenameAssistantChat(chatID, title string) error
	DeleteAssistantChat(chatID string) error
	AnswerCard(messageID string, answers map[string]string) error
	DismissCard(messageID string) error
	CreateAgentFromCard(messageID string, p AgentProposalDTO) (AgentDTO, error)
	ApplyChangeCard(messageID string) error
}

// App is bound to the frontend. Each method is one call from the UI.
type App struct {
	ctx       context.Context
	b         Backend
	chooseDir func(ctx context.Context, title string) (string, error)
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
func (a *App) RetryRun(runID string) error                { return a.b.RetryRun(runID) }
func (a *App) GetSettings() SettingsDTO                   { return a.b.GetSettings() }
func (a *App) SaveSettings(s SettingsDTO) error           { return a.b.SaveSettings(s) }
func (a *App) ListSpeechModels() []SpeechModelDTO         { return a.b.ListSpeechModels() }
func (a *App) DownloadSpeechModel(lang string) error      { return a.b.DownloadSpeechModel(lang) }
func (a *App) CancelSpeechDownload(lang string)           { a.b.CancelSpeechDownload(lang) }
func (a *App) ShowMediaFolder() error                     { return a.b.ShowMediaFolder() }
func (a *App) RecheckClaude() StatusDTO                   { return a.b.RecheckClaude() }

func (a *App) GetBoard() BoardDTO { return a.b.GetBoard() }
func (a *App) RunBoardQuery(query string, params map[string]any) (QueryResultDTO, error) {
	return a.b.RunBoardQuery(query, params)
}
func (a *App) RefreshBoardBlock(id string) error      { return a.b.RefreshBoardBlock(id) }
func (a *App) ChangeBoard(text string) error          { return a.b.ChangeBoard(text) }
func (a *App) ResetBoard() error                      { return a.b.ResetBoard() }
func (a *App) ListAssistantChats() []AssistantChatDTO { return a.b.ListAssistantChats() }
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

// ChooseFolder opens the folder picker and returns the chosen path, "" when cancelled.
func (a *App) ChooseFolder() (string, error) {
	if a.chooseDir == nil {
		return "", errors.New("the folder picker is not available")
	}
	return a.chooseDir(a.ctx, "Choose the agent's knowledge repo")
}
