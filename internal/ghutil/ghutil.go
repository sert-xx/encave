// Package ghutil wraps the small slice of the GitHub CLI (`gh`) that encave uses
// to create a GitHub Release for a pushed tag. Everything degrades gracefully:
// if `gh` is missing or cannot access the repo, callers simply skip releases.
package ghutil

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Available reports whether the gh binary is on PATH.
func Available() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// run executes gh in dir and returns trimmed stdout, or an error including
// stderr.
func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("gh", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("gh %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// CanAccessRepo reports whether gh can resolve and access the repository for the
// git remote in dir (i.e. it is a GitHub repo the user is authenticated for).
func CanAccessRepo(dir string) bool {
	_, err := run(dir, "repo", "view", "--json", "nameWithOwner")
	return err == nil
}

// ReleaseExists reports whether a release for tag already exists.
func ReleaseExists(dir, tag string) bool {
	_, err := run(dir, "release", "view", tag)
	return err == nil
}

// CreateRelease creates a GitHub release for tag (which must already be pushed),
// titled with the tag and with auto-generated notes.
func CreateRelease(dir, tag string) error {
	_, err := run(dir, "release", "create", tag, "--title", tag, "--generate-notes")
	return err
}
