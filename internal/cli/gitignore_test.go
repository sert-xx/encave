package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sert-xx/encave/internal/adapter"
)

func TestEnsureGitignoreMergesAndDedupes(t *testing.T) {
	dir := t.TempDir()
	// A .gitignore "copied from the user's home", with a pre-existing duplicate
	// and an entry that overlaps with what the adapter will add.
	home := "# my home ignores\n*.bak\n*.bak\nauth.json\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(home), 0o644); err != nil {
		t.Fatal(err)
	}

	ad, _ := adapter.Get("codex")
	if err := ensureGitignore(dir, ad); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	// Existing content preserved.
	if !strings.Contains(out, "# my home ignores") || !strings.Contains(out, "*.bak") {
		t.Errorf("existing entries not preserved:\n%s", out)
	}
	// Adapter entries appended.
	if !strings.Contains(out, "config.toml") {
		t.Errorf("adapter entries not appended:\n%s", out)
	}
	// No duplicate non-blank lines.
	counts := map[string]int{}
	for _, l := range strings.Split(out, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			counts[t]++
		}
	}
	for line, n := range counts {
		if n > 1 {
			t.Errorf("duplicate line %q appears %d times:\n%s", line, n, out)
		}
	}

	// Idempotent: a second run yields identical content.
	if err := ensureGitignore(dir, ad); err != nil {
		t.Fatal(err)
	}
	data2, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if string(data2) != out {
		t.Errorf("ensureGitignore not idempotent:\n--- first ---\n%s\n--- second ---\n%s", out, data2)
	}
}
