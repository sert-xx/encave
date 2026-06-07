package cli

import (
	"strings"
	"testing"
)

func TestRenderAgentReadmeWithAuthVars(t *testing.T) {
	out := renderAgentReadme("review-agent", "codex", []string{"PROXY_TOKEN"})

	mustContain := []string{
		"# review-agent",
		"encave install github.com/<owner>/review-agent",
		"encave auth set --global",
		"`PROXY_TOKEN`",
		"encave <owner>/review-agent",
		"encave run", // interactive picker mention
		"encave publish review-agent",
		"fail-closed secret scan",
	}
	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("README missing %q\n---\n%s", s, out)
		}
	}
}

func TestRenderAgentReadmeNoAuthVars(t *testing.T) {
	out := renderAgentReadme("plain-agent", "codex", nil)
	if !strings.Contains(out, "does not declare an environment-based credential") {
		t.Errorf("expected no-credential note, got:\n%s", out)
	}
	if strings.Contains(out, "reads its credential from the following") {
		t.Errorf("should not list credential vars when none exist:\n%s", out)
	}
}
