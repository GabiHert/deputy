package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// AgentRun is one headless Claude Code run of an agent.
type AgentRun struct {
	Dir          string // the agent's repo
	SystemPrompt string // Deputy's protocol, appended to Claude Code's prompt
	Prompt       string // the chat and the new messages
	Model        string // "" = the person's default
	Web          bool
	Bash         bool
	MCP          []string // names of the person's MCP servers the agent may use
	Folders      []string // extra folders it may use
	ReadOnly     bool     // Folders may be read, not changed
	Timeout      time.Duration
}

// AgentResult is what the agent decided.
type AgentResult struct {
	Reply  string   `json:"reply"`  // "" = stay quiet
	Images []string `json:"images"` // repo-relative image paths to send
	Voice  bool     `json:"voice"`  // send the reply as a voice note
	Note   string   `json:"note"`   // one line for Activity: why quiet, or what it learned
}

const resultSchema = `{"type":"object","properties":{` +
	`"reply":{"type":"string","description":"The WhatsApp message to send, or an empty string to stay quiet."},` +
	`"images":{"type":"array","items":{"type":"string"},"description":"Paths, relative to the repo, of images to send with the reply."},` +
	`"voice":{"type":"boolean","description":"True to send the reply as a voice note instead of text."},` +
	`"note":{"type":"string","description":"One short line for the owner: why you stayed quiet, or what you learned."}` +
	`},"required":["reply","note"]}`

// Runner runs agents with the person's own claude command.
type Runner struct {
	EnvFn func() []string // default LoginShellEnv
}

// Version returns the claude command's path and version.
func (rn Runner) Version(ctx context.Context) (path, version string, err error) {
	env := CleanEnv(rn.env())
	path, err = LookPath(env, "claude")
	if err != nil {
		return "", "", errors.New("Claude Code was not found. Install it and make sure the claude command runs in your terminal.")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		return path, "", fmt.Errorf("claude --version failed: %w", err)
	}
	version = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(string(out)), "(Claude Code)"))
	return path, version, nil
}

func (rn Runner) env() []string {
	if rn.EnvFn != nil {
		return rn.EnvFn()
	}
	return LoginShellEnv()
}

// Run runs the agent and returns its decision.
func (rn Runner) Run(ctx context.Context, r AgentRun) (AgentResult, error) {
	env := CleanEnv(rn.env())
	path, err := LookPath(env, "claude")
	if err != nil {
		return AgentResult{}, errors.New("Claude Code was not found. Install it and make sure the claude command runs in your terminal.")
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	tools := []string{"Read", "Write", "Edit", "MultiEdit", "Glob", "Grep"}
	if r.Web {
		tools = append(tools, "WebSearch", "WebFetch")
	}
	disallowed := []string{"Task", "NotebookEdit"}
	if r.Bash {
		tools = append(tools, "Bash")
	} else {
		disallowed = append(disallowed, "Bash")
	}
	for _, name := range r.MCP {
		tools = append(tools, MCPToolRule(name))
	}
	if r.ReadOnly {
		disallowed = append(disallowed, ReadOnlyRules(r.Folders)...)
	}
	args := []string{"-p", "--output-format", "json", "--json-schema", resultSchema, "--no-session-persistence",
		"--permission-mode", "acceptEdits", "--allowedTools", strings.Join(tools, " "),
		"--disallowedTools", strings.Join(disallowed, " "), "--append-system-prompt", r.SystemPrompt}
	for _, d := range r.Folders {
		args = append(args, "--add-dir", d)
	}
	if r.Model != "" {
		args = append(args, "--model", r.Model)
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, path, args...)
	cmd.Dir = r.Dir
	cmd.Env = env
	cmd.Stdin = strings.NewReader(r.Prompt)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		pid := cmd.Process.Pid
		err := syscall.Kill(-pid, syscall.SIGTERM)
		time.AfterFunc(500*time.Millisecond, func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })
		return err
	}
	cmd.WaitDelay = 2 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	err = cmd.Run()
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return AgentResult{}, fmt.Errorf("Claude Code didn't finish within %d minutes. Nothing was sent.", int(timeout/time.Minute))
	}
	if ctx.Err() != nil {
		return AgentResult{}, ctx.Err()
	}
	var out struct {
		IsError          bool            `json:"is_error"`
		Subtype          string          `json:"subtype"`
		Result           string          `json:"result"`
		StructuredOutput json.RawMessage `json:"structured_output"`
	}
	if jerr := json.Unmarshal(stdout.Bytes(), &out); jerr != nil {
		both := strings.ToLower(stdout.String() + stderr.String())
		if strings.Contains(both, "/login") || strings.Contains(both, "not logged in") {
			return AgentResult{}, errors.New("Claude Code is not logged in. Run claude in your terminal and log in.")
		}
		msg := "Claude Code stopped with an error"
		if err != nil {
			msg += ": " + err.Error()
		}
		if t := lastLines(stderr.String(), 6); t != "" {
			msg += "\n" + t
		}
		return AgentResult{}, errors.New(msg)
	}
	if out.IsError {
		return AgentResult{}, fmt.Errorf("Claude Code stopped with an error (%s): %s", out.Subtype, lastLines(out.Result, 6))
	}
	var res AgentResult
	raw := out.StructuredOutput
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage(out.Result)
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return AgentResult{}, fmt.Errorf("the agent's answer was not in the expected form: %s", lastLines(out.Result, 4))
	}
	res.Reply = strings.TrimSpace(res.Reply)
	if res.Reply == "NO_REPLY" {
		res.Reply = ""
	}
	return res, nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
