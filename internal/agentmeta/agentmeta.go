// Package agentmeta reads and writes the small, non-secret metadata file that
// encave stores inside each agent home (.encave.toml). The metadata records
// which target CLI an agent is built for, so `run` can select the right adapter
// without external state. It is committed with the agent and contains no
// secrets.
package agentmeta

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// FileName is the metadata file stored at the root of every agent home.
const FileName = ".encave.toml"

// Meta is the on-disk schema for .encave.toml.
type Meta struct {
	// Target names the adapter the agent is built for (e.g. "codex").
	Target string `toml:"target"`
	// SchemaVersion allows future migrations of this metadata format.
	SchemaVersion int `toml:"schema_version"`
}

// Path returns the metadata file path for an agent home directory.
func Path(agentDir string) string { return filepath.Join(agentDir, FileName) }

// Load reads .encave.toml from an agent home. It returns (nil, nil) when the
// file is absent so callers can fall back to a default target.
func Load(agentDir string) (*Meta, error) {
	data, err := os.ReadFile(Path(agentDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", FileName, err)
	}
	var m Meta
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", FileName, err)
	}
	return &m, nil
}

// DefaultTarget is the target assumed when an agent declares none.
const DefaultTarget = "codex"

// DefaultTargetOr returns the agent's declared target, or DefaultTarget when the
// metadata is missing or unreadable. Convenient for display code.
func DefaultTargetOr(agentDir string) string {
	if m, err := Load(agentDir); err == nil && m != nil && m.Target != "" {
		return m.Target
	}
	return DefaultTarget
}

// Save writes .encave.toml into an agent home.
func Save(agentDir string, m Meta) error {
	if m.SchemaVersion == 0 {
		m.SchemaVersion = 1
	}
	buf := fmt.Sprintf("# encave agent metadata (non-secret, safe to commit)\nschema_version = %d\ntarget = %q\n", m.SchemaVersion, m.Target)
	if err := os.WriteFile(Path(agentDir), []byte(buf), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", FileName, err)
	}
	return nil
}
