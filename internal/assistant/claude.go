package assistant

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/GabiHert/deputy/internal/claude"
	"github.com/google/uuid"
)

// streamEvent is the part of a stream-json line Deputy reads.
type streamEvent struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	SessionID string `json:"session_id"`
	IsError   bool   `json:"is_error"`
	Result    string `json:"result"`
	Message   struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
}

type runSpec struct {
	chatID    string // the tool server routes tools by this
	sessionID string
	resume    bool
	persist   bool // keep the session so the chat can resume
	system    string
	prompt    string
	tools     []string
	builtin   []string // Claude Code's own tools it may use on the Mac (none for most chats)
	dirs      []string // folders those tools may use
	onText    func(string)
}

// agentAccess is what a chat with an agent may do on the Mac: the agent's own folder access
// (Settings > What it can do), plus shell commands when the agent has them.
func (s *Service) agentAccess(c Chat) (tools, dirs []string) {
	if c.Purpose != PurposeAsk {
		return nil, nil
	}
	ag, err := s.d.Store.Agent(c.AgentID)
	if err != nil || ag.FolderAccess == "" || len(ag.Folders) == 0 {
		return nil, nil
	}
	tools = []string{"Read", "Glob", "Grep", "WebSearch", "WebFetch"}
	if ag.FolderAccess == "edit" {
		tools = append(tools, "Edit", "MultiEdit", "Write")
	}
	if ag.Bash {
		tools = append(tools, "Bash")
	}
	return tools, ag.Folders
}

type runResult struct {
	sessionID string
	final     string
}

func (s *Service) claudeArgs(r runSpec) []string {
	mcpCfg, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"deputy": map[string]any{
		"type": "http", "url": s.tools.url,
		"headers": map[string]string{"Authorization": "Bearer " + s.tools.token, chatHeader: r.chatID},
	}}})
	allowed := make([]string, 0, len(r.tools))
	for _, t := range r.tools {
		allowed = append(allowed, "mcp__deputy__"+t)
	}
	builtin := r.builtin
	allowed = append(allowed, builtin...)
	args := []string{"-p", "--output-format", "stream-json", "--verbose",
		"--append-system-prompt", r.system,
		"--mcp-config", string(mcpCfg), "--strict-mcp-config",
		"--tools", strings.Join(builtin, ","),
		"--allowedTools", strings.Join(allowed, ","),
		"--setting-sources", "",
		"--disable-slash-commands",
	}
	switch {
	case !r.persist:
		args = append(args, "--no-session-persistence")
	case r.resume:
		args = append(args, "--resume", r.sessionID)
	default:
		args = append(args, "--session-id", r.sessionID)
	}
	if len(builtin) > 0 {
		for _, d := range r.dirs {
			args = append(args, "--add-dir", d)
		}
		if slices.ContainsFunc(builtin, func(t string) bool { return t == "Edit" || t == "Bash" }) {
			args = append(args, "--permission-mode", "acceptEdits")
		}
	}
	if s.d.Model != "" {
		args = append(args, "--model", s.d.Model)
	}
	return args
}

// claudeRun runs the claude command once and streams its text.
func (s *Service) claudeRun(ctx context.Context, r runSpec) (runResult, error) {
	env := claude.CleanEnv(s.d.Env())
	env = withoutPrefix(env, "CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT")
	path, err := claude.LookPath(env, "claude")
	if err != nil {
		return runResult{}, errors.New("Claude Code was not found. Install it and make sure the claude command runs in your terminal.")
	}
	cmd := exec.CommandContext(ctx, path, s.claudeArgs(r)...)
	cmd.Dir = ensureDir(s.d.Dir)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(r.prompt)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 3 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return runResult{}, err
	}
	if err := cmd.Start(); err != nil {
		return runResult{}, fmt.Errorf("could not start Claude Code: %w", err)
	}
	var res runResult
	var resultErr string
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 0, 1<<20), 32<<20)
	for sc.Scan() {
		var ev streamEvent
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		if ev.SessionID != "" {
			res.sessionID = ev.SessionID
		}
		switch ev.Type {
		case "assistant":
			for _, c := range ev.Message.Content {
				if c.Type == "text" && strings.TrimSpace(c.Text) != "" && r.onText != nil {
					r.onText(strings.TrimSpace(c.Text))
				}
			}
		case "result":
			res.final = ev.Result
			if ev.IsError || ev.Subtype != "success" {
				resultErr = ev.Result
				if resultErr == "" {
					resultErr = ev.Subtype
				}
			}
		}
	}
	waitErr := cmd.Wait()
	switch {
	case ctx.Err() == context.Canceled:
		return res, errStopped
	case ctx.Err() == context.DeadlineExceeded:
		return res, errors.New("Claude Code took too long and was stopped")
	case resultErr != "":
		return res, friendly(resultErr + "\n" + stderr.String())
	case waitErr != nil:
		return res, friendly(lastLines(stderr.String(), 6) + "\n" + waitErr.Error())
	}
	return res, nil
}

var errStopped = errors.New("stopped")

func friendly(msg string) error {
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "/login") || strings.Contains(low, "invalid api key") || strings.Contains(low, "not logged in"):
		return errors.New("Claude Code isn't logged in. Run claude in Terminal, log in, then try again.")
	case strings.Contains(low, "no conversation found"):
		return errNoSession
	case strings.Contains(low, "usage limit") || strings.Contains(low, "rate limit"):
		return errors.New("Claude Code hit its usage limit. Try again later.")
	}
	return errors.New("Claude Code failed: " + lastLines(strings.TrimSpace(msg), 4))
}

var errNoSession = errors.New("the Claude session is gone")

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func withoutPrefix(env []string, prefixes ...string) []string {
	out := env[:0:0]
	for _, e := range env {
		skip := false
		for _, p := range prefixes {
			if strings.HasPrefix(e, p) {
				skip = true
			}
		}
		if !skip {
			out = append(out, e)
		}
	}
	return out
}

// run answers the person's message in a chat.
func (s *Service) run(ctx context.Context, chatID, prompt string) {
	c, err := s.chat(chatID)
	if err != nil {
		return
	}
	builtin, dirs := s.agentAccess(c)
	spec := runSpec{
		chatID: chatID, sessionID: c.SessionID, resume: c.Started, persist: true,
		system: s.systemPrompt(c), prompt: prompt, tools: toolsFor(c.Purpose),
		onText:  func(t string) { s.add(chatID, "assistant", t, nil) },
		builtin: builtin, dirs: dirs,
	}
	res, err := s.claudeRun(ctx, spec)
	if errors.Is(err, errNoSession) {
		// Claude Code lost the session: start a new one with the chat so far.
		c.SessionID, c.Started = uuid.NewString(), false
		_ = s.saveChat(c)
		spec.sessionID, spec.resume = c.SessionID, false
		spec.prompt = s.transcript(chatID) + "\n\n" + prompt
		res, err = s.claudeRun(ctx, spec)
	}
	if c2, e := s.chat(chatID); e == nil && !c2.Started && (err == nil || res.sessionID != "") {
		c2.Started = true
		_ = s.saveChat(c2)
	}
	switch {
	case errors.Is(err, errStopped):
		s.add(chatID, "note", "Stopped.", nil)
	case err != nil:
		log.Printf("assistant %s: %v", chatID, err)
		s.add(chatID, "error", err.Error(), nil)
	}
}

// transcript is the chat so far, for a session that has to start over.
func (s *Service) transcript(chatID string) string {
	var b strings.Builder
	b.WriteString("[Deputy: the earlier session was lost. The conversation so far:]\n")
	for _, m := range s.messages(chatID) {
		switch m.Role {
		case "user", "assistant", "note":
			fmt.Fprintf(&b, "%s: %s\n", m.Role, m.Text)
		case "card":
			if m.Card != nil {
				fmt.Fprintf(&b, "card (%s, %s): %s\n", m.Card.Kind, m.Card.Status, cardPreview(*m.Card))
			}
		}
	}
	return b.String()
}

// WriteBlock has Claude write a board block's text with read-only tools. No session is kept.
func (s *Service) WriteBlock(ctx context.Context, blockID, title, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	var parts []string
	res, err := s.claudeRun(ctx, runSpec{
		chatID: "block:" + blockID, sessionID: uuid.NewString(), persist: false,
		system: s.blockPrompt(title), prompt: prompt, tools: readTools,
		onText: func(t string) { parts = append(parts, t) },
	})
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(res.final)
	if text == "" && len(parts) > 0 {
		text = parts[len(parts)-1]
	}
	if text == "" {
		return "", errors.New("Claude wrote nothing")
	}
	return text, nil
}
