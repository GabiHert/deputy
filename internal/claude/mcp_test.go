package claude

import "testing"

func TestMCPToolRule(t *testing.T) {
	for in, want := range map[string]string{
		"linear":                       "mcp__linear",
		"claude.ai Google Calendar":    "mcp__claude_ai_Google_Calendar",
		"plugin:playwright:playwright": "mcp__plugin_playwright_playwright",
	} {
		if got := MCPToolRule(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestReadOnlyRules(t *testing.T) {
	got := ReadOnlyRules([]string{"/Users/g/cb5", "/tmp/x/"})
	if len(got) != 2 || got[0] != "Edit(//Users/g/cb5/**)" || got[1] != "Edit(//tmp/x/**)" {
		t.Fatalf("rules %v", got)
	}
}

func TestParseMCPList(t *testing.T) {
	out := `Checking MCP server health…

claude.ai Notion: https://mcp.notion.com/mcp - ✔ Connected
plugin:context7:context7: https://mcp.context7.com/mcp?client=x (HTTP) - ✘ Failed to connect — invalid_target: OAuthError
linear: https://mcp.linear.app/mcp (HTTP) - ✔ Connected
pencil: /Applications/Pencil.app/x --app desktop - ✔ Connected
quant: http://localhost:52945/mcp (HTTP) - ⏸ Pending approval (run ` + "`claude`" + ` to approve)

MCP config diagnostics ⚠
For help configuring MCP servers, see: https://code.claude.com/docs/en/mcp`
	got := parseMCPList(out)
	if len(got) != 5 {
		t.Fatalf("got %d servers: %+v", len(got), got)
	}
	want := []MCPServer{
		{"claude.ai Notion", "https://mcp.notion.com/mcp", "connected"},
		{"plugin:context7:context7", "https://mcp.context7.com/mcp?client=x", "failed"},
		{"linear", "https://mcp.linear.app/mcp", "connected"},
		{"pencil", "/Applications/Pencil.app/x --app desktop", "connected"},
		{"quant", "http://localhost:52945/mcp", "pending"},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}
