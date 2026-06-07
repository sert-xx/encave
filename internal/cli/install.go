package cli

import (
	"flag"
	"fmt"
	"os"

	"github.com/sert-xx/encave/internal/adapter"
	"github.com/sert-xx/encave/internal/agentmeta"
	"github.com/sert-xx/encave/internal/gitutil"
	"github.com/sert-xx/encave/internal/paths"
)

// cmdInstall clones a published agent and checks out a tag into
// <root>/<owner>/<repo> (design doc §4.3). Tags are preferred for byte-for-byte
// reproducibility; without one, encave uses the latest semver tag if present, or
// otherwise the default branch. After checkout it verifies the repo is actually
// an encave-managed agent (has a valid .encave.toml) unless --no-verify is given.
func cmdInstall(args []string) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	tag := fs.String("tag", "", "tag to check out (recommended for reproducibility)")
	force := fs.Bool("force", false, "reinstall, replacing any existing copy")
	noVerify := fs.Bool("no-verify", false, "skip the check that the repo is an encave-managed agent")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: encave install <github-url> [--tag vX.Y.Z] [--force] [--no-verify]")
		fs.PrintDefaults()
	}
	urlArg, ok := parseOnePositional(fs, args)
	if !ok {
		return 2
	}

	if !gitutil.Available() {
		errf("git is required for install but was not found on PATH")
		return 1
	}

	src, err := parseRepoSource(urlArg)
	if err != nil {
		errf("%v", err)
		return 2
	}

	root, ok := mustRoot()
	if !ok {
		return 1
	}
	dst := paths.AgentDir(root, src.Owner, src.Repo)

	if _, err := os.Stat(dst); err == nil {
		if !*force {
			errf("agent %s/%s already installed at %s (use --force to reinstall)", src.Owner, src.Repo, dst)
			return 1
		}
		if err := os.RemoveAll(dst); err != nil {
			errf("removing existing install: %v", err)
			return 1
		}
	}
	if err := os.MkdirAll(paths.AgentDir(root, src.Owner, ""), 0o755); err != nil {
		errf("creating owner dir: %v", err)
		return 1
	}

	fmt.Printf("Cloning %s ...\n", src.CloneURL)
	if err := gitutil.Clone(src.CloneURL, dst); err != nil {
		errf("clone failed: %v", err)
		return 1
	}

	checkoutTag := *tag
	if checkoutTag == "" {
		latest, lerr := gitutil.LatestSemverTag(dst)
		if lerr == nil && latest != "" {
			checkoutTag = latest
			fmt.Printf("No --tag given; using latest release tag %s\n", latest)
		}
	}
	if checkoutTag != "" {
		if !gitutil.TagExists(dst, checkoutTag) {
			errf("tag %q not found in %s/%s", checkoutTag, src.Owner, src.Repo)
			_ = os.RemoveAll(dst)
			return 1
		}
		if err := gitutil.CheckoutTag(dst, checkoutTag); err != nil {
			errf("checkout failed: %v", err)
			return 1
		}
	} else {
		fmt.Printf("No tags found; staying on default branch %s (not pinned — re-runs may drift)\n", gitutil.CurrentRef(dst))
	}

	// Verify the cloned repo is actually an encave-managed agent before keeping
	// it. A repo without a valid .encave.toml is almost certainly not meant to be
	// launched as an isolated agent home.
	target := adapter.DefaultName
	if !*noVerify {
		t, verr := checkInstalledAgent(dst)
		if verr != nil {
			errf("%s/%s does not look like an encave-managed agent: %v", src.Owner, src.Repo, verr)
			fmt.Fprintf(os.Stderr, "  encave agents are created with `encave new` (which writes %s).\n", agentmeta.FileName)
			fmt.Fprintln(os.Stderr, "  if you trust this repo and know it is a valid agent home, re-run with --no-verify.")
			_ = os.RemoveAll(dst)
			return 1
		}
		target = t
		// Soft check: warn if the home doesn't look right for its target, but
		// don't block (it declared itself as an encave agent).
		if ad, aerr := adapter.Get(target); aerr == nil {
			if werr := ad.Validate(dst); werr != nil {
				fmt.Fprintf(os.Stderr, "encave: warning: %v\n", werr)
			}
		}
	} else {
		target = agentmeta.DefaultTargetOr(dst)
	}

	fmt.Printf("Installed %s/%s [%s] -> %s (at %s)\n", src.Owner, src.Repo, target, dst, gitutil.CurrentRef(dst))
	fmt.Println()
	fmt.Printf("Launch it with:  encave %s/%s\n", src.Owner, src.Repo)
	return 0
}

// checkInstalledAgent verifies that a freshly cloned directory is an
// encave-managed agent: it must contain a readable .encave.toml that names a
// known target adapter. It returns the resolved target name on success.
func checkInstalledAgent(dst string) (string, error) {
	m, err := agentmeta.Load(dst)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", agentmeta.FileName, err)
	}
	if m == nil {
		return "", fmt.Errorf("no %s found", agentmeta.FileName)
	}
	target := m.Target
	if target == "" {
		target = adapter.DefaultName
	}
	if _, err := adapter.Get(target); err != nil {
		return "", fmt.Errorf("%s declares unknown target %q", agentmeta.FileName, m.Target)
	}
	return target, nil
}
