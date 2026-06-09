package semver

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want Version
		ok   bool
	}{
		{"v0.1.0", Version{0, 1, 0}, true},
		{"v1.2.3", Version{1, 2, 3}, true},
		{"v10.20.30", Version{10, 20, 30}, true},
		{"v1.2.3-rc1", Version{1, 2, 3}, true},  // prerelease stripped
		{"v1.2.3+meta", Version{1, 2, 3}, true}, // build metadata stripped
		{"  v1.2.3  ", Version{1, 2, 3}, true},  // surrounding space tolerated
		{"1.2.3", Version{}, false},             // missing leading v
		{"v1.2", Version{}, false},              // too few parts
		{"v1.2.3.4", Version{}, false},          // too many parts
		{"v1.x.3", Version{}, false},            // non-numeric
		{"vabc", Version{}, false},
		{"(devel)", Version{}, false},
		{"main", Version{}, false},
		{"", Version{}, false},
		{"v1..3", Version{}, false}, // empty component
	}
	for _, c := range cases {
		got, ok := Parse(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("Parse(%q) = (%v, %v), want (%v, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestLess(t *testing.T) {
	cases := []struct {
		a, b Version
		want bool
	}{
		{Version{0, 1, 0}, Version{0, 1, 1}, true},
		{Version{0, 1, 0}, Version{0, 2, 0}, true},
		{Version{0, 9, 0}, Version{1, 0, 0}, true},
		{Version{1, 2, 3}, Version{1, 2, 3}, false},
		{Version{1, 2, 4}, Version{1, 2, 3}, false},
		{Version{2, 0, 0}, Version{1, 9, 9}, false},
		{Version{1, 10, 0}, Version{1, 9, 0}, false}, // numeric, not lexical
	}
	for _, c := range cases {
		if got := c.a.Less(c.b); got != c.want {
			t.Errorf("%v.Less(%v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		candidate, current string
		want               bool
	}{
		{"v0.2.0", "v0.1.0", true},
		{"v1.0.0", "v0.9.9", true},
		{"v0.1.10", "v0.1.9", true},  // numeric compare, not lexical
		{"v0.1.0", "v0.1.0", false},  // equal is not newer
		{"v0.1.0", "v0.2.0", false},  // older
		{"v0.2.0", "(devel)", false}, // non-semver current: never upgradable
		{"v0.2.0", "main", false},    // branch checkout: never upgradable
		{"not-a-tag", "v0.1.0", false},
	}
	for _, c := range cases {
		if got := IsNewer(c.candidate, c.current); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.candidate, c.current, got, c.want)
		}
	}
}
