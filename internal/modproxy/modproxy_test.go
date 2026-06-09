package modproxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyBase(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"", defaultProxy, true},
		{"https://proxy.example.com", "https://proxy.example.com", true},
		{"https://a.example,https://b.example", "https://a.example", true},
		{"off,https://b.example", "https://b.example", true},    // skip "off"
		{"direct|https://b.example", "https://b.example", true}, // pipe-separated, skip "direct"
		{"off", "", false},
		{"direct", "", false},
		{"off,direct", "", false},
	}
	for _, c := range cases {
		got, ok := proxyBase(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("proxyBase(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestEscapePath(t *testing.T) {
	if got := escapePath("github.com/sert-xx/encave"); got != "github.com/sert-xx/encave" {
		t.Errorf("lowercase path changed: %q", got)
	}
	if got := escapePath("github.com/BurntSushi/toml"); got != "github.com/!burnt!sushi/toml" {
		t.Errorf("escapePath uppercase = %q", got)
	}
}

func TestLatest(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Version":"v1.4.2","Time":"2026-01-01T00:00:00Z"}`))
	}))
	defer srv.Close()
	t.Setenv("GOPROXY", srv.URL)

	v, err := Latest(context.Background(), "github.com/sert-xx/encave")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if v != "v1.4.2" {
		t.Fatalf("Latest = %q, want v1.4.2", v)
	}
	if !strings.HasSuffix(gotPath, "/github.com/sert-xx/encave/@latest") {
		t.Errorf("requested path = %q", gotPath)
	}
}

func TestLatestProxyError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()
	t.Setenv("GOPROXY", srv.URL)

	if _, err := Latest(context.Background(), "github.com/sert-xx/encave"); err == nil {
		t.Fatal("expected error on non-200 proxy response")
	}
}

func TestLatestNoUsableProxy(t *testing.T) {
	t.Setenv("GOPROXY", "off")
	if _, err := Latest(context.Background(), "github.com/sert-xx/encave"); err == nil {
		t.Fatal("expected error when GOPROXY is off")
	}
}
