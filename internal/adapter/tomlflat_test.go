package adapter

import (
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestEncodeDottedNoNesting(t *testing.T) {
	in := []byte(`
model = "internal-model"

[model_providers.proxy]
base_url = "https://proxy.example.com/v1"
env_key = "PROXY_TOKEN"

[model_providers.proxy.env_http_headers]
"X-Api-Key" = "PROXY_API_KEY"
`)
	var m map[string]any
	if err := toml.Unmarshal(in, &m); err != nil {
		t.Fatal(err)
	}
	out, err := encodeTOML(m)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)

	// Flat: no leading whitespace (no indentation) on any line.
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if line != strings.TrimLeft(line, " \t") {
			t.Errorf("line is indented: %q\n---\n%s", line, s)
		}
	}
	// Dotted keys, not nested table headers.
	if !strings.Contains(s, "model_providers.proxy.base_url = ") {
		t.Errorf("expected dotted key for base_url:\n%s", s)
	}
	if strings.Contains(s, "[model_providers]") || strings.Contains(s, "[model_providers.proxy]") {
		t.Errorf("expected no nested table headers:\n%s", s)
	}
	// Hyphens are valid in TOML bare keys, so X-Api-Key stays unquoted.
	if !strings.Contains(s, `model_providers.proxy.env_http_headers.X-Api-Key = "PROXY_API_KEY"`) {
		t.Errorf("expected flat dotted header key:\n%s", s)
	}
}

func TestEncodeDottedQuotesKeysNeedingIt(t *testing.T) {
	// A key segment containing a dot must be quoted (e.g. a projects path).
	m := map[string]any{
		"projects": map[string]any{
			"/home/me/proj.v2": map[string]any{"trust_level": "trusted"},
		},
	}
	out, err := encodeTOML(m)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `projects."/home/me/proj.v2".trust_level = "trusted"`) {
		t.Errorf("expected quoted path segment:\n%s", s)
	}
	// And it must round-trip.
	var back map[string]any
	if err := toml.Unmarshal(out, &back); err != nil || !reflect.DeepEqual(m, back) {
		t.Errorf("round-trip failed: err=%v\n%s", err, s)
	}
}

func TestEncodeDottedRoundTrip(t *testing.T) {
	in := []byte(`
model = "m"
project_root_markers = [".git", "go.mod"]
model_context_window = 256000

[profiles.fast]
model = "fast-model"

[[agents.roles]]
name = "reviewer"
[[agents.roles]]
name = "tester"

[shell_environment_policy]
inherit = "core"
`)
	var m map[string]any
	if err := toml.Unmarshal(in, &m); err != nil {
		t.Fatal(err)
	}
	out, err := encodeTOML(m)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := toml.Unmarshal(out, &back); err != nil {
		t.Fatalf("re-parsing dotted output failed: %v\n%s", err, out)
	}
	if !reflect.DeepEqual(m, back) {
		t.Errorf("round-trip mismatch\n in:  %#v\n out: %#v\n---\n%s", m, back, out)
	}
}
