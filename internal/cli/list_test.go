package cli

import "testing"

func TestParseAgentChoice(t *testing.T) {
	const n = 3

	// Valid selections (1-based input -> 0-based index).
	for in, want := range map[string]int{"1": 0, "2": 1, "3": 2, " 2 ": 1} {
		idx, cancel, err := parseAgentChoice(in, n)
		if err != nil || cancel {
			t.Errorf("%q: unexpected cancel=%v err=%v", in, cancel, err)
			continue
		}
		if idx != want {
			t.Errorf("%q: idx = %d, want %d", in, idx, want)
		}
	}

	// Cancel inputs.
	for _, in := range []string{"q", "Q", "quit", " quit "} {
		if _, cancel, err := parseAgentChoice(in, n); !cancel || err != nil {
			t.Errorf("%q: expected cancel, got cancel=%v err=%v", in, cancel, err)
		}
	}

	// Invalid inputs (out of range, non-numeric, empty).
	for _, in := range []string{"0", "4", "-1", "abc", "", "1.5"} {
		if _, cancel, err := parseAgentChoice(in, n); cancel || err == nil {
			t.Errorf("%q: expected error, got cancel=%v err=%v", in, cancel, err)
		}
	}
}
