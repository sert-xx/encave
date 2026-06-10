package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyTreeExcludes(t *testing.T) {
	src := t.TempDir()
	// Build a small tree:
	//   config.toml
	//   auth.json          (excluded by name)
	//   agents/review.toml
	//   logs/run.log       (logs dir excluded)
	//   sessions/x.jsonl   (sessions dir excluded)
	mk := func(rel, content string) {
		p := filepath.Join(src, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("config.toml", "model = \"x\"")
	mk("auth.json", "{secret}")
	mk("agents/review.toml", "role = \"reviewer\"")
	mk("logs/run.log", "noise")
	mk("sessions/x.jsonl", "{}")

	dst := filepath.Join(t.TempDir(), "out")
	excludes := []string{"auth.json", "logs", "sessions"}
	res, err := CopyTree(src, dst, excludes)
	if err != nil {
		t.Fatal(err)
	}

	mustExist := []string{"config.toml", "agents/review.toml"}
	for _, r := range mustExist {
		if _, err := os.Stat(filepath.Join(dst, r)); err != nil {
			t.Errorf("expected %s to be copied: %v", r, err)
		}
	}
	mustNotExist := []string{"auth.json", "logs/run.log", "logs", "sessions/x.jsonl", "sessions"}
	for _, r := range mustNotExist {
		if _, err := os.Stat(filepath.Join(dst, r)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be excluded, but it exists", r)
		}
	}
	if res.FilesCopied != 2 {
		t.Errorf("FilesCopied = %d, want 2", res.FilesCopied)
	}
}

func TestCopyTreeRootAnchoredExcludes(t *testing.T) {
	src := t.TempDir()
	mk := func(rel, content string) {
		p := filepath.Join(src, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A root-level state dir to drop, and a like-named dir nested inside authored
	// content that must be KEPT.
	mk("projects/session.jsonl", "{}")           // root state — excluded
	mk("skills/notes/projects/data.md", "keep")  // authored content — kept
	mk("settings.json", "{}")                    // root file — excluded
	mk("skills/x/settings.json", "fixture-keep") // authored fixture — kept

	dst := filepath.Join(t.TempDir(), "out")
	excludes := []string{"/projects", "/settings.json"}
	if _, err := CopyTree(src, dst, excludes); err != nil {
		t.Fatal(err)
	}

	for _, keep := range []string{"skills/notes/projects/data.md", "skills/x/settings.json"} {
		if _, err := os.Stat(filepath.Join(dst, keep)); err != nil {
			t.Errorf("root-anchored exclude wrongly pruned nested %s: %v", keep, err)
		}
	}
	for _, drop := range []string{"projects/session.jsonl", "projects", "settings.json"} {
		if _, err := os.Stat(filepath.Join(dst, drop)); !os.IsNotExist(err) {
			t.Errorf("expected root %s to be excluded, but it exists", drop)
		}
	}
}

func TestCopyTreeRefusesExistingDest(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir() // already exists
	if _, err := CopyTree(src, dst, nil); err == nil {
		t.Fatal("expected error copying into existing destination")
	}
}
