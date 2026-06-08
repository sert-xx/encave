// Package gitutil wraps the `git` CLI for the small set of operations encave
// needs: scaffolding a repo, staging, listing staged files, committing, tagging,
// cloning, and checking out tags. We shell out to the user's git rather than
// embed a git library so behavior matches what providers see in their own
// terminal, and so `gh`-driven workflows remain compatible.
package gitutil

import (
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// Run executes git in dir with the given args, returning trimmed stdout. On
// failure it returns an error that includes stderr.
func Run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Available reports whether the git binary is on PATH.
func Available() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// IsRepo reports whether dir is inside a git work tree.
func IsRepo(dir string) bool {
	out, err := Run(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

// Init initializes a new repository in dir (no-op if already a repo).
func Init(dir string) error {
	if IsRepo(dir) {
		return nil
	}
	_, err := Run(dir, "init")
	return err
}

// AddAll stages all changes in dir.
func AddAll(dir string) error {
	_, err := Run(dir, "add", "-A")
	return err
}

// AddPaths stages only the given paths (relative to dir).
func AddPaths(dir string, paths ...string) error {
	args := append([]string{"add", "--"}, paths...)
	_, err := Run(dir, args...)
	return err
}

// StagedFiles returns the repo-relative paths of files currently staged.
func StagedFiles(dir string) ([]string, error) {
	out, err := Run(dir, "diff", "--cached", "--name-only", "-z")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	parts := strings.Split(out, "\x00")
	files := parts[:0]
	for _, p := range parts {
		if p != "" {
			files = append(files, p)
		}
	}
	return files, nil
}

// HasStagedChanges reports whether anything is staged for commit.
func HasStagedChanges(dir string) (bool, error) {
	files, err := StagedFiles(dir)
	if err != nil {
		return false, err
	}
	return len(files) > 0, nil
}

// HasCommits reports whether the repo has at least one commit (HEAD resolves).
func HasCommits(dir string) bool {
	_, err := Run(dir, "rev-parse", "--verify", "--quiet", "HEAD")
	return err == nil
}

// Commit creates a commit with the given message.
func Commit(dir, message string) error {
	_, err := Run(dir, "commit", "-m", message)
	return err
}

// Tag creates an annotated tag. It fails if the tag already exists.
func Tag(dir, tag, message string) error {
	_, err := Run(dir, "tag", "-a", tag, "-m", message)
	return err
}

// TagExists reports whether a tag is present in dir.
func TagExists(dir, tag string) bool {
	out, err := Run(dir, "tag", "--list", tag)
	return err == nil && out != ""
}

// Clone clones url into dst.
func Clone(url, dst string) error {
	_, err := Run("", "clone", url, dst)
	return err
}

// Fetch runs `git fetch` in dir with the given arguments.
func Fetch(dir string, args ...string) error {
	_, err := Run(dir, append([]string{"fetch"}, args...)...)
	return err
}

// CheckoutTag checks out the given tag in dir (detached HEAD).
func CheckoutTag(dir, tag string) error {
	_, err := Run(dir, "checkout", "--quiet", "refs/tags/"+tag)
	return err
}

// CurrentRef returns a human-friendly description of the checked-out ref.
func CurrentRef(dir string) string {
	if out, err := Run(dir, "describe", "--tags", "--exact-match"); err == nil && out != "" {
		return out
	}
	if out, err := Run(dir, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		return out
	}
	return "unknown"
}

// defaultFirstTag is suggested when a repo has no semver tags yet.
const defaultFirstTag = "v0.1.0"

// NextPatchTag suggests the next release tag for dir: the highest existing
// vMAJOR.MINOR.PATCH tag with its patch incremented, or v0.1.0 when there are no
// semver tags yet.
func NextPatchTag(dir string) string {
	latest, err := LatestSemverTag(dir)
	if err != nil {
		return defaultFirstTag
	}
	return nextPatch(latest)
}

// nextPatch returns the patch-incremented successor of a vX.Y.Z tag, or
// defaultFirstTag when latest is empty or not semver.
func nextPatch(latest string) string {
	v := parseSemver(latest)
	if v == nil {
		return defaultFirstTag
	}
	return fmt.Sprintf("v%d.%d.%d", v[0], v[1], v[2]+1)
}

// LatestSemverTag returns the highest vMAJOR.MINOR.PATCH tag in dir, or "" if
// none exist. Comparison is numeric per component; non-semver tags are ignored.
func LatestSemverTag(dir string) (string, error) {
	out, err := Run(dir, "tag", "--list")
	if err != nil {
		return "", err
	}
	var tags []string
	for _, t := range strings.Split(out, "\n") {
		t = strings.TrimSpace(t)
		if parseSemver(t) != nil {
			tags = append(tags, t)
		}
	}
	if len(tags) == 0 {
		return "", nil
	}
	sort.Slice(tags, func(i, j int) bool {
		return lessSemver(parseSemver(tags[i]), parseSemver(tags[j]))
	})
	return tags[len(tags)-1], nil
}

// RemoteExists reports whether a named remote is configured.
func RemoteExists(dir, name string) bool {
	out, err := Run(dir, "remote")
	if err != nil {
		return false
	}
	for _, r := range strings.Split(out, "\n") {
		if strings.TrimSpace(r) == name {
			return true
		}
	}
	return false
}

// AddRemote adds a remote pointing at url.
func AddRemote(dir, name, url string) error {
	_, err := Run(dir, "remote", "add", name, url)
	return err
}

// RemoteURL returns the URL configured for a named remote.
func RemoteURL(dir, name string) (string, error) {
	return Run(dir, "remote", "get-url", name)
}

// Push runs `git push` in dir with the given arguments.
func Push(dir string, args ...string) error {
	_, err := Run(dir, append([]string{"push"}, args...)...)
	return err
}

// CurrentBranch returns the name of the currently checked-out branch, or "" if
// HEAD is detached.
func CurrentBranch(dir string) string {
	out, err := Run(dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return out
}

// parseSemver parses "vX.Y.Z" into [3]int, returning nil if it doesn't match.
func parseSemver(t string) []int {
	if !strings.HasPrefix(t, "v") {
		return nil
	}
	core := strings.SplitN(strings.TrimPrefix(t, "v"), "-", 2)[0]
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return nil
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n := 0
		if p == "" {
			return nil
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return nil
			}
			n = n*10 + int(c-'0')
		}
		nums[i] = n
	}
	return nums
}

func lessSemver(a, b []int) bool {
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}
