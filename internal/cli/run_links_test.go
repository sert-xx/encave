package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sert-xx/encave/internal/adapter"
)

func TestPersonalLinkPlan(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", base) // adapter.BaseHome() reads this

	ad, err := adapter.Get("codex")
	if err != nil {
		t.Fatal(err)
	}

	// Fresh agent home with no rules/ -> the plan links it from the base home.
	agentDir := t.TempDir()
	plan := personalLinkPlan(ad, agentDir)
	if len(plan) != 1 {
		t.Fatalf("expected 1 planned link, got %d (%+v)", len(plan), plan)
	}
	if plan[0].dst != filepath.Join(agentDir, "rules") || plan[0].src != filepath.Join(base, "rules") {
		t.Fatalf("unexpected link: %+v", plan[0])
	}

	// Apply it and confirm the symlink resolves to the base rules dir.
	applyPersonalLinks(plan)
	got, err := os.Readlink(filepath.Join(agentDir, "rules"))
	if err != nil {
		t.Fatalf("expected a symlink: %v", err)
	}
	if got != filepath.Join(base, "rules") {
		t.Fatalf("symlink target = %q, want %q", got, filepath.Join(base, "rules"))
	}
}

func TestPersonalLinkPlanRespectsRealDir(t *testing.T) {
	base := t.TempDir()
	_ = os.MkdirAll(filepath.Join(base, "rules"), 0o755)
	t.Setenv("CODEX_HOME", base)

	ad, _ := adapter.Get("codex")

	// Agent already has a real rules/ directory: it must not be replaced.
	agentDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(agentDir, "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if plan := personalLinkPlan(ad, agentDir); len(plan) != 0 {
		t.Fatalf("expected no link when a real dir is present, got %+v", plan)
	}
}

func TestPersonalLinkPlanNoBaseDir(t *testing.T) {
	base := t.TempDir() // no rules/ subdir
	t.Setenv("CODEX_HOME", base)
	ad, _ := adapter.Get("codex")
	if plan := personalLinkPlan(ad, t.TempDir()); len(plan) != 0 {
		t.Fatalf("expected no link when base has no rules dir, got %+v", plan)
	}
}
