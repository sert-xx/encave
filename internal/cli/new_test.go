package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sert-xx/encave/internal/adapter"
)

// TestMaybeWriteReadmeOverwrites verifies `new` replaces a README copied from
// the base home with the freshly generated template.
func TestMaybeWriteReadmeOverwrites(t *testing.T) {
	dst := t.TempDir()
	// Simulate a generic README copied from the user's ~/.codex.
	generic := "# my generic codex home\nnothing to do with any agent\n"
	if err := os.WriteFile(filepath.Join(dst, "README.md"), []byte(generic), 0o644); err != nil {
		t.Fatal(err)
	}
	// A config so the renderer can discover auth vars (none here).
	if err := os.WriteFile(filepath.Join(dst, "config.toml"), []byte("model=\"m\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ad, err := adapter.Get("codex")
	if err != nil {
		t.Fatal(err)
	}

	status := maybeWriteReadme(dst, AgentRef{Owner: "dai", Repo: "review-agent"}, ad)
	if !strings.Contains(status, "replaced") {
		t.Errorf("status should note replacement, got %q", status)
	}

	got, err := os.ReadFile(filepath.Join(dst, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(got)
	if strings.Contains(out, "generic codex home") {
		t.Errorf("the copied README was not overwritten:\n%s", out)
	}
	if !strings.Contains(out, "# review-agent") || !strings.Contains(out, "encave install github.com/dai/review-agent") {
		t.Errorf("generated template not written:\n%s", out)
	}
}
