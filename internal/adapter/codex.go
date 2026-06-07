package adapter

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
)

func init() { register(Codex{}) }

// codexPersonalSubdirs are home subdirectories that hold the user's own settings
// rather than the agent author's: Codex "rules" stores locally-approved commands.
// These are never packaged — they are symlinked from the user's base home at
// launch so each user's approvals apply to (and accumulate across) every agent.
var codexPersonalSubdirs = []string{"rules"}

// Codex is the adapter for the Codex CLI, encave's first target.
//
// Key facts the adapter encodes (verify against current Codex docs before
// relying on them — see design doc §11):
//   - Home directory variable: CODEX_HOME (default ~/.codex).
//   - Custom model providers live under [model_providers.<id>] in config.toml.
//     A provider reads its bearer token from the env var named by `env_key`, and
//     any custom auth headers from `env_http_headers` (header name -> env var
//     name). encave injects keyring values into exactly those env vars.
//   - Config overrides use `-c key=value` where value is a TOML literal, so
//     string values must carry their own quotes (e.g. -c model='"some-model"').
type Codex struct{}

// Name implements Adapter.
func (Codex) Name() string { return "codex" }

// HomeEnvVar implements Adapter.
func (Codex) HomeEnvVar() string { return "CODEX_HOME" }

// BaseHome implements Adapter. It honors CODEX_HOME, then falls back to
// ~/.codex.
func (Codex) BaseHome() (string, error) {
	if h := os.Getenv("CODEX_HOME"); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating home directory: %w", err)
	}
	return filepath.Join(home, ".codex"), nil
}

// Validate implements Adapter. A Codex home is expected to contain config.toml.
func (Codex) Validate(agentDir string) error {
	info, err := os.Stat(agentDir)
	if err != nil {
		return fmt.Errorf("agent home %q: %w", agentDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("agent home %q is not a directory", agentDir)
	}
	cfg := filepath.Join(agentDir, "config.toml")
	if _, err := os.Stat(cfg); err != nil {
		return fmt.Errorf("agent home %q has no config.toml (not a valid Codex home?)", agentDir)
	}
	return nil
}

// AuthEnvVars implements Adapter by reading config.toml and collecting every
// env var name referenced by a model provider's env_key or env_http_headers.
func (Codex) AuthEnvVars(agentDir string) ([]string, error) {
	cfgPath := filepath.Join(agentDir, "config.toml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", cfgPath, err)
	}
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", cfgPath, err)
	}

	providers, ok := raw["model_providers"].(map[string]any)
	if !ok {
		return nil, nil
	}

	seen := map[string]struct{}{}
	for _, pv := range providers {
		prov, ok := pv.(map[string]any)
		if !ok {
			continue
		}
		if ek, ok := prov["env_key"].(string); ok && ek != "" {
			seen[ek] = struct{}{}
		}
		if hdrs, ok := prov["env_http_headers"].(map[string]any); ok {
			for _, v := range hdrs {
				if name, ok := v.(string); ok && name != "" {
					seen[name] = struct{}{}
				}
			}
		}
	}

	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

// BuildLaunch implements Adapter. It places config overrides (as `-c` global
// options) before any user-supplied arguments so they apply regardless of
// whether the user invokes the TUI or a subcommand like `exec`.
func (Codex) BuildLaunch(req LaunchRequest) (LaunchSpec, error) {
	var args []string

	// Dedicated encave conveniences, translated to Codex's TOML override syntax.
	if req.Model != "" {
		args = append(args, "-c", "model="+tomlString(req.Model))
	}
	if req.Sandbox != "" {
		args = append(args, "-c", "sandbox_mode="+tomlString(req.Sandbox))
	}
	// Raw passthrough overrides are already in Codex's `key=value` TOML form.
	for _, c := range req.RawConfig {
		args = append(args, "-c", c)
	}

	args = append(args, req.UserArgs...)

	return LaunchSpec{Bin: "codex", Args: args}, nil
}

// ScaffoldExcludes implements Adapter. Best-effort removal of secrets, state,
// logs, regenerable artifacts, and the user's personal subdirs when copying a
// user's ~/.codex into a new agent.
func (Codex) ScaffoldExcludes() []string {
	out := []string{
		"auth.json",       // OpenAI/login credentials
		"history.jsonl",   // prompt history
		"*.session.jsonl", // session transcripts
		"sessions",        // session storage dir
		"log",             // log dir/file
		"logs",
		"*.log",
		"cache",
		".cache",
		"tmp",
		".tmp",
		"version.json", // regenerable metadata
		".git",         // never copy a stray repo from the base home
	}
	// Personal subdirs (e.g. rules) are linked at launch, never packaged.
	return append(out, codexPersonalSubdirs...)
}

// PersonalSubdirs implements Adapter.
func (Codex) PersonalSubdirs() []string {
	return append([]string(nil), codexPersonalSubdirs...)
}

// GitignoreLines implements Adapter.
func (Codex) GitignoreLines() []string {
	out := []string{
		"# encave: never commit Codex credentials or local state",
		"auth.json",
		"history.jsonl",
		"*.session.jsonl",
		"sessions/",
		"logs/",
		"log/",
		"*.log",
		"cache/",
		".cache/",
		"tmp/",
		"# encave: personal settings — symlinked from your base home at launch",
	}
	for _, sub := range codexPersonalSubdirs {
		out = append(out, sub+"/")
	}
	return out
}

// tomlString renders s as a TOML basic string literal (with surrounding quotes
// and escaping) suitable for a Codex `-c key=value` override.
func tomlString(s string) string {
	out := make([]rune, 0, len(s)+2)
	out = append(out, '"')
	for _, r := range s {
		switch r {
		case '"':
			out = append(out, '\\', '"')
		case '\\':
			out = append(out, '\\', '\\')
		case '\n':
			out = append(out, '\\', 'n')
		case '\t':
			out = append(out, '\\', 't')
		case '\r':
			out = append(out, '\\', 'r')
		default:
			out = append(out, r)
		}
	}
	out = append(out, '"')
	return string(out)
}
