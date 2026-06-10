package gitutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestLatestRemoteSemverTag(t *testing.T) {
	if !Available() {
		t.Skip("git not available")
	}
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
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AddPaths(dir, "f"); err != nil {
		t.Fatal(err)
	}
	if err := Commit(dir, "init"); err != nil {
		t.Fatal(err)
	}
	// Mix semver and non-semver tags; v0.10.0 must beat v0.9.0 numerically.
	for _, tag := range []string{"v0.1.0", "v0.9.0", "v0.10.0", "nightly"} {
		if err := Tag(dir, tag, tag); err != nil {
			t.Fatal(err)
		}
	}

	// ls-remote works against a local repository path (no network).
	got, err := LatestRemoteSemverTag(dir, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got != "v0.10.0" {
		t.Fatalf("LatestRemoteSemverTag = %q, want v0.10.0", got)
	}

	// A repo with no semver tags yields "".
	empty := t.TempDir()
	if err := Init(empty); err != nil {
		t.Fatal(err)
	}
	if got, err := LatestRemoteSemverTag(empty, 10*time.Second); err != nil || got != "" {
		t.Fatalf("empty repo: got (%q, %v), want (\"\", nil)", got, err)
	}
}
