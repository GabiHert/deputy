package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/GabiHert/positron/internal/app"
	"github.com/GabiHert/positron/internal/claude"
	"github.com/GabiHert/positron/internal/mcpserver"
)

// SetMCP connects the server for Claude Code in other projects and starts it when the setting is on.
func (b *Backend) SetMCP(m *mcpserver.Server) {
	b.mcp = m
	if b.d.Store.Settings().MCPEnabled {
		_ = m.SetEnabled(true) // a failure shows in GetMCPStatus
	}
}

// SendNow sends text to a chat right away, as an agent or, with agentID "", as the owner.
func (b *Backend) SendNow(agentID, chatJID, text string) error {
	return b.d.Engine.SendNow(agentID, chatJID, text)
}

// AskAgent has an agent do the owner's request in a chat (engine.Request) and returns the run.
func (b *Backend) AskAgent(ctx context.Context, agentID, chatJID, request string, review bool) (app.RunDTO, error) {
	r, err := b.d.Engine.Request(ctx, agentID, chatJID, request, review)
	if err != nil {
		return app.RunDTO{}, err
	}
	return b.RunDTO(r), nil
}

const (
	mcpName       = "positron"
	legacyMCPName = "deputy"
)

func (b *Backend) GetMCPStatus() app.MCPStatusDTO {
	s := app.MCPStatusDTO{Enabled: b.d.Store.Settings().MCPEnabled}
	if b.mcp == nil {
		s.Error = "Positron is still starting"
		return s
	}
	s.URL = b.mcp.URL()
	s.Running, s.Error = b.mcp.Status()
	if tok, err := b.mcp.Token(); err == nil {
		s.Command = strings.Join(quoteAll(mcpAddArgs(s.URL, tok)), " ")
	} else if s.Error == "" {
		s.Error = err.Error()
	}
	return s
}

func (b *Backend) SetMCPEnabled(on bool) (app.MCPStatusDTO, error) {
	s := b.d.Store.Settings()
	s.MCPEnabled = on
	if err := b.d.Store.SaveSettings(s); err != nil {
		return b.GetMCPStatus(), err
	}
	if b.mcp != nil {
		_ = b.mcp.SetEnabled(on) // a failure shows in the status
	}
	return b.GetMCPStatus(), nil
}

func (b *Backend) RotateMCPToken() (app.MCPStatusDTO, error) {
	if b.mcp == nil {
		return b.GetMCPStatus(), errors.New("Positron is still starting")
	}
	_, err := b.mcp.RotateToken()
	return b.GetMCPStatus(), err
}

// AddMCPToClaude registers Positron in the person's Claude Code (user scope), replacing an older entry.
func (b *Backend) AddMCPToClaude() (string, error) {
	if b.mcp == nil {
		return "", errors.New("Positron is still starting")
	}
	tok, err := b.mcp.Token()
	if err != nil {
		return "", err
	}
	env := claude.LoginShellEnv()
	if b.d.Env != nil {
		env = b.d.Env()
	}
	env = claude.CleanEnv(env)
	path, err := claude.LookPath(env, "claude")
	if err != nil {
		return "", errors.New("Claude Code was not found. Install it and make sure the claude command runs in your terminal.")
	}
	run := func(args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, path, args...)
		cmd.Env = env
		if home, err := os.UserHomeDir(); err == nil {
			cmd.Dir = home
		}
		out, err := cmd.CombinedOutput()
		// The output never shows the token, even if claude echoes the header.
		return strings.TrimSpace(strings.ReplaceAll(string(out), tok, "•••")), err
	}
	_, _ = run("mcp", "remove", "--scope", "user", mcpName)
	// Positron was called Deputy, and so was its entry; the old one points at the same server.
	_, _ = run("mcp", "remove", "--scope", "user", legacyMCPName)
	out, err := run(mcpAddArgs(b.mcp.URL(), tok)[1:]...)
	if err != nil {
		if out == "" {
			out = err.Error()
		}
		return out, fmt.Errorf("claude mcp add failed: %s", out)
	}
	return out, nil
}

// mcpAddArgs is the claude command that adds Positron. --header goes last: it takes several values.
func mcpAddArgs(url, token string) []string {
	return []string{"claude", "mcp", "add", "--transport", "http", "--scope", "user", mcpName, url,
		"--header", "Authorization: Bearer " + token}
}

func quoteAll(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \"'$\\") {
			a = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
		out[i] = a
	}
	return out
}
