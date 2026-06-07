package cli

import "testing"

func TestVersionFallsBackToNonEmpty(t *testing.T) {
	old := Version
	defer func() { Version = old }()

	// With no explicit override, version() must still return something usable
	// (module build-info version, or "(devel)" for local/test builds).
	Version = ""
	if got := version(); got == "" {
		t.Fatal("version() returned empty string")
	}

	// An explicit build-time override takes precedence.
	Version = "v9.9.9"
	if got := version(); got != "v9.9.9" {
		t.Fatalf("version() = %q, want v9.9.9", got)
	}
}
