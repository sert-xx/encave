package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sert-xx/encave/internal/gitutil"
	"github.com/sert-xx/encave/internal/paths"
	"github.com/sert-xx/encave/internal/semver"
)

// modulePath is encave's Go module path, used to build the `go install` line for
// a self-update; encaveRepoURL is the git remote the self-check reads tags from.
const (
	modulePath    = "github.com/sert-xx/encave"
	encaveRepoURL = "https://github.com/sert-xx/encave"
)

// Update checks read the network at most this often per target; in between, the
// cached "latest known" version still drives the prompt on every run, so an
// available update keeps being offered until you act on it. Detection is from git
// release tags (not the lagging module-proxy @latest), so a freshly pushed tag is
// seen at the next check.
const (
	selfCheckInterval  = time.Hour
	agentCheckInterval = time.Hour
)

// noUpdateCheckEnv lets users (and CI) disable all update checking.
const noUpdateCheckEnv = "ENCAVE_NO_UPDATE_CHECK"

// updateCheckDisabled reports whether update checks are turned off via the
// environment. Any value other than "", "0", or "false" disables them.
func updateCheckDisabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(noUpdateCheckEnv)))
	return v != "" && v != "0" && v != "false"
}

// checkRecord is the cached state of one target's update check.
type checkRecord struct {
	CheckedAt int64  `json:"checked_at"` // unix seconds of the last network check
	Latest    string `json:"latest"`     // latest version seen at that time
	Dismissed string `json:"dismissed"`  // version the user last declined (per-version, not time-based)
}

// due reports whether it is time to hit the network again for this target.
func (r checkRecord) due(now time.Time, interval time.Duration) bool {
	return r.CheckedAt == 0 || now.Sub(time.Unix(r.CheckedAt, 0)) >= interval
}

// shouldOfferUpdate decides whether to offer latest over the currently installed
// version, given the version the user last declined. It returns false when there
// is no latest, when latest is the same as current, or when the user already
// dismissed exactly this version (a newer one would still be offered). When
// current is a release tag, only a strictly newer tag is offered; when current is
// not a tag (e.g. installed on a branch), any release tag counts as an upgrade.
func shouldOfferUpdate(latest, current, dismissed string) bool {
	if latest == "" || latest == dismissed || latest == current {
		return false
	}
	if _, onTag := semver.Parse(current); onTag {
		return semver.IsNewer(latest, current)
	}
	return true
}

// updateCache persists update-check state under the encave root so network checks
// are throttled while prompts can still be re-offered every run.
type updateCache struct {
	Encave checkRecord            `json:"encave"`
	Agents map[string]checkRecord `json:"agents"`
}

// updateCachePath is the cache file location under the encave root.
func updateCachePath(root string) string {
	return filepath.Join(root, ".update-check.json")
}

// loadUpdateCache reads the cache, returning an empty (usable) value on any
// error — the cache is an optimization, never a source of truth.
func loadUpdateCache(root string) updateCache {
	c := updateCache{Agents: map[string]checkRecord{}}
	data, err := os.ReadFile(updateCachePath(root))
	if err != nil {
		return c
	}
	_ = json.Unmarshal(data, &c)
	if c.Agents == nil {
		c.Agents = map[string]checkRecord{}
	}
	return c
}

// saveUpdateCache writes the cache best-effort; failures are ignored.
func saveUpdateCache(root string, c updateCache) {
	data, err := json.Marshal(c)
	if err != nil {
		return
	}
	_ = os.MkdirAll(root, 0o755)
	_ = os.WriteFile(updateCachePath(root), data, 0o644)
}

// latestEncaveRelease returns the newest encave release tag from the repository,
// or "" on any error. It reads git tags directly so a just-pushed tag is visible
// immediately (the module proxy's @latest can lag well behind).
func latestEncaveRelease() string {
	if !gitutil.Available() {
		return ""
	}
	tag, err := gitutil.LatestRemoteSemverTag(encaveRepoURL, 4*time.Second)
	if err != nil {
		return ""
	}
	return tag
}

// maybeOfferSelfUpdate offers to install a newer encave release with
// `go install module@vX.Y.Z` (the exact tag, never @latest). The network is
// consulted at most once per selfCheckInterval, but the cached latest version is
// compared against the running version on every interactive run, so an available
// update keeps being offered until installed or declined for that version. It is
// silent on every non-actionable path: disabled checks, non-terminals, dev
// builds, network errors, already-latest, and a version the user dismissed.
func maybeOfferSelfUpdate() {
	if updateCheckDisabled() || !isInteractive() {
		return
	}
	current := version()
	if _, ok := semver.Parse(current); !ok {
		return // dev build or unknown version: nothing to compare against
	}
	root, err := paths.Root()
	if err != nil {
		return
	}

	cache := loadUpdateCache(root)
	now := time.Now()
	rec := cache.Encave
	if rec.due(now, selfCheckInterval) {
		if latest := latestEncaveRelease(); latest != "" {
			rec.Latest = latest
		}
		rec.CheckedAt = now.Unix()
		cache.Encave = rec
		saveUpdateCache(root, cache)
	}

	if !shouldOfferUpdate(rec.Latest, current, rec.Dismissed) {
		return
	}

	fmt.Printf("A newer encave is available: %s (you have %s).\n", rec.Latest, current)
	if _, err := exec.LookPath("go"); err != nil {
		fmt.Printf("  Update with:  go install %s@%s\n", modulePath, rec.Latest)
		return
	}
	if !confirm(fmt.Sprintf("Install encave %s now?", rec.Latest)) {
		rec.Dismissed = rec.Latest // don't nag for this version again; a newer one will re-prompt
		cache.Encave = rec
		saveUpdateCache(root, cache)
		return
	}
	if err := runGoInstall(rec.Latest); err != nil {
		errf("self-update failed: %v", err)
		fmt.Fprintf(os.Stderr, "  install it manually:  go install %s@%s\n", modulePath, rec.Latest)
		return
	}
	fmt.Printf("Installed encave %s. Re-run your command to use the new version.\n", rec.Latest)
}

// runGoInstall installs a specific encave version via `go install`. It pins the
// exact version (never @latest) so the user gets precisely what was offered, and
// streams go's own output so toolchain/download progress is visible.
func runGoInstall(version string) error {
	cmd := exec.Command("go", "install", modulePath+"@"+version)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// maybeOfferAgentUpdate checks an installed agent's origin for a newer release
// tag and offers to update before launching, reusing updateOne so an accepted
// update behaves exactly like `encave update <ref>`. Like the self-check, the
// network fetch is throttled per agentCheckInterval while the offer is re-made on
// every run (from the local tags) until the agent is updated or that version is
// declined. Best-effort and silent unless it has an actionable, interactive
// offer.
func maybeOfferAgentUpdate(root string, ref AgentRef, dir string) {
	if updateCheckDisabled() || !isInteractive() {
		return
	}
	if !gitutil.Available() || !gitutil.IsRepo(dir) || !gitutil.RemoteExists(dir, "origin") {
		return
	}

	cache := loadUpdateCache(root)
	now := time.Now()
	key := ref.String()
	rec := cache.Agents[key]
	if rec.due(now, agentCheckInterval) {
		// Refresh tags so "latest" reflects origin. Bounded so a slow remote can't
		// hang the launch; the attempt is recorded regardless to throttle retries.
		_ = gitutil.FetchTimeout(dir, 8*time.Second, "origin", "--tags", "--prune")
		rec.CheckedAt = now.Unix()
		if l, lerr := gitutil.LatestSemverTag(dir); lerr == nil {
			rec.Latest = l
		}
		cache.Agents[key] = rec
		saveUpdateCache(root, cache)
	}

	latest, lerr := gitutil.LatestSemverTag(dir)
	if lerr != nil || latest == "" {
		return
	}
	current := gitutil.CurrentRef(dir)
	if !shouldOfferUpdate(latest, current, rec.Dismissed) {
		return
	}

	fmt.Printf("A newer version of %s is available: %s (installed: %s).\n", ref, latest, current)
	if !confirm("Update before launching?") {
		rec.Dismissed = latest
		cache.Agents[key] = rec
		saveUpdateCache(root, cache)
		return
	}
	if err := updateOne(root, ref, ""); err != nil {
		errf("update failed; launching the currently installed version: %v", err)
	}
}
