package cli

import (
	"fmt"
	"strings"
)

// repoSource is a parsed reference to a remote agent repository.
type repoSource struct {
	Owner    string
	Repo     string
	CloneURL string // a URL git can clone
}

// parseRepoSource accepts the common ways of naming a GitHub repository and
// normalizes them to owner, repo, and a clonable URL:
//
//	github.com/owner/repo
//	https://github.com/owner/repo[.git]
//	http://github.com/owner/repo
//	git@github.com:owner/repo[.git]
//	owner/repo                         (assumed to be on github.com)
//
// Non-github hosts are accepted for cloning as long as the trailing path is
// owner/repo, which keeps the installed layout <root>/<owner>/<repo> meaningful.
func parseRepoSource(raw string) (repoSource, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return repoSource{}, fmt.Errorf("empty repository reference")
	}

	var host, path string
	switch {
	case strings.HasPrefix(s, "git@"):
		// git@host:owner/repo(.git)
		rest := strings.TrimPrefix(s, "git@")
		h, p, ok := strings.Cut(rest, ":")
		if !ok {
			return repoSource{}, fmt.Errorf("malformed scp-style URL %q", raw)
		}
		host, path = h, p
	case strings.Contains(s, "://"):
		_, rest, _ := strings.Cut(s, "://")
		h, p, ok := strings.Cut(rest, "/")
		if !ok {
			return repoSource{}, fmt.Errorf("malformed URL %q", raw)
		}
		host, path = h, p
	case strings.HasPrefix(s, "github.com/"):
		host = "github.com"
		path = strings.TrimPrefix(s, "github.com/")
	default:
		// Bare owner/repo shorthand.
		host = "github.com"
		path = s
	}

	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return repoSource{}, fmt.Errorf("could not extract <owner>/<repo> from %q", raw)
	}
	owner, repo := parts[0], parts[1]

	clone := raw
	if !strings.Contains(raw, "://") && !strings.HasPrefix(raw, "git@") {
		clone = fmt.Sprintf("https://%s/%s/%s.git", host, owner, repo)
	}

	return repoSource{Owner: owner, Repo: repo, CloneURL: clone}, nil
}
