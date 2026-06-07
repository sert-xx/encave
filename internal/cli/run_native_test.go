package cli

import (
	"strings"
	"testing"
)

// TestRunDefaultNativeDryRun verifies that `encave run default` resolves to the
// native passthrough launch: no home override and no credential injection.
func TestRunDefaultNativeDryRun(t *testing.T) {
	t.Setenv("ENCAVE_ROOT", t.TempDir())
	t.Setenv("CODEX_HOME", "/home/tester/.codex")

	var code int
	out := captureStdout(t, func() {
		code = cmdRun([]string{"default", "--dry-run", "--", "exec", "hi"})
	})
	if code != 0 {
		t.Fatalf("cmdRun exit = %d\n%s", code, out)
	}

	for _, want := range []string{
		"your default codex home",
		"/home/tester/.codex",
		"not overridden",
		"none injected",
		"codex exec hi",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("native dry-run output missing %q\n---\n%s", want, out)
		}
	}
}
