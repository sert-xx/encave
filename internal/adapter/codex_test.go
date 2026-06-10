package adapter

import (
	"reflect"
	"slices"
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

func TestCodexAuthEnvVars(t *testing.T) {
	got, err := Codex{}.AuthEnvVars([]byte(sampleConfig))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"PROXY_API_KEY", "PROXY_TENANT", "PROXY_TOKEN"} // sorted
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCodexAuthEnvVarsNoProviders(t *testing.T) {
	got, err := Codex{}.AuthEnvVars([]byte("model = \"x\"\n"))
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
	for _, sub := range personal {
		if !slices.Contains(c.ScaffoldExcludes(), sub) {
			t.Errorf("personal subdir %q must be excluded from scaffolding", sub)
		}
		// No trailing slash, so the symlink encave creates is also ignored.
		if !slices.Contains(c.GitignoreLines(), sub) {
			t.Errorf("personal subdir %q must be gitignored (no trailing slash so the symlink matches)", sub)
		}
		if slices.Contains(c.GitignoreLines(), sub+"/") {
			t.Errorf("personal subdir %q should be gitignored as %q (no slash), not %q", sub, sub, sub+"/")
		}
	}
	if !slices.Contains(personal, "rules") {
		t.Errorf("expected 'rules' among personal subdirs, got %v", personal)
	}
}

func TestCodexIgnoresGeneratedState(t *testing.T) {
	c := Codex{}

	// Machine-generated state that must be both scaffold-excluded and gitignored.
	excludeWant := []string{
		"auth.json", "history.jsonl", "sessions", "archived_sessions",
		"session_index.jsonl", "*.sqlite", "*.sqlite-wal", "*.sqlite-shm",
		"*.db", "version.json",
	}
	for _, w := range excludeWant {
		if !slices.Contains(c.ScaffoldExcludes(), w) {
			t.Errorf("ScaffoldExcludes missing %q", w)
		}
	}
	gitignoreWant := []string{
		"auth.json", "sessions/", "archived_sessions/", "session_index.jsonl",
		"*.sqlite", "*.sqlite-wal", "*.sqlite-shm", "*.db", "version.json",
	}
	for _, w := range gitignoreWant {
		if !slices.Contains(c.GitignoreLines(), w) {
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
	for _, keep := range []string{"model", "approval_policy", "agents"} {
		if _, ok := m[keep]; !ok {
			t.Errorf("whitelisted key %q was dropped", keep)
		}
	}
	for _, drop := range []string{"projects", "tui", "mcp_servers", "model_providers", "model_provider", "sandbox_workspace_write"} {
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

func TestCodexBuildEffectiveConfigForcesAuthWiring(t *testing.T) {
	// Provider from the user's home with NO env_key, plus Codex's own credential
	// store enabled. The generated config must drop the store and force env_key.
	home := []byte(`
cli_auth_credentials_store = "keyring"
cli_auth_credentials_store_mode = "auto"
model_provider = "proxy"

[model_providers.proxy]
base_url = "https://proxy.example.com/v1"
wire_api = "responses"
`)
	base := []byte(`model = "agent-model"` + "\n")

	out, err := Codex{}.BuildEffectiveConfig(base, home)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := toml.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["cli_auth_credentials_store"]; ok {
		t.Error("cli_auth_credentials_store should be removed")
	}
	if _, ok := m["cli_auth_credentials_store_mode"]; ok {
		t.Error("cli_auth_credentials_store_mode should be removed")
	}
	mp := m["model_providers"].(map[string]any)
	proxy := mp["proxy"].(map[string]any)
	if proxy["env_key"] != codexInjectedEnvKey {
		t.Errorf("provider env_key = %v, want %q", proxy["env_key"], codexInjectedEnvKey)
	}
	// base_url preserved from home.
	if proxy["base_url"] != "https://proxy.example.com/v1" {
		t.Errorf("base_url not preserved: %v", proxy["base_url"])
	}

	// And auth discovery on the generated config returns the injected env var.
	authVars, err := Codex{}.AuthEnvVars(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(authVars) != 1 || authVars[0] != codexInjectedEnvKey {
		t.Errorf("AuthEnvVars = %v, want [%s]", authVars, codexInjectedEnvKey)
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
