package gitutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNextPatch(t *testing.T) {
	cases := map[string]string{
		"":           "v0.1.0",
		"v0.1.0":     "v0.1.1",
		"v1.2.3":     "v1.2.4",
		"v2.0.9":     "v2.0.10",
		"v1.2.3-rc1": "v1.2.4", // prerelease stripped
		"not-a-tag":  "v0.1.0",
		"1.2.3":      "v0.1.0", // missing leading v -> not semver here
	}
	for in, want := range cases {
		if got := nextPatch(in); got != want {
			t.Errorf("nextPatch(%q) = %q, want %q", in, got, want)
		}
	}
}

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
