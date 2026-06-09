package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sert-xx/encave/internal/gitutil"
	"github.com/sert-xx/encave/internal/modproxy"
	"github.com/sert-xx/encave/internal/paths"
	"github.com/sert-xx/encave/internal/semver"
)

// modulePath is encave's Go module path, used both to query the proxy for the
// latest version and to build the `go install` line for a self-update.
const modulePath = "github.com/sert-xx/encave"

// updateCheckTTL throttles network update checks: at most one per target per
// window. It also doubles as anti-nag — having checked recently, encave does not
// prompt again until the window elapses, so declining an offer is respected for a
// day rather than re-asked on the next command.
const updateCheckTTL = 24 * time.Hour

// noUpdateCheckEnv lets users (and CI) disable all update checking.
const noUpdateCheckEnv = "ENCAVE_NO_UPDATE_CHECK"

// updateCheckDisabled reports whether update checks are turned off via the
// environment. Any value other than "", "0", or "false" disables them.
func updateCheckDisabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(noUpdateCheckEnv)))
	return v != "" && v != "0" && v != "false"
}

// checkRecord is the cached result of one update check.
type checkRecord struct {
	CheckedAt int64  `json:"checked_at"` // unix seconds of the last network check
	Latest    string `json:"latest"`     // latest version seen at that time
}

// fresh reports whether the record was checked within the TTL of now.
func (r checkRecord) fresh(now time.Time) bool {
	return r.CheckedAt > 0 && now.Sub(time.Unix(r.CheckedAt, 0)) < updateCheckTTL
}

// updateCache persists update-check state under the encave root so checks (and
// their prompts) are throttled across invocations.
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

// maybeOfferSelfUpdate checks whether a newer encave release exists and, if so,
// offers to install that specific version with `go install module@vX.Y.Z`. It is
// best-effort and silent on every path that isn't an actionable, interactive
// upgrade: disabled checks, non-terminals, dev builds, recent checks, network
// errors, and "already latest" all return without output.
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
	if cache.Encave.fresh(now) {
		return // checked recently; don't nag
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	latest, ferr := modproxy.Latest(ctx, modulePath)

	cache.Encave.CheckedAt = now.Unix()
	if ferr == nil && latest != "" {
		cache.Encave.Latest = latest
	}
	saveUpdateCache(root, cache)
	if ferr != nil || !semver.IsNewer(latest, current) {
		return
	}

	fmt.Printf("A newer encave is available: %s (you have %s).\n", latest, current)
	if _, err := exec.LookPath("go"); err != nil {
		fmt.Printf("  Update with:  go install %s@%s\n", modulePath, latest)
		return
	}
	if !confirm(fmt.Sprintf("Install encave %s now?", latest)) {
		return
	}
	if err := runGoInstall(latest); err != nil {
		errf("self-update failed: %v", err)
		fmt.Fprintf(os.Stderr, "  install it manually:  go install %s@%s\n", modulePath, latest)
		return
	}
	fmt.Printf("Installed encave %s. Re-run your command to use the new version.\n", latest)
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
// tag and, if one exists, offers to update before launching. It reuses updateOne
// so an accepted update behaves exactly like `encave update <ref>`; a declined or
// failed update falls through to launching the currently installed version. Like
// the self-check it is best-effort, throttled, and silent unless it has an
// actionable, interactive offer to make.
func maybeOfferAgentUpdate(root string, ref AgentRef, dir string) {
	if updateCheckDisabled() || !isInteractive() {
		return
	}
	if !gitutil.Available() || !gitutil.IsRepo(dir) || !gitutil.RemoteExists(dir, "origin") {
		return
	}

	cache := loadUpdateCache(root)
	now := time.Now()
	rec := cache.Agents[ref.String()]
	if rec.fresh(now) {
		return // checked recently; don't fetch or nag
	}

	// Fetch tags so "latest" reflects the origin, not just what we cloned. Record
	// the attempt regardless of outcome to throttle retries when offline.
	ferr := gitutil.Fetch(dir, "origin", "--tags", "--prune")
	rec.CheckedAt = now.Unix()

	latest := ""
	if ferr == nil {
		if l, lerr := gitutil.LatestSemverTag(dir); lerr == nil {
			latest = l
		}
	}
	rec.Latest = latest
	cache.Agents[ref.String()] = rec
	saveUpdateCache(root, cache)

	if latest == "" {
		return
	}
	current := gitutil.CurrentRef(dir)
	if current == latest {
		return
	}
	// Offer when the installed ref is an older release tag, or when it isn't a
	// release tag at all (e.g. installed on a branch) but a tagged release exists.
	if _, onTag := semver.Parse(current); onTag && !semver.IsNewer(latest, current) {
		return
	}

	fmt.Printf("A newer version of %s is available: %s (installed: %s).\n", ref, latest, current)
	if !confirm("Update before launching?") {
		return
	}
	if err := updateOne(root, ref, ""); err != nil {
		errf("update failed; launching the currently installed version: %v", err)
	}
}
