package adapter

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestClaudeRegistered(t *testing.T) {
	a, err := Get("claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if a.HomeEnvVar() != "CLAUDE_CONFIG_DIR" {
		t.Fatalf("home env var = %q", a.HomeEnvVar())
	}
}

func TestClaudeManagesNoAuth(t *testing.T) {
	// encave never injects a Claude credential: AuthEnvVars is always empty,
	// regardless of the config passed in.
	got, err := ClaudeCode{}.AuthEnvVars([]byte(`{"env":{"ANTHROPIC_AUTH_TOKEN":"x"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no auth vars (encave does not manage Claude auth), got %v", got)
	}
}

func TestClaudeConfigLayout(t *testing.T) {
	base, eff := ClaudeCode{}.ConfigLayout()
	if base != "settings_base.json" || eff != "settings.json" {
		t.Fatalf("layout = (%q, %q)", base, eff)
	}
}

func TestClaudeBuildBaseConfigWhitelist(t *testing.T) {
	full := []byte(`{
		"model": "claude-opus-4-8",
		"permissions": {"allow": ["Read"]},
		"hooks": {"PreToolUse": []},
		"outputStyle": "concise",
		"env": {"ANTHROPIC_BASE_URL": "https://gw.example.com", "ANTHROPIC_AUTH_TOKEN": "secret"},
		"apiKeyHelper": "/usr/local/bin/key.sh",
		"forceLoginMethod": "console",
		"editorMode": "vim",
		"tui": "fullscreen"
	}`)
	out, err := ClaudeCode{}.BuildBaseConfig(full)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	for _, keep := range []string{"model", "permissions", "outputStyle"} {
		if _, ok := m[keep]; !ok {
			t.Errorf("whitelisted key %q was dropped", keep)
		}
	}
	// Environment/secret/identity/UI keys must never be packaged. "hooks" is
	// excluded too: it executes arbitrary shell commands, so packaging it would be
	// silent code execution on the consumer (same rationale as Codex mcp_servers).
	for _, drop := range []string{"hooks", "env", "apiKeyHelper", "forceLoginMethod", "editorMode", "tui"} {
		if _, ok := m[drop]; ok {
			t.Errorf("non-whitelisted key %q was kept (would leak environment/secret/personal config or enable RCE)", drop)
		}
	}
}

func TestClaudeBuildBaseConfigEmpty(t *testing.T) {
	// No source settings must still yield a valid JSON object, never a 0-byte
	// (invalid) file.
	out, err := ClaudeCode{}.BuildBaseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "{}" {
		t.Fatalf("expected \"{}\" for empty input, got %q", out)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("empty-input output is not valid JSON: %v", err)
	}
}

func TestClaudeManagedAuthAndNotes(t *testing.T) {
	c := ClaudeCode{}
	if c.ManagedAuth() {
		t.Fatal("claude-code auth must not be encave-managed")
	}
	notes := c.CredentialNotes("dai/review-agent")
	joined := strings.Join(notes, "\n")
	for _, want := range []string{"macOS", "claude setup-token", "CLAUDE_CODE_OAUTH_TOKEN", "dai/review-agent"} {
		if !strings.Contains(joined, want) {
			t.Errorf("CredentialNotes missing %q:\n%s", want, joined)
		}
	}
}

func TestClaudeExampleInvocation(t *testing.T) {
	got := ClaudeCode{}.ExampleInvocation()
	if len(got) == 0 || got[0] != "-p" {
		t.Fatalf("ExampleInvocation = %v, want it to start with -p", got)
	}
}

func TestClaudeBuildEffectiveConfigOverlay(t *testing.T) {
	base := []byte(`{"model": "agent-model", "permissions": {"allow": ["Read"]}}`)
	home := []byte(`{"model": "user-model", "env": {"ANTHROPIC_BASE_URL": "https://gw.example.com"}, "editorMode": "vim"}`)

	out, err := ClaudeCode{}.BuildEffectiveConfig(base, home)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	// Agent's whitelisted key wins.
	if m["model"] != "agent-model" {
		t.Errorf("model = %v, want agent-model (agent wins)", m["model"])
	}
	// User's environment keys are preserved from home.
	if _, ok := m["env"]; !ok {
		t.Error("user's env (e.g. ANTHROPIC_BASE_URL) should come from home settings")
	}
	if m["editorMode"] != "vim" {
		t.Error("user's editorMode should be preserved from home settings")
	}
	// Agent's permissions are present.
	if _, ok := m["permissions"]; !ok {
		t.Error("agent's permissions should be present")
	}
}

func TestClaudeBuildLaunch(t *testing.T) {
	spec, err := ClaudeCode{}.BuildLaunch(LaunchRequest{
		AgentDir: "/tmp/agent",
		Model:    "claude-opus-4-8",
		Sandbox:  "acceptEdits",
		UserArgs: []string{"-p", "review this diff"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Bin != "claude" {
		t.Fatalf("bin = %q", spec.Bin)
	}
	want := []string{
		"--model", "claude-opus-4-8",
		"--permission-mode", "acceptEdits",
		"-p", "review this diff",
	}
	if !reflect.DeepEqual(spec.Args, want) {
		t.Fatalf("args:\n got  %#v\n want %#v", spec.Args, want)
	}
}

func TestClaudeBuildLaunchRejectsRawConfig(t *testing.T) {
	_, err := ClaudeCode{}.BuildLaunch(LaunchRequest{RawConfig: []string{"model=x"}})
	if err == nil {
		t.Fatal("expected an error: claude-code has no -c override mechanism")
	}
}

func TestClaudeIgnoresStateAndSecrets(t *testing.T) {
	c := ClaudeCode{}

	// Credentials, app state, sessions, and local overrides must be both
	// scaffold-excluded and gitignored so they are never copied or committed.
	// Entries are root-anchored (leading "/") so they don't clobber like-named
	// author content deeper in the tree (verified in fsutil's copy tests).
	excludeWant := []string{
		"/.credentials.json", "/.claude.json", "/projects", "/todos",
		"/shell-snapshots", "/history", "/logs", "/statsig",
		"/settings.local.json", "/CLAUDE.local.md", "/settings.json",
	}
	for _, w := range excludeWant {
		if !slices.Contains(c.ScaffoldExcludes(), w) {
			t.Errorf("ScaffoldExcludes missing %q", w)
		}
	}
	gitignoreWant := []string{
		"/.credentials.json", "/.claude.json", "/projects/", "/todos/",
		"/shell-snapshots/", "/history/", "/logs/", "/statsig/",
		"/settings.local.json", "/CLAUDE.local.md", "/settings.json",
	}
	for _, w := range gitignoreWant {
		if !slices.Contains(c.GitignoreLines(), w) {
			t.Errorf("GitignoreLines missing %q", w)
		}
	}
}

func TestClaudeNoPersonalSubdirs(t *testing.T) {
	if subs := (ClaudeCode{}).PersonalSubdirs(); len(subs) != 0 {
		t.Fatalf("expected no personal subdirs for claude-code, got %v", subs)
	}
}
