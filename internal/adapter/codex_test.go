package adapter

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/BurntSushi/toml"
)

const sampleConfig = `
model = "some-model"

[model_providers.proxy]
name = "Internal Proxy"
base_url = "https://proxy.internal.example.com/v1"
env_key = "PROXY_TOKEN"
wire_api = "responses"

[model_providers.proxy.env_http_headers]
"X-Api-Key" = "PROXY_API_KEY"
"X-Tenant" = "PROXY_TENANT"
`

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCodexAuthEnvVars(t *testing.T) {
	dir := writeConfig(t, sampleConfig)
	got, err := Codex{}.AuthEnvVars(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"PROXY_API_KEY", "PROXY_TENANT", "PROXY_TOKEN"} // sorted
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCodexAuthEnvVarsNoProviders(t *testing.T) {
	dir := writeConfig(t, "model = \"x\"\n")
	got, err := Codex{}.AuthEnvVars(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no auth vars, got %v", got)
	}
}

func TestCodexBuildLaunch(t *testing.T) {
	spec, err := Codex{}.BuildLaunch(LaunchRequest{
		AgentDir:  "/tmp/agent",
		Model:     "my-model",
		Sandbox:   "workspace-write",
		RawConfig: []string{`model_providers.proxy.base_url="https://x"`},
		UserArgs:  []string{"exec", "do the thing"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Bin != "codex" {
		t.Fatalf("bin = %q", spec.Bin)
	}
	want := []string{
		"-c", `model="my-model"`,
		"-c", `sandbox_mode="workspace-write"`,
		"-c", `model_providers.proxy.base_url="https://x"`,
		"exec", "do the thing",
	}
	if !reflect.DeepEqual(spec.Args, want) {
		t.Fatalf("args:\n got  %#v\n want %#v", spec.Args, want)
	}
}

func TestTomlStringEscaping(t *testing.T) {
	if got := tomlString(`a"b\c`); got != `"a\"b\\c"` {
		t.Fatalf("tomlString = %s", got)
	}
}

func TestCodexPersonalSubdirsLinkedNotPackaged(t *testing.T) {
	c := Codex{}
	personal := c.PersonalSubdirs()
	if len(personal) == 0 {
		t.Fatal("expected at least one personal subdir (rules)")
	}
	has := func(list []string, want string) bool {
		for _, s := range list {
			if s == want {
				return true
			}
		}
		return false
	}
	for _, sub := range personal {
		if !has(c.ScaffoldExcludes(), sub) {
			t.Errorf("personal subdir %q must be excluded from scaffolding", sub)
		}
		// No trailing slash, so the symlink encave creates is also ignored.
		if !has(c.GitignoreLines(), sub) {
			t.Errorf("personal subdir %q must be gitignored (no trailing slash so the symlink matches)", sub)
		}
		if has(c.GitignoreLines(), sub+"/") {
			t.Errorf("personal subdir %q should be gitignored as %q (no slash), not %q", sub, sub, sub+"/")
		}
	}
	if !has(personal, "rules") {
		t.Errorf("expected 'rules' among personal subdirs, got %v", personal)
	}
}

func TestCodexIgnoresGeneratedState(t *testing.T) {
	c := Codex{}
	contains := func(list []string, want string) bool {
		for _, s := range list {
			if s == want {
				return true
			}
		}
		return false
	}

	// Machine-generated state that must be both scaffold-excluded and gitignored.
	excludeWant := []string{
		"auth.json", "history.jsonl", "sessions", "archived_sessions",
		"session_index.jsonl", "*.sqlite", "*.sqlite-wal", "*.sqlite-shm",
		"*.db", "version.json",
	}
	for _, w := range excludeWant {
		if !contains(c.ScaffoldExcludes(), w) {
			t.Errorf("ScaffoldExcludes missing %q", w)
		}
	}
	gitignoreWant := []string{
		"auth.json", "sessions/", "archived_sessions/", "session_index.jsonl",
		"*.sqlite", "*.sqlite-wal", "*.sqlite-shm", "*.db", "version.json",
	}
	for _, w := range gitignoreWant {
		if !contains(c.GitignoreLines(), w) {
			t.Errorf("GitignoreLines missing %q", w)
		}
	}
}

func TestCodexBuildBaseConfigWhitelist(t *testing.T) {
	full := []byte(`
model = "internal-model"
approval_policy = "on-request"

[model_providers.proxy]
base_url = "https://proxy.example.com/v1"
env_key = "PROXY_TOKEN"

[agents]
max_threads = 4

# not whitelisted — must be dropped (reusing others' MCP config is risky)
[mcp_servers.github]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-github"]

# environment/personal — must be dropped
[projects."/home/alice/secret-project"]
trust_level = "trusted"

[tui]
theme = "dark"
`)
	out, err := Codex{}.BuildBaseConfig(full)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := toml.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	for _, keep := range []string{"model", "approval_policy", "model_providers", "agents"} {
		if _, ok := m[keep]; !ok {
			t.Errorf("whitelisted key %q was dropped", keep)
		}
	}
	for _, drop := range []string{"projects", "tui", "mcp_servers"} {
		if _, ok := m[drop]; ok {
			t.Errorf("non-whitelisted key %q was kept", drop)
		}
	}
}

func TestCodexMCPServers(t *testing.T) {
	full := []byte(`
[mcp_servers.github]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-github"]

[mcp_servers.linear]
url = "https://mcp.linear.app/sse"
`)
	got, err := Codex{}.MCPServers(full)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d servers, want 2: %+v", len(got), got)
	}
	// sorted by name: github, linear
	if got[0].Name != "github" || got[0].Command != "npx" ||
		len(got[0].Args) != 2 || got[0].Args[0] != "-y" {
		t.Errorf("github server parsed wrong: %+v", got[0])
	}
	if got[1].Name != "linear" || got[1].URL != "https://mcp.linear.app/sse" {
		t.Errorf("linear server parsed wrong: %+v", got[1])
	}

	none, _ := Codex{}.MCPServers([]byte("model = \"x\"\n"))
	if none != nil {
		t.Errorf("expected nil for config without mcp_servers, got %+v", none)
	}
}

func TestCodexBuildEffectiveConfigOverlay(t *testing.T) {
	base := []byte("model = \"agent-model\"\n[model_providers.proxy]\nenv_key = \"PROXY_TOKEN\"\n")
	home := []byte("model = \"user-model\"\n[projects.\"/home/me/proj\"]\ntrust_level = \"trusted\"\n[tui]\ntheme = \"dark\"\n")

	out, err := Codex{}.BuildEffectiveConfig(base, home)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := toml.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	// Agent's whitelisted key wins.
	if m["model"] != "agent-model" {
		t.Errorf("model = %v, want agent-model (agent wins)", m["model"])
	}
	// User's environment keys are preserved from home.
	if _, ok := m["projects"]; !ok {
		t.Error("user's projects (trust) should come from home config")
	}
	if _, ok := m["tui"]; !ok {
		t.Error("user's tui settings should come from home config")
	}
	// Agent's provider config is present.
	if _, ok := m["model_providers"]; !ok {
		t.Error("agent's model_providers should be present")
	}
}

func TestCodexConfigLayout(t *testing.T) {
	base, eff := Codex{}.ConfigLayout()
	if base != "config_base.toml" || eff != "config.toml" {
		t.Fatalf("layout = (%q, %q)", base, eff)
	}
}

func TestRegistryHasCodex(t *testing.T) {
	a, err := Get("codex")
	if err != nil {
		t.Fatal(err)
	}
	if a.HomeEnvVar() != "CODEX_HOME" {
		t.Fatalf("home env var = %q", a.HomeEnvVar())
	}
}
