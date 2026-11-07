package claude

import (
	"bufio"
	"context"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// MCPServer is one MCP server the person's Claude Code knows about.
type MCPServer struct {
	Name   string `json:"name"`   // as claude mcp list shows it, e.g. "linear", "claude.ai Notion"
	Target string `json:"target"` // its URL or command
	Status string `json:"status"` // "connected", "failed", "needs auth", "pending", or the raw text
}

var nonTool = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// MCPToolRule is the --allowedTools rule that allows every tool of the server called name.
// Claude Code names tools mcp__<server>__<tool>, with characters other than letters,
// digits, _ and - turned into _ ("claude.ai Notion" becomes claude_ai_Notion).
func MCPToolRule(name string) string { return "mcp__" + nonTool.ReplaceAllString(name, "_") }

// ListMCP runs claude mcp list, which also checks each server's health.
func (rn Runner) ListMCP(ctx context.Context) ([]MCPServer, error) {
	env := CleanEnv(rn.env())
	path, err := LookPath(env, "claude")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "mcp", "list")
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return nil, err
	}
	return parseMCPList(string(out)), nil
}

func parseMCPList(out string) []MCPServer {
	var list []MCPServer
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		name, rest, ok := strings.Cut(line, ": ")
		if !ok || name == "" || strings.Contains(name, " health") || strings.HasPrefix(line, "MCP config") || strings.HasPrefix(line, "For help") {
			continue
		}
		i := strings.LastIndex(rest, " - ")
		if i < 0 {
			continue
		}
		target, status := rest[:i], strings.TrimSpace(rest[i+3:])
		target = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(target, " (HTTP)"), " (SSE)"), " (stdio)")
		low := strings.ToLower(status)
		switch {
		case strings.Contains(low, "connected") && !strings.Contains(low, "failed"):
			status = "connected"
		case strings.Contains(low, "needs auth") || strings.Contains(low, "authenticat"):
			status = "needs auth"
		case strings.Contains(low, "pending"):
			status = "pending"
		case strings.Contains(low, "failed"):
			status = "failed"
		}
		list = append(list, MCPServer{Name: name, Target: target, Status: status})
	}
	return list
}

// ReadOnlyRules are --disallowedTools rules that stop file edits inside dirs (absolute paths).
// One Edit rule covers every file-editing tool; "//" marks an absolute path.
func ReadOnlyRules(dirs []string) []string {
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		out = append(out, "Edit(/"+strings.TrimSuffix(d, "/")+"/**)")
	}
	return out
}
