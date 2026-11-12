// Package backend implements the calls the frontend makes, over the store, WhatsApp and the engine.
package backend

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GabiHert/positron/internal/app"
	"github.com/GabiHert/positron/internal/claude"
	"github.com/GabiHert/positron/internal/domain"
	"github.com/GabiHert/positron/internal/engine"
	"github.com/GabiHert/positron/internal/gitrepo"
	"github.com/GabiHert/positron/internal/mcpserver"
	"github.com/GabiHert/positron/internal/media"
	"github.com/GabiHert/positron/internal/port"
	"github.com/GabiHert/positron/internal/speech"
	"github.com/GabiHert/positron/internal/store"
	"github.com/GabiHert/positron/internal/update"
	"github.com/GabiHert/positron/internal/whatsapp"
	"github.com/google/uuid"
)

// Deps are the backend's collaborators.
type Deps struct {
	Store   *store.Store
	WA      *whatsapp.Client
	Engine  *engine.Engine
	Media   media.Store
	Models  *speech.ModelManager
	Runner  claude.Runner
	Env     func() []string
	Emit    func(event string, data any)
	Version string
	Login   LoginItem
	Update  *update.Service

	// The inbox's macOS side.
	Focused           func() bool                              // a Positron window has the keyboard focus
	NotifyChat        func(chat, title, subtitle, body string) // show a message notification
	ClearNotification func(id string)                          // take a delivered notification away
	OnUnreadTotal     func(total int)                          // the Dock badge and menu bar
}

// LoginItem turns "open at login" on or off.
type LoginItem interface {
	Set(on bool) error
}

type download struct {
	cancel context.CancelFunc
	prog   port.ModelProgress
}

// Backend implements app.Backend.
type Backend struct {
	d Deps

	mu        sync.Mutex
	claude    app.ClaudeDTO
	downloads map[domain.Language]*download
	v2        *v2
	mcp       *mcpserver.Server

	inboxOnce  sync.Once
	inboxState *inbox
}

var _ app.Backend = (*Backend)(nil)

// New returns a backend and checks Claude Code in the background.
func New(d Deps) *Backend {
	b := &Backend{d: d, downloads: map[domain.Language]*download{}}
	go b.RecheckClaude()
	return b
}

func (b *Backend) emit(event string, data any) {
	if b.d.Emit != nil {
		b.d.Emit(event, data)
	}
}

// EmitStatus sends the current status to the UI.
func (b *Backend) EmitStatus() { b.emit(app.EventStatus, b.Status()) }

func ms(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// MaskPhone shows a number like "+55 51 9•••• 4412".
func MaskPhone(digits string) string {
	if len(digits) < 8 {
		if digits == "" {
			return ""
		}
		return "+" + digits
	}
	return fmt.Sprintf("+%s %s %s•••• %s", digits[:2], digits[2:4], digits[4:5], digits[len(digits)-4:])
}

// Status is the app's overall state.
func (b *Backend) Status() app.StatusDTO {
	s := b.d.WA.State()
	b.mu.Lock()
	cl := b.claude
	b.mu.Unlock()
	return app.StatusDTO{
		WhatsApp: app.WhatsAppDTO{State: s.State, QR: s.QR, QRExpiresAt: ms(s.QRExpiresAt), Phone: MaskPhone(s.Phone),
			PushName: s.PushName, Since: ms(s.Since), Error: s.Error},
		Claude:  cl,
		Version: b.d.Version,
		Waiting: b.d.Store.CountRuns(domain.RunWaiting, time.Time{}),
	}
}

// RecheckClaude looks for the claude command again.
func (b *Backend) RecheckClaude() app.StatusDTO {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	path, version, err := b.d.Runner.Version(ctx)
	c := app.ClaudeDTO{Found: err == nil, Path: path, Version: version}
	if err != nil {
		c.Error = err.Error()
	}
	if home, _ := os.UserHomeDir(); home != "" && strings.HasPrefix(c.Path, home) {
		c.Path = "~" + strings.TrimPrefix(c.Path, home)
	}
	b.mu.Lock()
	b.claude = c
	b.mu.Unlock()
	st := b.Status()
	b.emit(app.EventStatus, st)
	return st
}

func (b *Backend) LinkWithPhone(ctx context.Context, phone string) (string, error) {
	return b.d.WA.LinkWithPhone(ctx, phone)
}

func (b *Backend) Logout(ctx context.Context) error { return b.d.WA.Logout(ctx) }

// ---- chats ----

func (b *Backend) ListChats() []app.ChatDTO {
	router := domain.NewRouter(b.d.Store.Agents())
	all, hasAll := router.AllDirect()
	chats := b.d.Store.Chats()
	out := make([]app.ChatDTO, 0, len(chats))
	for _, c := range chats {
		d := app.ChatDTO{JID: c.JID, Name: c.Name, IsGroup: c.IsGroup, Members: c.Members, LastText: c.LastText, LastAt: ms(c.LastAt)}
		if a, viaAll, ok := router.Owner(c.JID); ok {
			d.AgentID, d.AgentName, d.Mode, d.ViaAll = a.ID, a.Name, string(a.Mode), viaAll
		} else if hasAll && domain.IsDirect(c.JID) && all.Skips(c.JID) {
			d.Skipped = true
		}
		out = append(out, d)
	}
	return out
}

// AssignChat gives jid to agentID, taking it from any other agent. "" leaves it without an agent,
// which for a direct chat also tells the all-direct agent to skip it.
func (b *Backend) AssignChat(jid, agentID string) error {
	found := agentID == ""
	direct := domain.IsDirect(jid)
	for _, a := range b.d.Store.Agents() {
		keep := a.Chats[:0:0]
		for _, c := range a.Chats {
			if c != jid {
				keep = append(keep, c)
			}
		}
		except := a.Except
		if a.AllDirect && direct {
			except = without(a.Except, jid)
			if agentID == "" {
				except = append(except, jid)
			}
		}
		if a.ID == agentID {
			found = true
			// The all-direct agent already has a direct chat once it stops skipping it.
			if !(a.AllDirect && direct) {
				keep = append(keep, jid)
			}
		}
		if len(keep) != len(a.Chats) || a.ID == agentID || len(except) != len(a.Except) {
			a.Chats, a.Except = keep, except
			if err := b.d.Store.SaveAgent(a); err != nil {
				return err
			}
		}
	}
	if !found {
		return errors.New("that agent no longer exists")
	}
	b.emit(app.EventAgents, nil)
	b.emit(app.EventChats, nil)
	return nil
}

func without(list []string, v string) []string {
	out := make([]string, 0, len(list))
	for _, x := range list {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

// ---- agents ----

func (b *Backend) toDTO(a domain.Agent, full bool) app.AgentDTO {
	d := app.AgentDTO{
		ID: a.ID, Name: a.Name, Enabled: a.Enabled, RepoPath: tilde(a.RepoPath), Chats: append([]string{}, a.Chats...),
		AllDirect: a.AllDirect, Except: append([]string{}, a.Except...), AnswerMe: a.AnswerMe, MCP: append([]string{}, a.MCP...),
		FolderAccess: a.FolderAccess, Folders: append([]string{}, a.Folders...),
		Mode: string(a.Mode), Trigger: string(a.Trigger), WaitSeconds: a.WaitSeconds, MaxPerHour: a.MaxPerHour,
		Prefix: a.Prefix, Push: a.Push, ReadImages: a.ReadImages, TranscribeVoice: a.TranscribeVoice,
		SendImages: a.SendImages, VoiceReplies: a.VoiceReplies, Web: a.Web, Bash: a.Bash, Model: a.Model,
		Language: string(a.Language),
	}
	if a.ID != "" {
		d.Week = b.agentWeek(a.ID)
	}
	if full && a.RepoPath != "" {
		repo := gitrepo.Repo{Dir: a.RepoPath, EnvFn: b.d.Env}
		d.Instructions = repo.Instructions()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s := repo.Stats(ctx)
		d.Stats = app.RepoStatsDTO{Exists: s.Exists, Files: s.Files, Commits: s.Commits, LastLearned: ms(s.LastLearned), Remote: s.Remote}
	}
	return d
}

func tilde(p string) string {
	if home, _ := os.UserHomeDir(); home != "" && strings.HasPrefix(p, home+"/") {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

func expand(p string) string {
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, "~/") {
		if home, _ := os.UserHomeDir(); home != "" {
			p = filepath.Join(home, p[2:])
		}
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

func (b *Backend) ListAgents() []app.AgentDTO {
	agents := b.d.Store.Agents()
	out := make([]app.AgentDTO, 0, len(agents))
	for _, a := range agents {
		out = append(out, b.toDTO(a, false))
	}
	return out
}

func (b *Backend) GetAgent(id string) (app.AgentDTO, error) {
	a, err := b.d.Store.Agent(id)
	if err != nil {
		return app.AgentDTO{}, errors.New("that agent no longer exists")
	}
	return b.toDTO(a, true), nil
}

func (b *Backend) NewAgent() app.AgentDTO {
	a := domain.NewAgent(b.d.Store.Settings().Defaults)
	d := b.toDTO(a, false)
	d.Instructions = ""
	return d
}

// SaveAgent creates or updates an agent and sets up its repo.
func (b *Backend) SaveAgent(d app.AgentDTO) (app.AgentDTO, error) {
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" {
		return d, errors.New("Give the agent a name.")
	}
	if strings.TrimSpace(d.RepoPath) == "" {
		return d, errors.New("Choose a folder for the agent's knowledge repo.")
	}
	var a domain.Agent
	isNew := d.ID == ""
	if isNew {
		a = domain.NewAgent(b.d.Store.Settings().Defaults)
		a.ID, a.CreatedAt = uuid.NewString(), time.Now()
	} else {
		var err error
		if a, err = b.d.Store.Agent(d.ID); err != nil {
			return d, errors.New("that agent no longer exists")
		}
	}
	mode := domain.ReplyMode(d.Mode)
	if mode != domain.ModeAuto && mode != domain.ModeApprove && mode != domain.ModeNever {
		mode = domain.ModeApprove
	}
	trig := domain.Trigger(d.Trigger)
	if trig != domain.TriggerAll && trig != domain.TriggerMention {
		trig = domain.TriggerMention
	}
	lang := domain.Language(d.Language)
	if lang != domain.LangEN {
		lang = domain.LangPTBR
	}
	for _, other := range b.d.Store.Agents() {
		if other.ID != a.ID && strings.EqualFold(other.Name, d.Name) {
			return d, fmt.Errorf("Another agent is already called %s.", other.Name)
		}
	}
	a.Name, a.Enabled, a.RepoPath, a.Mode, a.Trigger = d.Name, d.Enabled, expand(d.RepoPath), mode, trig
	a.WaitSeconds, a.MaxPerHour, a.Prefix, a.Push = clamp(d.WaitSeconds, 0, 600), clamp(d.MaxPerHour, 0, 1000), strings.TrimSpace(d.Prefix), d.Push
	a.ReadImages, a.TranscribeVoice, a.SendImages, a.VoiceReplies = d.ReadImages, d.TranscribeVoice, d.SendImages, d.VoiceReplies
	a.Web, a.Bash, a.Model, a.Language = d.Web, d.Bash, d.Model, lang
	a.Chats = dedupe(d.Chats)
	a.AllDirect, a.AnswerMe, a.MCP = d.AllDirect, d.AnswerMe, dedupe(d.MCP)
	switch d.FolderAccess {
	case "", "read", "edit":
	default:
		return d, fmt.Errorf("unknown folder access %q", d.FolderAccess)
	}
	a.FolderAccess, a.Folders = d.FolderAccess, nil
	for _, f := range dedupe(d.Folders) {
		f = expand(strings.TrimSpace(f))
		if st, err := os.Stat(f); err != nil || !st.IsDir() || !filepath.IsAbs(f) {
			return d, fmt.Errorf("%s is not a folder on this Mac.", f)
		}
		a.Folders = append(a.Folders, filepath.Clean(f))
	}
	if a.FolderAccess != "" && len(a.Folders) == 0 {
		return d, errors.New("Pick at least one folder for computer access, or turn it off.")
	}
	a.Except = nil
	if a.AllDirect {
		for _, c := range dedupe(d.Except) {
			if domain.IsDirect(c) {
				a.Except = append(a.Except, c)
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	repo := gitrepo.Repo{Dir: a.RepoPath, EnvFn: b.d.Env}
	if err := repo.Init(ctx); err != nil {
		return d, fmt.Errorf("Could not set up the repo: %v", err)
	}
	if strings.TrimSpace(d.Instructions) != "" && d.Instructions != repo.Instructions() {
		if err := repo.SetInstructions(d.Instructions); err != nil {
			return d, err
		}
		_, _, _ = repo.Commit(ctx, a.Name+": instructions edited in Positron")
	}
	// A chat belongs to one agent: take this agent's chats away from the others.
	mine := map[string]bool{}
	for _, c := range a.Chats {
		mine[c] = true
	}
	for _, other := range b.d.Store.Agents() {
		if other.ID == a.ID {
			continue
		}
		keep := other.Chats[:0:0]
		for _, c := range other.Chats {
			if !mine[c] {
				keep = append(keep, c)
			}
		}
		// Only one agent can have all direct chats.
		dropAll := a.AllDirect && other.AllDirect
		if len(keep) != len(other.Chats) || dropAll {
			other.Chats = keep
			if dropAll {
				other.AllDirect, other.Except = false, nil
			}
			_ = b.d.Store.SaveAgent(other)
		}
	}
	if err := b.d.Store.SaveAgent(a); err != nil {
		return d, err
	}
	b.emit(app.EventAgents, nil)
	b.emit(app.EventChats, nil)
	return b.toDTO(a, true), nil
}

func clamp(v, lo, hi int) int { return max(lo, min(hi, v)) }

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func (b *Backend) DeleteAgent(id string) error {
	if err := b.d.Store.DeleteAgent(id); err != nil {
		return err
	}
	b.emit(app.EventAgents, nil)
	b.emit(app.EventChats, nil)
	return nil
}

// OpenRepoInClaude opens Terminal in the agent's repo running claude.
func (b *Backend) OpenRepoInClaude(id string) error {
	a, err := b.d.Store.Agent(id)
	if err != nil {
		return errors.New("that agent no longer exists")
	}
	quoted := "'" + strings.ReplaceAll(a.RepoPath, "'", `'\''`) + "'"
	cmd := "cd " + quoted + " && claude"
	script := fmt.Sprintf(`tell application "Terminal"
	activate
	do script %q
end tell`, cmd)
	if out, err := exec.Command("/usr/bin/osascript", "-e", script).CombinedOutput(); err != nil {
		return fmt.Errorf("could not open Terminal: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// ---- activity ----

func (b *Backend) runDTO(r domain.Run, agents map[string]domain.Agent) app.RunDTO {
	a, ok := agents[r.AgentID]
	name := a.Name
	if !ok {
		name = "Deleted agent"
	}
	chat, _ := b.d.Store.Chat(r.ChatJID)
	d := app.RunDTO{ID: r.ID, At: ms(r.At), AgentID: r.AgentID, AgentName: name, ChatJID: r.ChatJID, ChatName: chat.Name,
		IsGroup: chat.IsGroup, Status: string(r.Status), Reply: r.Reply, Voice: r.Voice, Note: r.Note, Request: r.Request, Commit: r.Commit,
		Files: append([]string{}, r.Files...), Error: r.Error, SentAt: ms(r.SentAt), Incoming: []app.MessageDTO{}, Images: []app.AttachmentDTO{}}
	if d.ChatName == "" {
		d.ChatName = "a chat"
	}
	if !r.DismissedAt.IsZero() {
		d.DismissedAt = ms(r.DismissedAt)
	}
	if r.Usage != nil {
		u := usageDTO(*r.Usage)
		d.Usage = &u
	}
	if t := b.d.Store.LastFromMe(r.ChatJID, r.At); !t.IsZero() {
		d.AnsweredAt = ms(t)
	}
	for _, id := range r.MessageIDs {
		m, err := b.d.Store.Message(r.ChatJID, id)
		if err != nil {
			continue
		}
		md := app.MessageDTO{ID: m.ID, At: ms(m.At), SenderName: m.SenderName, FromMe: m.FromMe, Kind: string(m.Kind),
			Text: m.Text, Seconds: m.Seconds, Transcript: m.Transcript, MentionsMe: m.MentionsMe}
		if rel := b.d.Media.Rel(m.MediaPath); rel != "" {
			md.MediaURL = "/media/" + pathEscape(rel)
		}
		d.Incoming = append(d.Incoming, md)
	}
	for _, p := range r.Images {
		att := app.AttachmentDTO{Path: p, URL: "/repo/" + url.PathEscape(r.AgentID) + "/" + pathEscape(p)}
		if ok {
			if st, err := os.Stat(filepath.Join(a.RepoPath, filepath.FromSlash(p))); err == nil {
				att.Bytes = st.Size()
			}
		}
		d.Images = append(d.Images, att)
	}
	return d
}

func usageDTO(u domain.Usage) app.UsageDTO {
	return app.UsageDTO{Model: u.Model, Input: u.Input, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite, Output: u.Output,
		Tokens: u.Tokens(), CostUSD: u.CostUSD, Turns: u.Turns, DurationMs: u.DurationMs}
}

// agentWeek adds up what an agent's runs used in the last 7 days.
func (b *Backend) agentWeek(agentID string) app.UsageTotalDTO {
	t := domain.SumUsage(b.d.Store.Runs(store.RunQuery{AgentID: agentID, Since: time.Now().AddDate(0, 0, -7)}))
	return app.UsageTotalDTO{Runs: t.Runs, Measured: t.Measured, Usage: usageDTO(t.Usage)}
}

func pathEscape(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

func (b *Backend) agentMap() map[string]domain.Agent {
	m := map[string]domain.Agent{}
	for _, a := range b.d.Store.Agents() {
		m[a.ID] = a
	}
	return m
}

// RunDTO converts one run for the "activity" event.
func (b *Backend) RunDTO(r domain.Run) app.RunDTO { return b.runDTO(r, b.agentMap()) }

// ListActivity lists runs: "" for all, "needs" for waiting and failed, or an agent ID.
func (b *Backend) ListActivity(filter string) []app.RunDTO {
	q := store.RunQuery{Limit: 200}
	var runs []domain.Run
	switch filter {
	case "":
		runs = b.d.Store.Runs(q)
	case "needs":
		q.Status = domain.RunWaiting
		runs = b.d.Store.Runs(q)
		q.Status = domain.RunFailed
		q.Since = time.Now().AddDate(0, 0, -2)
		runs = append(runs, b.d.Store.Runs(q)...)
	default:
		q.AgentID = filter
		runs = b.d.Store.Runs(q)
	}
	agents := b.agentMap()
	out := make([]app.RunDTO, 0, len(runs))
	for _, r := range runs {
		out = append(out, b.runDTO(r, agents))
	}
	if filter == "needs" {
		sortByAt(out)
	}
	return out
}

func sortByAt(rs []app.RunDTO) {
	for i := 1; i < len(rs); i++ {
		for j := i; j > 0 && rs[j].At > rs[j-1].At; j-- {
			rs[j], rs[j-1] = rs[j-1], rs[j]
		}
	}
}

func (b *Backend) TodayStats() app.StatsDTO {
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var s app.StatsDTO
	for _, r := range b.d.Store.Runs(store.RunQuery{Since: midnight}) {
		switch r.Status {
		case domain.RunSent:
			s.Sent++
			s.ImagesSent += len(r.Images)
		case domain.RunWaiting:
			s.Waiting++
		case domain.RunQuiet:
			s.Quiet++
		case domain.RunFailed:
			s.Failed++
		}
		if r.Commit != "" {
			s.Learned++
		}
		for _, id := range r.MessageIDs {
			if m, err := b.d.Store.Message(r.ChatJID, id); err == nil {
				if m.Kind == domain.KindImage && m.MediaPath != "" {
					s.Photos++
				}
				if m.Transcript != "" {
					s.Voice++
				}
			}
		}
	}
	return s
}

func (b *Backend) SendDraft(runID, text string) error { return b.d.Engine.SendDraft(runID, text) }
func (b *Backend) DiscardDraft(runID string) error    { return b.d.Engine.DiscardDraft(runID) }

// DismissRun clears a run from "needs you" lists on the board, or brings it back.
func (b *Backend) DismissRun(runID string, dismissed bool) error {
	return b.d.Engine.DismissRun(runID, dismissed)
}
func (b *Backend) RetryRun(runID string) error { return b.d.Engine.Retry(runID) }

// ---- settings ----

func (b *Backend) GetSettings() app.SettingsDTO {
	s := b.d.Store.Settings()
	files, bytes := b.d.Media.Usage()
	return app.SettingsDTO{
		Defaults:           app.DefaultsDTO{Mode: string(s.Defaults.Mode), MaxPerHour: s.Defaults.MaxPerHour, WaitSeconds: s.Defaults.WaitSeconds, Prefix: s.Defaults.Prefix},
		MediaRetentionDays: s.MediaRetentionDays, OpenAtLogin: s.OpenAtLogin, Notify: s.Notify, MCPEnabled: s.MCPEnabled,
		MediaDir: tilde(b.d.Media.Dir), MediaBytes: bytes, MediaFiles: files,
	}
}

func (b *Backend) SaveSettings(d app.SettingsDTO) error {
	old := b.d.Store.Settings()
	mode := domain.ReplyMode(d.Defaults.Mode)
	if mode != domain.ModeAuto && mode != domain.ModeApprove && mode != domain.ModeNever {
		mode = domain.ModeApprove
	}
	s := domain.Settings{
		Defaults: domain.AgentDefaults{Mode: mode, MaxPerHour: clamp(d.Defaults.MaxPerHour, 0, 1000),
			WaitSeconds: clamp(d.Defaults.WaitSeconds, 0, 600), Prefix: strings.TrimSpace(d.Defaults.Prefix)},
		MediaRetentionDays: clamp(d.MediaRetentionDays, 0, 3650), OpenAtLogin: d.OpenAtLogin, Notify: d.Notify,
		MCPEnabled: d.MCPEnabled,
	}
	if s.OpenAtLogin != old.OpenAtLogin && b.d.Login != nil {
		if err := b.d.Login.Set(s.OpenAtLogin); err != nil {
			return fmt.Errorf("could not change Open at login: %w", err)
		}
	}
	if err := b.d.Store.SaveSettings(s); err != nil {
		return err
	}
	if s.MediaRetentionDays != old.MediaRetentionDays {
		go b.d.Media.Prune(s.MediaRetentionDays)
	}
	if s.MCPEnabled != old.MCPEnabled && b.mcp != nil {
		_ = b.mcp.SetEnabled(s.MCPEnabled) // a failure shows in GetMCPStatus
	}
	return nil
}

func (b *Backend) ShowMediaFolder() error {
	_ = os.MkdirAll(b.d.Media.Dir, 0o700)
	return exec.Command("/usr/bin/open", b.d.Media.Dir).Run()
}

// ---- speech models ----

func (b *Backend) ListSpeechModels() []app.SpeechModelDTO {
	out := make([]app.SpeechModelDTO, 0, len(domain.Languages))
	for _, lang := range domain.Languages {
		st := b.d.Models.Status(lang)
		m := app.SpeechModelDTO{Language: string(lang), Label: lang.Label(), Installed: st.Ready, SizeBytes: st.DownloadBytes}
		b.mu.Lock()
		if dl := b.downloads[lang]; dl != nil {
			m.Downloading = dl.cancel != nil
			m.Done, m.Total, m.Phase, m.Error = dl.prog.Done, dl.prog.Total, dl.prog.Phase, dl.prog.Err
			if m.Phase == "done" || m.Phase == "error" {
				m.Phase = ""
			}
		}
		b.mu.Unlock()
		out = append(out, m)
	}
	return out
}

func (b *Backend) DownloadSpeechModel(lang string) error {
	l := domain.Language(lang)
	if l != domain.LangEN && l != domain.LangPTBR {
		return fmt.Errorf("unknown language %q", lang)
	}
	b.mu.Lock()
	if dl := b.downloads[l]; dl != nil && dl.cancel != nil {
		b.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	dl := &download{cancel: cancel}
	b.downloads[l] = dl
	b.mu.Unlock()
	b.emit(app.EventSpeech, b.ListSpeechModels())

	go func() {
		var last time.Time
		err := b.d.Models.Download(ctx, l, func(p port.ModelProgress) {
			b.mu.Lock()
			dl.prog = p
			b.mu.Unlock()
			if time.Since(last) > 300*time.Millisecond || p.Phase != "download" {
				last = time.Now()
				b.emit(app.EventSpeech, b.ListSpeechModels())
			}
		})
		b.mu.Lock()
		dl.cancel = nil
		if err != nil && !errors.Is(err, context.Canceled) {
			dl.prog.Err = err.Error()
		} else {
			dl.prog = port.ModelProgress{}
		}
		b.mu.Unlock()
		cancel()
		b.emit(app.EventSpeech, b.ListSpeechModels())
	}()
	return nil
}

func (b *Backend) CancelSpeechDownload(lang string) {
	b.mu.Lock()
	dl := b.downloads[domain.Language(lang)]
	b.mu.Unlock()
	if dl != nil && dl.cancel != nil {
		dl.cancel()
	}
}
