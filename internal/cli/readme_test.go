package cli

import (
	"strings"
	"testing"

	"github.com/sert-xx/encave/internal/adapter"
)

func TestRenderAgentReadmeWithAuthVars(t *testing.T) {
	out := renderAgentReadme("dai", "review-agent", "codex", []string{"PROXY_TOKEN"}, nil, nil)

	mustContain := []string{
		"# review-agent",
		"encave install github.com/dai/review-agent",
		"encave auth set --global",
		"ベアラートークン", // credentials section (JA)
		"encave dai/review-agent",
		"encave run", // interactive picker mention
		"encave publish dai/review-agent",
		"git@github.com:dai/review-agent.git",
		"秘密スキャン", // fail-closed secret scan (JA)
	}
	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("README missing %q\n---\n%s", s, out)
		}
	}
	if strings.Contains(out, "必要な MCP サーバー") {
		t.Errorf("should not render MCP section when there are none:\n%s", out)
	}
}

func TestRenderAgentReadmeNoCredential(t *testing.T) {
	out := renderAgentReadme("bob", "plain-agent", "codex", nil, nil, nil)
	if !strings.Contains(out, "トークンを要するモデルプロバイダを宣言していません") {
		t.Errorf("expected no-credential note, got:\n%s", out)
	}
}

func TestRenderAgentReadmeListsModelProviders(t *testing.T) {
	providers := []adapter.ProviderInfo{
		{Name: "proxy", BaseURL: "https://proxy.example.com/v1", WireAPI: "responses", EnvKey: "PROXY_TOKEN"},
	}
	out := renderAgentReadme("dai", "review-agent", "codex", []string{"PROXY_TOKEN"}, providers, nil)
	for _, s := range []string{
		"## モデルプロバイダ",
		"含まれていません",
		"**proxy**",
		"base_url `https://proxy.example.com/v1`",
		"wire_api `responses`",
		"認証トークンの配線は encave",
	} {
		if !strings.Contains(out, s) {
			t.Errorf("provider README missing %q\n---\n%s", s, out)
		}
	}
	// encave forces its own env var, so the author's env_key is not surfaced.
	if strings.Contains(out, "PROXY_TOKEN") {
		t.Errorf("should not surface the author's env_key:\n%s", out)
	}
}

func TestRenderAgentReadmeListsMCPServers(t *testing.T) {
	mcps := []adapter.MCPServerInfo{
		{Name: "github", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-github"}},
		{Name: "linear", URL: "https://mcp.linear.app/sse"},
	}
	out := renderAgentReadme("dai", "review-agent", "codex", nil, nil, mcps)

	for _, s := range []string{
		"## 必要な MCP サーバー",
		"含まれません",
		"**github** — `npx -y @modelcontextprotocol/server-github`",
		"**linear**（リモート） — `https://mcp.linear.app/sse`",
	} {
		if !strings.Contains(out, s) {
			t.Errorf("MCP README missing %q\n---\n%s", s, out)
		}
	}
}
