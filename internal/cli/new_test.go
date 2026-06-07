package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sert-xx/encave/internal/adapter"
	"github.com/sert-xx/encave/internal/gitutil"
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

// TestGitInitCommitReadme verifies the initial commit contains only README.md.
func TestGitInitCommitReadme(t *testing.T) {
	if !gitutil.Available() {
		t.Skip("git not available")
	}
	// Isolate from global/system git config (e.g. commit.gpgsign) and provide a
	// commit identity via env, so the test is self-contained.
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A non-README file that must NOT be in the initial commit.
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("model=\"m\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	status := gitInitCommitReadme(dir)
	if !strings.Contains(status, "initial commit") {
		t.Fatalf("unexpected status: %q", status)
	}
	if !gitutil.IsRepo(dir) {
		t.Fatal("expected a git repo")
	}

	tracked, err := gitutil.Run(dir, "ls-files")
	if err != nil {
		t.Fatal(err)
	}
	if tracked != "README.md" {
		t.Errorf("initial commit tracked files = %q, want just README.md", tracked)
	}
}
