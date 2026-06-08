package gitutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasCommits(t *testing.T) {
	if !Available() {
		t.Skip("git not available")
	}
	// Isolate from global/system git config and provide a commit identity.
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	t.Setenv("GIT_AUTHOR_NAME", "T")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@e.com")
	t.Setenv("GIT_COMMITTER_NAME", "T")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@e.com")

	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if HasCommits(dir) {
		t.Error("a freshly initialized repo should have no commits")
	}

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AddPaths(dir, "README.md"); err != nil {
		t.Fatal(err)
	}
	if err := Commit(dir, "init"); err != nil {
		t.Fatal(err)
	}
	if !HasCommits(dir) {
		t.Error("repo with a commit should report HasCommits=true")
	}
}
