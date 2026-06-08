package adapter

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/BurntSushi/toml"
)

func init() { register(Codex{}) }

// codexPersonalSubdirs are home subdirectories that hold the user's own settings
// rather than the agent author's: Codex "rules" stores locally-approved commands.
// These are never packaged — they are symlinked from the user's base home at
// launch so each user's approvals apply to (and accumulate across) every agent.
var codexPersonalSubdirs = []string{"rules"}

// Codex config layout: the agent ships a whitelist-filtered config_base.toml
// (committed); at launch encave merges it over the user's own ~/.codex/config.toml
// to produce config.toml (gitignored), which is what Codex actually reads. This
// keeps environment/personal settings (project trust, UI, telemetry, paths) on
// the user's side while the agent owns its defining keys.
const (
	codexBaseConfig      = "config_base.toml"
	codexEffectiveConfig = "config.toml"
)

// codexConfigWhitelist is the set of top-level config.toml keys an agent owns and
// ships. Everything else is treated as environment/personal and comes from the
// user's home at launch. Derived from the Codex ConfigToml struct
// (codex-rs/core/src/config/mod.rs); unknown/new keys default to NOT packaged.
var codexConfigWhitelist = map[string]bool{
	// Model (NOT model_provider / model_providers — those are environment-specific;
	// see note below)
	"model": true, "review_model": true,
	"model_reasoning_effort": true, "plan_mode_reasoning_effort": true,
	"model_reasoning_summary": true, "model_supports_reasoning_summaries": true,
	"model_verbosity": true, "model_context_window": true,
	"model_auto_compact_token_limit": true, "model_auto_compact_token_limit_scope": true,
	"service_tier": true, "personality": true, "oss_provider": true,
	"model_catalog_json": true,
	// Instructions & project docs
	"base_instructions": true, "developer_instructions": true, "compact_prompt": true,
	"project_doc_max_bytes": true, "project_doc_fallback_filenames": true,
	"include_permissions_instructions": true, "include_apps_instructions": true,
	"include_collaboration_mode_instructions": true, "include_skill_instructions": true,
	"include_environment_context": true,
	// Safety posture (author's intended permissions/sandbox).
	// NOT sandbox_workspace_write — it carries environment-specific local paths.
	"sandbox_mode": true, "default_permissions": true,
	"permissions": true, "profiles": true, "shell_environment_policy": true,
	"approval_policy": true, "approvals_reviewer": true,
	// Orchestration
	"agents": true, "agent_roles": true, "agent_max_threads": true,
	"agent_max_depth": true, "agent_job_max_runtime_seconds": true,
	"agent_interrupt_message_enabled": true, "multi_agent_v2": true,
	// Tools & capabilities
	"tools": true, "code_mode": true,
	"use_experimental_unified_exec_tool": true, "background_terminal_max_timeout": true,
	"web_search": true, "web_search_config": true, "features": true,
	// Project detection
	"project_root_markers": true,
}

// Note: several keys are intentionally NOT whitelisted because they are
// environment-specific or risky to inherit from someone else:
//   - mcp_servers: can run arbitrary local commands/endpoints.
//   - model_provider / model_providers: provider wiring (base URL, auth env var,
//     wire protocol) belongs to the executor's environment.
//   - sandbox_workspace_write: carries absolute local writable paths.
// These come from the user's own home config at launch; `new` lists the author's
// MCP servers and model providers in the README as setup requirements (see
// MCPServers / ModelProviders).

// ConfigLayout implements Adapter: Codex uses the base/effective split.
func (Codex) ConfigLayout() (base, effective string) {
	return codexBaseConfig, codexEffectiveConfig
}

// BuildBaseConfig implements Adapter: keep only whitelisted top-level keys from a
// full ~/.codex/config.toml so the packaged config carries just the agent's own
// settings.
func (Codex) BuildBaseConfig(full []byte) ([]byte, error) {
	if len(bytes.TrimSpace(full)) == 0 {
		return nil, nil
	}
	var m map[string]any
	if err := toml.Unmarshal(full, &m); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if codexConfigWhitelist[k] {
			out[k] = v
		}
	}
	return encodeTOML(out)
}

// BuildEffectiveConfig implements Adapter: overlay the agent's base config over
// the user's home config at the top level. The agent's keys win; every other key
// (project trust, UI, paths, telemetry, …) comes from the user's home.
func (Codex) BuildEffectiveConfig(base, home []byte) ([]byte, error) {
	merged := map[string]any{}
	if len(bytes.TrimSpace(home)) > 0 {
		if err := toml.Unmarshal(home, &merged); err != nil {
			return nil, fmt.Errorf("parsing home config: %w", err)
		}
	}
	if len(bytes.TrimSpace(base)) > 0 {
		var bm map[string]any
		if err := toml.Unmarshal(base, &bm); err != nil {
			return nil, fmt.Errorf("parsing base config: %w", err)
		}
		for k, v := range bm {
			merged[k] = v // agent's keys win (full top-level replace)
		}
	}
	return encodeTOML(merged)
}

// encodeTOML serializes a map to TOML bytes using [table] headers with dotted
// section names and plain keys inside (no indentation). It verifies the output
// round-trips back to the same data; if anything is off, it falls back to the
// standard encoder without indentation, so output is always valid.
func encodeTOML(m map[string]any) ([]byte, error) {
	if out, err := encodeSectionedTOML(m); err == nil {
		var back map[string]any
		if err := toml.Unmarshal(out, &back); err == nil && reflect.DeepEqual(m, back) {
			return out, nil
		}
	}
	// Fallback: standard encoder, but without indentation.
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.Indent = ""
	if err := enc.Encode(m); err != nil {
		return nil, fmt.Errorf("encoding config: %w", err)
	}
	return buf.Bytes(), nil
}

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

// Validate implements Adapter. A Codex agent has either the packaged base config
// (new layout) or a config.toml (legacy / generated effective config).
func (Codex) Validate(agentDir string) error {
	info, err := os.Stat(agentDir)
	if err != nil {
		return fmt.Errorf("agent home %q: %w", agentDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("agent home %q is not a directory", agentDir)
	}
	if _, err := os.Stat(filepath.Join(agentDir, codexBaseConfig)); err == nil {
		return nil
	}
	if _, err := os.Stat(filepath.Join(agentDir, codexEffectiveConfig)); err == nil {
		return nil
	}
	return fmt.Errorf("agent home %q has no %s or %s (not a valid Codex home?)", agentDir, codexBaseConfig, codexEffectiveConfig)
}

// AuthEnvVars implements Adapter by collecting every env var name referenced by
// a model provider's env_key or env_http_headers. Provider config is not
// packaged, so callers pass the effective/merged config (which includes the
// user's home providers) at launch, or the source config when generating docs.
func (Codex) AuthEnvVars(configData []byte) ([]string, error) {
	if len(bytes.TrimSpace(configData)) == 0 {
		return nil, nil
	}
	var raw map[string]any
	if err := toml.Unmarshal(configData, &raw); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
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

// MCPServers implements Adapter by reading [mcp_servers.<name>] tables from a
// full config and returning each server's launch command/args or URL.
func (Codex) MCPServers(configData []byte) ([]MCPServerInfo, error) {
	if len(bytes.TrimSpace(configData)) == 0 {
		return nil, nil
	}
	var raw map[string]any
	if err := toml.Unmarshal(configData, &raw); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	servers, ok := raw["mcp_servers"].(map[string]any)
	if !ok {
		return nil, nil
	}
	out := make([]MCPServerInfo, 0, len(servers))
	for name, sv := range servers {
		info := MCPServerInfo{Name: name}
		if s, ok := sv.(map[string]any); ok {
			if c, ok := s["command"].(string); ok {
				info.Command = c
			}
			if u, ok := s["url"].(string); ok {
				info.URL = u
			}
			if args, ok := s["args"].([]any); ok {
				for _, a := range args {
					if as, ok := a.(string); ok {
						info.Args = append(info.Args, as)
					}
				}
			}
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ModelProviders implements Adapter by reading [model_providers.<name>] tables
// from a full config and returning each provider's base_url, wire_api and the
// env var its token is read from.
func (Codex) ModelProviders(configData []byte) ([]ProviderInfo, error) {
	if len(bytes.TrimSpace(configData)) == 0 {
		return nil, nil
	}
	var raw map[string]any
	if err := toml.Unmarshal(configData, &raw); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	providers, ok := raw["model_providers"].(map[string]any)
	if !ok {
		return nil, nil
	}
	out := make([]ProviderInfo, 0, len(providers))
	for name, pv := range providers {
		info := ProviderInfo{Name: name}
		if p, ok := pv.(map[string]any); ok {
			if s, ok := p["base_url"].(string); ok {
				info.BaseURL = s
			}
			if s, ok := p["wire_api"].(string); ok {
				info.WireAPI = s
			}
			if s, ok := p["env_key"].(string); ok {
				info.EnvKey = s
			}
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
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

// What encave manages vs. ignores for a Codex home (verified against Codex docs
// and the openai/codex issue tracker — see PR discussion):
//
//   - Managed (packaged): the author's tuned configuration — config.toml and
//     <profile>.config.toml, AGENTS.md / AGENTS.override.md, prompts/, agents/
//     (subagent definitions), skills/, and any other authored files.
//   - Personal (symlinked, never packaged): rules/ (see codexPersonalSubdirs).
//   - Ignored (machine-generated state/secrets): auth.json; history.jsonl;
//     sessions/, archived_sessions/, session_index.jsonl, *.session.jsonl; the
//     SQLite state/log databases (state_*.sqlite, logs_*.sqlite) and their
//     WAL/SHM sidecars; log/ logs/ *.log; cache/ .cache/ tmp/; version.json.

// ScaffoldExcludes implements Adapter. Best-effort removal of secrets, state,
// history, databases, logs, caches, regenerable artifacts, and the user's
// personal subdirs when copying a user's ~/.codex into a new agent.
func (Codex) ScaffoldExcludes() []string {
	out := []string{
		// Credentials
		"auth.json",
		// History, session transcripts and indices
		"history.jsonl",
		"*.session.jsonl",
		"sessions",
		"archived_sessions",
		"session_index.jsonl",
		// SQLite state/log DBs (e.g. state_5.sqlite, logs_2.sqlite) + WAL/SHM sidecars
		"*.sqlite",
		"*.sqlite-wal",
		"*.sqlite-shm",
		"*.db",
		// Logs
		"log",
		"logs",
		"*.log",
		// Caches, temp, regenerable
		"cache",
		".cache",
		"tmp",
		".tmp",
		"version.json",
		// The effective config.toml is generated at launch (base config + the
		// user's home config); `new` writes config_base.toml separately, so the
		// raw config.toml is never copied.
		"config.toml",
		// Never copy a stray repo from the base home
		".git",
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
		"# encave: never commit Codex credentials, history, session state, or databases",
		"auth.json",
		"history.jsonl",
		"*.session.jsonl",
		"sessions/",
		"archived_sessions/",
		"session_index.jsonl",
		"*.sqlite",
		"*.sqlite-wal",
		"*.sqlite-shm",
		"*.db",
		"logs/",
		"log/",
		"*.log",
		"cache/",
		".cache/",
		"tmp/",
		"version.json",
		"# encave: generated at launch (config_base.toml merged with your ~/.codex/config.toml)",
		"config.toml",
		"# encave: personal settings — symlinked from your base home at launch",
	}
	for _, sub := range codexPersonalSubdirs {
		// No trailing slash: this also matches the symlink encave creates at
		// new/install/run (a symlink is a file to git, so "rules/" would not).
		out = append(out, sub)
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
