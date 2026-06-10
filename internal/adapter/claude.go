package adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
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
//
// "hooks" is deliberately NOT packaged: Claude Code hooks run arbitrary shell
// commands automatically on lifecycle events, with no model mediation or per-run
// approval. Shipping them would let an agent author execute code on a consumer's
// machine at install/run time — the same reason the Codex adapter refuses to
// package mcp_servers (see codex.go). Authors who rely on hooks document them in
// the agent README as a setup step for the consumer's own settings.
var claudeConfigWhitelist = map[string]bool{
	// Model & reasoning
	"model": true, "effortLevel": true, "alwaysThinkingEnabled": true,
	// Tool/file access posture (the author's intended permissions)
	"permissions": true,
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
func (ClaudeCode) BaseHome() (string, error) { return baseHome("CLAUDE_CONFIG_DIR", ".claude") }

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

// CredentialNotes implements Adapter: Claude Code's credential is not managed by
// encave, so the README explains how to authenticate the isolated home per OS.
func (ClaudeCode) CredentialNotes(ref string) []string {
	return []string{
		"このターゲットの認証情報は encave では管理しません。ターゲット CLI 自身のログインを",
		"使います:",
		"",
		"- **macOS**: 通常の `claude /login`（OS の Keychain に保存）がそのまま使えます。",
		"  encave が隔離ホームを使っても、Keychain はグローバルなのでログイン状態を保てます。",
		"- **Linux / Windows**: 隔離ホームは最初ログアウト状態です。中で一度だけ認証してください",
		"  （`encave " + ref + " -- /login`、または `claude setup-token` で得た",
		"  `CLAUDE_CODE_OAUTH_TOKEN` を設定）。",
		"",
		"> **TODO:** 接続先ゲートウェイがある場合は `ANTHROPIC_BASE_URL`（環境固有・非梱包）の",
		"> 設定方法を記載してください。",
	}
}

// ExampleInvocation implements Adapter: Claude Code's non-interactive flag.
func (ClaudeCode) ExampleInvocation() []string { return []string{"-p", "do the task"} }

// ConfigLayout implements Adapter: Claude Code uses the base/effective split.
func (ClaudeCode) ConfigLayout() (base, effective string) {
	return claudeBaseConfig, claudeEffectiveConfig
}

// BuildBaseConfig implements Adapter: keep only whitelisted top-level keys from a
// full ~/.claude/settings.json so the packaged config carries just the agent's
// own settings.
func (ClaudeCode) BuildBaseConfig(full []byte) ([]byte, error) {
	out := map[string]any{}
	if len(bytes.TrimSpace(full)) > 0 {
		var m map[string]any
		if err := json.Unmarshal(full, &m); err != nil {
			return nil, fmt.Errorf("parsing settings: %w", err)
		}
		for k, v := range m {
			if claudeConfigWhitelist[k] {
				out[k] = v
			}
		}
	}
	// Always emit a valid JSON object ("{}\n" when nothing is whitelisted or the
	// source had no settings.json), never a 0-byte file that fails JSON parsing.
	return encodeJSON(out)
}

// BuildEffectiveConfig implements Adapter: overlay the agent's base settings over
// the user's home settings at the top level. The agent's keys win; every other
// key (env, login, UI prefs, …) comes from the user's home.
//
// Note on secrets: the user's home settings may include an "env" block, and
// Claude Code users sometimes put credential values there (e.g.
// ANTHROPIC_AUTH_TOKEN). Those values flow into the generated effective
// settings.json, which the launcher writes into the agent home. That file is
// gitignored (and written 0600) so it is never committed — the same protection
// the Codex effective config relies on — but unlike Codex (which carries only env
// var *names*) it can hold a real value at rest. Keeping the user's env is
// deliberate: it is how their ANTHROPIC_BASE_URL and other settings continue to
// apply in the isolated home. Prefer keeping credentials in your shell/keychain
// rather than settings.json env.
// prev is unused: Claude Code writes its runtime state (.credentials.json,
// .claude.json, projects/, …) to separate files, never into settings.json, so
// there is nothing to carry forward across regeneration.
func (ClaudeCode) BuildEffectiveConfig(base, home, _ []byte) ([]byte, error) {
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
		maps.Copy(merged, bm) // agent's keys win (full top-level replace)
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
//
// State/secret entries are root-anchored (leading "/") so they only prune the
// home root, never an author's legitimately-named file deeper in the tree (e.g. a
// skill directory named history/ or a fixture skills/x/settings.json). ".git" is
// intentionally unanchored so nested repos are stripped at any depth.
func (ClaudeCode) ScaffoldExcludes() []string {
	return []string{
		// Credentials & app state (OAuth session, personal MCP servers, caches)
		"/.credentials.json",
		"/.claude.json",
		// Session transcripts, todos, shell snapshots, history
		"/projects",
		"/todos",
		"/shell-snapshots",
		"/history",
		// Logs and feature-flag cache
		"/logs",
		"/statsig",
		// Defensive: any local DB/cache Claude Code may keep at the home root.
		"/*.sqlite",
		"/*.db",
		"/cache",
		// Local, personal overrides (never shared)
		"/settings.local.json",
		"/CLAUDE.local.md",
		// The effective settings.json is generated at launch (base + the user's
		// home settings); new writes settings_base.json separately.
		"/settings.json",
		// Never copy a stray repo from the base home (any depth)
		".git",
	}
}

// PersonalSubdirs implements Adapter. Claude Code has no Codex-style per-user
// "approved commands" directory to symlink across agents (personal overrides live
// in settings.local.json, which is excluded), so there are none.
func (ClaudeCode) PersonalSubdirs() []string { return nil }

// GitignoreLines implements Adapter. Entries are root-anchored (leading "/") so
// they ignore only the home root, matching ScaffoldExcludes and leaving any
// like-named author content deeper in the tree committable.
func (ClaudeCode) GitignoreLines() []string {
	return []string{
		"# encave: never commit Claude Code credentials, app state, sessions, or caches",
		"/.credentials.json",
		"/.claude.json",
		"/projects/",
		"/todos/",
		"/shell-snapshots/",
		"/history/",
		"/logs/",
		"/statsig/",
		"/*.sqlite",
		"/*.db",
		"/cache/",
		"/settings.local.json",
		"/CLAUDE.local.md",
		"# encave: generated at launch (settings_base.json merged with your ~/.claude/settings.json)",
		"/settings.json",
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
