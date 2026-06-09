// Package semver provides the tiny slice of semantic-version handling encave
// needs: parsing "vMAJOR.MINOR.PATCH" strings and comparing them. encave uses it
// both for git release tags (agent versions) and for its own binary version,
// which is itself a git tag, so the rules are deliberately identical.
//
// Only the three numeric core components are significant; any prerelease or
// build suffix (after '-' or '+') is ignored for comparison.
package semver

import "strings"

// Version is a parsed major/minor/patch triple.
type Version [3]int

// Parse parses a "vMAJOR.MINOR.PATCH" string, ignoring any prerelease/build
// suffix. It reports ok=false for anything that is not a leading-"v", three-part,
// all-numeric version (e.g. "main", "(devel)", "1.2.3" without the leading v).
func Parse(s string) (v Version, ok bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "v") {
		return Version{}, false
	}
	core := strings.TrimPrefix(s, "v")
	// Drop a prerelease ("-rc1") or build ("+meta") suffix.
	if i := strings.IndexAny(core, "-+"); i >= 0 {
		core = core[:i]
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}, false
	}
	for i, p := range parts {
		if p == "" {
			return Version{}, false
		}
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				return Version{}, false
			}
			n = n*10 + int(c-'0')
		}
		v[i] = n
	}
	return v, true
}

// Less reports whether a sorts before b by numeric component.
func (a Version) Less(b Version) bool {
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// IsNewer reports whether candidate is a valid semver strictly greater than
// current. It returns false unless both parse as semver, so a non-semver current
// version (e.g. a local "(devel)" build or a branch checkout) is never treated as
// upgradable — the caller decides what to do in that case.
func IsNewer(candidate, current string) bool {
	c, ok := Parse(candidate)
	if !ok {
		return false
	}
	cur, ok := Parse(current)
	if !ok {
		return false
	}
	return cur.Less(c)
}
