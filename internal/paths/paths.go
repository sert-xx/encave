// Package paths centralizes resolution of encave's root directory and the
// well-known locations beneath it. Everything encave owns lives under a single
// root (default ~/.encave, overridable via ENCAVE_ROOT) so that the tool never
// has to touch the user's personal agent home (e.g. ~/.codex).
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// RootEnv is the environment variable that overrides the default root location.
const RootEnv = "ENCAVE_ROOT"

// Root returns encave's root directory, honoring ENCAVE_ROOT when set.
func Root() (string, error) {
	if r := os.Getenv(RootEnv); r != "" {
		abs, err := filepath.Abs(r)
		if err != nil {
			return "", fmt.Errorf("resolving %s=%q: %w", RootEnv, r, err)
		}
		return abs, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating home directory: %w", err)
	}
	return filepath.Join(home, ".encave"), nil
}

// AgentDir is where an agent lives: <root>/<owner>/<repo>. Both `encave new`
// (locally authored) and `encave install` (fetched) use this single location.
func AgentDir(root, owner, repo string) string { return filepath.Join(root, owner, repo) }

// ConfigFile is encave's own (non-secret) configuration file.
func ConfigFile(root string) string { return filepath.Join(root, "config.toml") }
