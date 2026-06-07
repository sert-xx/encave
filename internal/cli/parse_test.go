package cli

import "testing"

func TestParseAgentRef(t *testing.T) {
	ok := map[string]AgentRef{
		"dai/review-agent":  {Owner: "dai", Repo: "review-agent"},
		"dai/review-agent/": {Owner: "dai", Repo: "review-agent"},
	}
	for in, want := range ok {
		got, err := parseAgentRef(in)
		if err != nil {
			t.Errorf("%q: unexpected error %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("%q: got %+v want %+v", in, got, want)
		}
	}

	bad := []string{"", "single", "a/b/c", "../etc", "owner/", "/repo", `o\r/r`}
	for _, in := range bad {
		if _, err := parseAgentRef(in); err == nil {
			t.Errorf("%q: expected error, got none", in)
		}
	}
}

func TestParseRepoSource(t *testing.T) {
	cases := map[string]struct{ owner, repo string }{
		"github.com/dai/review-agent":             {"dai", "review-agent"},
		"https://github.com/dai/review-agent":     {"dai", "review-agent"},
		"https://github.com/dai/review-agent.git": {"dai", "review-agent"},
		"git@github.com:dai/review-agent.git":     {"dai", "review-agent"},
		"dai/review-agent":                        {"dai", "review-agent"},
	}
	for in, want := range cases {
		got, err := parseRepoSource(in)
		if err != nil {
			t.Errorf("%q: unexpected error %v", in, err)
			continue
		}
		if got.Owner != want.owner || got.Repo != want.repo {
			t.Errorf("%q: got %s/%s want %s/%s", in, got.Owner, got.Repo, want.owner, want.repo)
		}
		if got.CloneURL == "" {
			t.Errorf("%q: empty clone URL", in)
		}
	}

	for _, in := range []string{"", "not-a-repo", "https://github.com/onlyowner"} {
		if _, err := parseRepoSource(in); err == nil {
			t.Errorf("%q: expected error", in)
		}
	}
}
