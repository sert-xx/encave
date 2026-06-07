package cli

import (
	"flag"
	"fmt"
	"os"

	"github.com/sert-xx/encave/internal/gitutil"
	"github.com/sert-xx/encave/internal/paths"
)

// cmdInstall clones a published agent and checks out a tag into
// <root>/<owner>/<repo> (design doc §4.3). Tags are preferred for byte-for-byte
// reproducibility; without one, encave uses the latest semver tag if present, or
// otherwise the default branch.
func cmdInstall(args []string) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	tag := fs.String("tag", "", "tag to check out (recommended for reproducibility)")
	force := fs.Bool("force", false, "reinstall, replacing any existing copy")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: encave install <github-url> [--tag vX.Y.Z] [--force]")
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

	fmt.Printf("Installed %s/%s -> %s (at %s)\n", src.Owner, src.Repo, dst, gitutil.CurrentRef(dst))
	fmt.Println()
	fmt.Printf("Launch it with:  encave %s/%s\n", src.Owner, src.Repo)
	return 0
}
