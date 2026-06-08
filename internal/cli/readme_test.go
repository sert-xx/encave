package cli

import (
	"strings"
	"testing"

	"github.com/sert-xx/encave/internal/adapter"
)

func TestRenderAgentReadmeWithAuthVars(t *testing.T) {
	out := renderAgentReadme("dai", "review-agent", "codex", []string{"PROXY_TOKEN"}, nil)

	mustContain := []string{
		"# review-agent",
		"encave install github.com/dai/review-agent",
		"encave auth set --global",
		"`PROXY_TOKEN`",
		"encave dai/review-agent",
		"encave run", // interactive picker mention
		"encave publish dai/review-agent",
		"git@github.com:dai/review-agent.git",
		"fail-closed secret scan",
	}
	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("README missing %q\n---\n%s", s, out)
		}
	}
	if strings.Contains(out, "Required MCP servers") {
		t.Errorf("should not render MCP section when there are none:\n%s", out)
	}
}

func TestRenderAgentReadmeNoAuthVars(t *testing.T) {
	out := renderAgentReadme("bob", "plain-agent", "codex", nil, nil)
	if !strings.Contains(out, "does not declare an environment-based credential") {
		t.Errorf("expected no-credential note, got:\n%s", out)
	}
	if strings.Contains(out, "reads its credential from the following") {
		t.Errorf("should not list credential vars when none exist:\n%s", out)
	}
}

func TestRenderAgentReadmeListsMCPServers(t *testing.T) {
	mcps := []adapter.MCPServerInfo{
		{Name: "github", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-github"}},
		{Name: "linear", URL: "https://mcp.linear.app/sse"},
	}
	out := renderAgentReadme("dai", "review-agent", "codex", nil, mcps)

	for _, s := range []string{
		"## Required MCP servers",
		"not** bundled",
		"**github** — `npx -y @modelcontextprotocol/server-github`",
		"**linear** (remote) — `https://mcp.linear.app/sse`",
	} {
		if !strings.Contains(out, s) {
			t.Errorf("MCP README missing %q\n---\n%s", s, out)
		}
	}
}
