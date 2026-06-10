package adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func init() { register(ClaudeCode{}) }

// ClaudeCode is the adapter for the Claude Code CLI, encave's second target.
//
// Key facts the adapter encodes (verify against current Claude Code docs before
// relying on them — see code.claude.com/docs):
//   - Home directory variable: CLAUDE_CONFIG_DIR (overrides the default
//     ~/.claude). When set, Claude Code reads its user-level settings.json,
//     CLAUDE.md, skills/, etc. from there, and writes its state/credentials
//     there too.
//   - Settings are JSON (settings.json), not TOML.
//   - Authentication is deliberately NOT managed by encave for this target (see
//     AuthEnvVars). On macOS the user's existing /login lives in the OS Keychain
//     (independent of CLAUDE_CONFIG_DIR), so an isolated home stays logged in;
//     on Linux/Windows the credential file moves with CLAUDE_CONFIG_DIR, so the
//     user simply re-authenticates (or sets CLAUDE_CODE_OAUTH_TOKEN themselves)
//     inside the isolated home. encave never stores or injects a Claude
//     credential.
type ClaudeCode struct{}

// claudeBaseConfig / claudeEffectiveConfig: the agent ships a whitelist-filtered
// settings_base.json (committed); at launch encave merges it over the user's own
// ~/.claude/settings.json to produce settings.json (gitignored), which is what
// Claude Code actually reads. This mirrors the Codex base/effective split and
// keeps environment/personal settings (env, login, UI prefs) on the user's side
// while the agent owns its defining keys.
const (
	claudeBaseConfig      = "settings_base.json"
	claudeEffectiveConfig = "settings.json"
)

// claudeConfigWhitelist is the set of top-level settings.json keys an agent owns
// and ships. Everything else is treated as environment/personal and comes from
// the user's home at launch. The list is conservative and behavior-focused;
// unknown/new keys default to NOT packaged (like the Codex adapter). Notably
// excluded: env (API base URL/keys, timeouts), apiKeyHelper, forceLoginMethod /
// forceLoginOrgUUID and other login/identity keys, managed-org keys, UI prefs
// (editorMode, language, tui, statusLine), and retention/marketplace settings.
var claudeConfigWhitelist = map[string]bool{
	// Model & reasoning
	"model": true, "effortLevel": true, "alwaysThinkingEnabled": true,
	// Tool/file access posture (the author's intended permissions)
	"permissions": true,
	// Automation
	"hooks": true,
	// Behavior / output
	"outputStyle": true, "includeCoAuthoredBy": true,
	// Memory behavior (not the storage location, which is environment-specific)
	"autoMemoryEnabled": true, "claudeMdExcludes": true,
}

// Name implements Adapter.
func (ClaudeCode) Name() string { return "claude-code" }

// HomeEnvVar implements Adapter.
func (ClaudeCode) HomeEnvVar() string { return "CLAUDE_CONFIG_DIR" }

// BaseHome implements Adapter. It honors CLAUDE_CONFIG_DIR, then falls back to
// ~/.claude.
func (ClaudeCode) BaseHome() (string, error) {
	if h := os.Getenv("CLAUDE_CONFIG_DIR"); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating home directory: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

// Validate implements Adapter. A Claude Code agent has either the packaged base
// settings (new layout), a generated settings.json (legacy/effective), or at
// least a CLAUDE.md — any of which marks a plausible Claude home.
func (ClaudeCode) Validate(agentDir string) error {
	info, err := os.Stat(agentDir)
	if err != nil {
		return fmt.Errorf("agent home %q: %w", agentDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("agent home %q is not a directory", agentDir)
	}
	for _, marker := range []string{claudeBaseConfig, claudeEffectiveConfig, "CLAUDE.md"} {
		if _, err := os.Stat(filepath.Join(agentDir, marker)); err == nil {
			return nil
		}
	}
	return fmt.Errorf("agent home %q has no %s, %s or CLAUDE.md (not a valid Claude Code home?)", agentDir, claudeBaseConfig, claudeEffectiveConfig)
}

// ManagedAuth implements Adapter: encave does not manage Claude Code's
// credential — the target uses its own login (see AuthEnvVars).
func (ClaudeCode) ManagedAuth() bool { return false }

// AuthEnvVars implements Adapter. encave does NOT manage credentials for Claude
// Code, so it returns no env vars: nothing is injected, and the launcher relies
// on the user's own login (macOS Keychain) or a credential they set inside the
// isolated home (Linux/Windows re-login, or CLAUDE_CODE_OAUTH_TOKEN). The config
// bytes are intentionally unused.
func (ClaudeCode) AuthEnvVars(_ []byte) ([]string, error) { return nil, nil }

// ConfigLayout implements Adapter: Claude Code uses the base/effective split.
func (ClaudeCode) ConfigLayout() (base, effective string) {
	return claudeBaseConfig, claudeEffectiveConfig
}

// BuildBaseConfig implements Adapter: keep only whitelisted top-level keys from a
// full ~/.claude/settings.json so the packaged config carries just the agent's
// own settings.
func (ClaudeCode) BuildBaseConfig(full []byte) ([]byte, error) {
	if len(bytes.TrimSpace(full)) == 0 {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal(full, &m); err != nil {
		return nil, fmt.Errorf("parsing settings: %w", err)
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if claudeConfigWhitelist[k] {
			out[k] = v
		}
	}
	return encodeJSON(out)
}

// BuildEffectiveConfig implements Adapter: overlay the agent's base settings over
// the user's home settings at the top level. The agent's keys win; every other
// key (env, login, UI prefs, …) comes from the user's home.
func (ClaudeCode) BuildEffectiveConfig(base, home []byte) ([]byte, error) {
	merged := map[string]any{}
	if len(bytes.TrimSpace(home)) > 0 {
		if err := json.Unmarshal(home, &merged); err != nil {
			return nil, fmt.Errorf("parsing home settings: %w", err)
		}
	}
	if len(bytes.TrimSpace(base)) > 0 {
		var bm map[string]any
		if err := json.Unmarshal(base, &bm); err != nil {
			return nil, fmt.Errorf("parsing base settings: %w", err)
		}
		for k, v := range bm {
			merged[k] = v // agent's keys win (full top-level replace)
		}
	}
	return encodeJSON(merged)
}

// MCPServers implements Adapter. Claude Code stores user-scope MCP servers in
// ~/.claude.json (machine state, not packaged) rather than in settings.json, so
// there is nothing to surface from the settings config this method receives.
func (ClaudeCode) MCPServers(_ []byte) ([]MCPServerInfo, error) { return nil, nil }

// ModelProviders implements Adapter. Claude Code has no Codex-style model
// provider tables; the endpoint is environment-specific (ANTHROPIC_BASE_URL) and
// owned by the running user, so there is nothing to package or document here.
func (ClaudeCode) ModelProviders(_ []byte) ([]ProviderInfo, error) { return nil, nil }

// BuildLaunch implements Adapter. encave conveniences are translated to Claude
// Code's flags and placed before any user-supplied arguments so they apply
// regardless of subcommand. Claude has no Codex-style `-c key=value` mechanism;
// raw config overrides are rejected with a pointer to passing native flags after
// `--` instead.
func (ClaudeCode) BuildLaunch(req LaunchRequest) (LaunchSpec, error) {
	if len(req.RawConfig) > 0 {
		return LaunchSpec{}, fmt.Errorf("the claude-code target does not support -c overrides; pass native claude flags after `--` instead")
	}
	var args []string
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.Sandbox != "" {
		args = append(args, "--permission-mode", req.Sandbox)
	}
	args = append(args, req.UserArgs...)
	return LaunchSpec{Bin: "claude", Args: args}, nil
}

// What encave manages vs. ignores for a Claude Code home:
//
//   - Managed (packaged): the author's tuned configuration — settings_base.json
//     (filtered), CLAUDE.md, skills/, agents/ (subagents), commands/ (slash
//     commands), rules/, output-styles/, and any other authored files.
//   - Ignored (machine-generated state/secrets): .credentials.json (login),
//     .claude.json (app state + OAuth session + personal MCP servers),
//     projects/ (session transcripts), todos/, shell-snapshots/, history/,
//     logs/, statsig/ (feature-flag cache), and the user's local overrides
//     (settings.local.json, CLAUDE.local.md).

// ScaffoldExcludes implements Adapter. Best-effort removal of credentials, app
// state, session transcripts, caches, logs, and local overrides when copying a
// user's ~/.claude into a new agent.
func (ClaudeCode) ScaffoldExcludes() []string {
	return []string{
		// Credentials & app state (OAuth session, personal MCP servers, caches)
		".credentials.json",
		".claude.json",
		// Session transcripts, todos, shell snapshots, history
		"projects",
		"todos",
		"shell-snapshots",
		"history",
		// Logs and feature-flag cache
		"logs",
		"statsig",
		// Local, personal overrides (never shared)
		"settings.local.json",
		"CLAUDE.local.md",
		// The effective settings.json is generated at launch (base + the user's
		// home settings); new writes settings_base.json separately.
		"settings.json",
		// Never copy a stray repo from the base home
		".git",
	}
}

// PersonalSubdirs implements Adapter. Claude Code has no Codex-style per-user
// "approved commands" directory to symlink across agents (personal overrides live
// in settings.local.json, which is excluded), so there are none.
func (ClaudeCode) PersonalSubdirs() []string { return nil }

// GitignoreLines implements Adapter.
func (ClaudeCode) GitignoreLines() []string {
	return []string{
		"# encave: never commit Claude Code credentials, app state, sessions, or caches",
		".credentials.json",
		".claude.json",
		"projects/",
		"todos/",
		"shell-snapshots/",
		"history/",
		"logs/",
		"statsig/",
		"settings.local.json",
		"CLAUDE.local.md",
		"# encave: generated at launch (settings_base.json merged with your ~/.claude/settings.json)",
		"settings.json",
	}
}

// encodeJSON serializes a map to indented JSON bytes with a trailing newline.
func encodeJSON(m map[string]any) ([]byte, error) {
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding settings: %w", err)
	}
	return append(out, '\n'), nil
}
