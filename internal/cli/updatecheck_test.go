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

func TestCheckRecordFresh(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name      string
		checkedAt int64
		want      bool
	}{
		{"never checked", 0, false},
		{"just now", now.Unix(), true},
		{"within ttl", now.Add(-updateCheckTTL / 2).Unix(), true},
		{"past ttl", now.Add(-updateCheckTTL - time.Hour).Unix(), false},
	}
	for _, c := range cases {
		r := checkRecord{CheckedAt: c.checkedAt}
		if got := r.fresh(now); got != c.want {
			t.Errorf("%s: fresh() = %v, want %v", c.name, got, c.want)
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

	c.Encave = checkRecord{CheckedAt: 12345, Latest: "v9.9.9"}
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
