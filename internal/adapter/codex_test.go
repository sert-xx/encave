package adapter

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
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

func TestRegistryHasCodex(t *testing.T) {
	a, err := Get("codex")
	if err != nil {
		t.Fatal(err)
	}
	if a.HomeEnvVar() != "CODEX_HOME" {
		t.Fatalf("home env var = %q", a.HomeEnvVar())
	}
}
