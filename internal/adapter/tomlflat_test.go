package adapter

import (
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestEncodeSectionedTables(t *testing.T) {
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

	// No indentation on any line.
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if line != strings.TrimLeft(line, " \t") {
			t.Errorf("line is indented: %q\n---\n%s", line, s)
		}
	}
	// Section names are dotted; keys inside are plain (not dotted).
	if !strings.Contains(s, "[model_providers.proxy]\n") {
		t.Errorf("expected dotted section header [model_providers.proxy]:\n%s", s)
	}
	if !strings.Contains(s, "[model_providers.proxy.env_http_headers]\n") {
		t.Errorf("expected dotted section header for env_http_headers:\n%s", s)
	}
	if !strings.Contains(s, "\nbase_url = \"https://proxy.example.com/v1\"\n") {
		t.Errorf("expected plain key base_url inside section:\n%s", s)
	}
	if !strings.Contains(s, "\nX-Api-Key = \"PROXY_API_KEY\"\n") {
		t.Errorf("expected plain key X-Api-Key inside section:\n%s", s)
	}
	// No dotted keys inside sections, and no redundant lone parent header.
	if strings.Contains(s, "proxy.base_url") {
		t.Errorf("keys inside a section must not be dotted:\n%s", s)
	}
	if strings.Contains(s, "[model_providers]\n") {
		t.Errorf("the redundant lone parent header should be omitted:\n%s", s)
	}
}

func TestEncodeSectionedQuotesKeysNeedingIt(t *testing.T) {
	// A section-name segment containing a dot must be quoted (e.g. a projects path).
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
	if !strings.Contains(s, `[projects."/home/me/proj.v2"]`) {
		t.Errorf("expected quoted path segment in section header:\n%s", s)
	}
	if !strings.Contains(s, "\ntrust_level = \"trusted\"\n") {
		t.Errorf("expected plain key inside section:\n%s", s)
	}
	var back map[string]any
	if err := toml.Unmarshal(out, &back); err != nil || !reflect.DeepEqual(m, back) {
		t.Errorf("round-trip failed: err=%v\n%s", err, s)
	}
}

func TestEncodeSectionedRoundTrip(t *testing.T) {
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
		t.Fatalf("re-parsing output failed: %v\n%s", err, out)
	}
	if !reflect.DeepEqual(m, back) {
		t.Errorf("round-trip mismatch\n in:  %#v\n out: %#v\n---\n%s", m, back, out)
	}
}
