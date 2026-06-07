package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sert-xx/encave/internal/agentmeta"
)

func TestCheckInstalledAgent(t *testing.T) {
	// Missing .encave.toml -> rejected.
	t.Run("no metadata", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := checkInstalledAgent(dir); err == nil {
			t.Fatal("expected error when .encave.toml is absent")
		}
	})

	// Valid encave agent -> accepted, target resolved.
	t.Run("valid codex agent", func(t *testing.T) {
		dir := t.TempDir()
		if err := agentmeta.Save(dir, agentmeta.Meta{Target: "codex"}); err != nil {
			t.Fatal(err)
		}
		target, err := checkInstalledAgent(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if target != "codex" {
			t.Fatalf("target = %q, want codex", target)
		}
	})

	// .encave.toml present but naming an unknown target -> rejected.
	t.Run("unknown target", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, agentmeta.FileName),
			[]byte("schema_version = 1\ntarget = \"nope\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := checkInstalledAgent(dir); err == nil {
			t.Fatal("expected error for unknown target")
		}
	})
}
