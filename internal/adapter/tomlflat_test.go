package adapter

import (
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestEncodeOneLevelTables(t *testing.T) {
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
	// Exactly one level of table header: [model_providers] is allowed; the
	// deeper [model_providers.proxy] / .env_http_headers headers are not.
	if !strings.Contains(s, "[model_providers]\n") {
		t.Errorf("expected a [model_providers] header:\n%s", s)
	}
	if strings.Contains(s, "[model_providers.proxy]") || strings.Contains(s, "env_http_headers]") {
		t.Errorf("expected no second-level table headers:\n%s", s)
	}
	// Inside the section, deeper nesting uses dotted keys.
	if !strings.Contains(s, "proxy.base_url = ") {
		t.Errorf("expected dotted key proxy.base_url inside section:\n%s", s)
	}
	if !strings.Contains(s, `proxy.env_http_headers.X-Api-Key = "PROXY_API_KEY"`) {
		t.Errorf("expected dotted deep key inside section:\n%s", s)
	}
}

func TestEncodeOneLevelQuotesKeysNeedingIt(t *testing.T) {
	// A key segment containing a dot must be quoted (e.g. a projects path),
	// inside the single [projects] section.
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
	if !strings.Contains(s, "[projects]\n") {
		t.Errorf("expected [projects] section:\n%s", s)
	}
	if !strings.Contains(s, `"/home/me/proj.v2".trust_level = "trusted"`) {
		t.Errorf("expected quoted path segment inside section:\n%s", s)
	}
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
