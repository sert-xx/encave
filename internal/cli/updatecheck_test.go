package cli

import (
	"os"
	"testing"
	"time"
)

func TestUpdateCheckDisabled(t *testing.T) {
	cases := map[string]bool{
		"":      false,
		"0":     false,
		"false": false,
		"FALSE": false,
		"1":     true,
		"true":  true,
		"yes":   true,
	}
	for v, want := range cases {
		t.Setenv(noUpdateCheckEnv, v)
		if got := updateCheckDisabled(); got != want {
			t.Errorf("updateCheckDisabled() with %s=%q = %v, want %v", noUpdateCheckEnv, v, got, want)
		}
	}
	// Unset behaves like empty (not disabled).
	os.Unsetenv(noUpdateCheckEnv)
	if updateCheckDisabled() {
		t.Error("updateCheckDisabled() = true with env unset, want false")
	}
}

func TestCheckRecordDue(t *testing.T) {
	now := time.Now()
	const interval = time.Hour
	cases := []struct {
		name      string
		checkedAt int64
		want      bool
	}{
		{"never checked", 0, true},
		{"just now", now.Unix(), false},
		{"within interval", now.Add(-interval / 2).Unix(), false},
		{"exactly interval", now.Add(-interval).Unix(), true},
		{"past interval", now.Add(-2 * interval).Unix(), true},
	}
	for _, c := range cases {
		r := checkRecord{CheckedAt: c.checkedAt}
		if got := r.due(now, interval); got != c.want {
			t.Errorf("%s: due() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestShouldOfferUpdate(t *testing.T) {
	cases := []struct {
		name                       string
		latest, current, dismissed string
		want                       bool
	}{
		{"newer tag", "v1.1.0", "v1.0.0", "", true},
		{"same version", "v1.0.0", "v1.0.0", "", false},
		{"older latest", "v1.0.0", "v1.1.0", "", false},
		{"no latest", "", "v1.0.0", "", false},
		{"dismissed exact version", "v1.1.0", "v1.0.0", "v1.1.0", false},
		{"dismissed older, newer available", "v1.2.0", "v1.0.0", "v1.1.0", true},
		{"installed on a branch", "v1.0.0", "main", "", true},
		{"installed on a branch, dismissed", "v1.0.0", "main", "v1.0.0", false},
		{"numeric not lexical", "v0.10.0", "v0.9.0", "", true},
	}
	for _, c := range cases {
		if got := shouldOfferUpdate(c.latest, c.current, c.dismissed); got != c.want {
			t.Errorf("%s: shouldOfferUpdate(%q,%q,%q) = %v, want %v",
				c.name, c.latest, c.current, c.dismissed, got, c.want)
		}
	}
}

func TestUpdateCacheRoundTrip(t *testing.T) {
	root := t.TempDir()

	// Loading a fresh root yields a usable, empty cache.
	c := loadUpdateCache(root)
	if c.Agents == nil {
		t.Fatal("Agents map should be initialized")
	}

	c.Encave = checkRecord{CheckedAt: 12345, Latest: "v9.9.9", Dismissed: "v9.9.9"}
	c.Agents["dai/review-agent"] = checkRecord{CheckedAt: 678, Latest: "v1.0.0"}
	saveUpdateCache(root, c)

	got := loadUpdateCache(root)
	if got.Encave != c.Encave {
		t.Errorf("Encave round-trip = %+v, want %+v", got.Encave, c.Encave)
	}
	if got.Agents["dai/review-agent"] != c.Agents["dai/review-agent"] {
		t.Errorf("agent round-trip = %+v", got.Agents["dai/review-agent"])
	}
}

func TestLoadUpdateCacheCorrupt(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(updateCachePath(root), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Corrupt cache must degrade to an empty, usable value rather than panic.
	c := loadUpdateCache(root)
	if c.Agents == nil {
		t.Error("Agents map should be initialized even for corrupt cache")
	}
}
