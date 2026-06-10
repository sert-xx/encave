package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sert-xx/encave/internal/agentmeta"
)

func TestUnmanagedAuthTarget(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ENCAVE_ROOT", root)

	// Install a claude-code agent (unmanaged auth) and a codex agent (managed).
	mkAgent := func(owner, repo, target string) {
		dir := filepath.Join(root, owner, repo)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := agentmeta.Save(dir, agentmeta.Meta{Target: target}); err != nil {
			t.Fatal(err)
		}
	}
	mkAgent("dai", "claude-agent", "claude-code")
	mkAgent("dai", "codex-agent", "codex")

	cases := []struct {
		name      string
		agentFlag string
		wantWarn  bool
	}{
		{"global scope (empty)", "", false},
		{"claude agent → warn", "dai/claude-agent", true},
		{"codex agent → no warn (managed)", "dai/codex-agent", false},
		{"not installed → no warn", "dai/missing", false},
		{"invalid ref → no warn", "not-a-ref", false},
	}
	for _, c := range cases {
		_, warn := unmanagedAuthTarget(c.agentFlag)
		if warn != c.wantWarn {
			t.Errorf("%s: warn = %v, want %v", c.name, warn, c.wantWarn)
		}
	}
}
