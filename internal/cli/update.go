package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sert-xx/encave/internal/gitutil"
	"github.com/sert-xx/encave/internal/paths"
)

// cmdUpdate updates installed agents to a release tag by fetching their origin
// and checking out the requested (or latest) tag (design doc §4.3 reproducibility).
//
//	encave update <owner>/<repo>            # latest release tag
//	encave update <owner>/<repo> --tag vX   # a specific tag
//	encave update --all                     # every agent to its latest tag
func cmdUpdate(args []string) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	tag := fs.String("tag", "", "tag to check out (default: latest release tag)")
	all := fs.Bool("all", false, "update every installed agent to its latest tag")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: encave update [<owner>/<repo>] [--tag vX.Y.Z] | --all")
		fs.PrintDefaults()
	}

	// Optional leading agent reference, like run/publish.
	var refArg string
	var flagArgs []string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		refArg = args[0]
		flagArgs = args[1:]
	} else {
		flagArgs = args
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		errf("unexpected arguments %v (the agent reference must come first)", fs.Args())
		return 2
	}

	if !gitutil.Available() {
		errf("git is required for update but was not found on PATH")
		return 1
	}
	root, ok := mustRoot()
	if !ok {
		return 1
	}

	if *all {
		if refArg != "" || *tag != "" {
			errf("--all cannot be combined with an agent reference or --tag")
			return 2
		}
		return updateAll(root)
	}

	// Resolve the agent: explicit reference, or interactive selection.
	var ref AgentRef
	switch {
	case refArg != "":
		r, err := parseAgentRef(refArg)
		if err != nil {
			errf("%v", err)
			return 2
		}
		ref = r
	case isInteractive():
		r, ok := pickAgentRef(root, "Select an agent to update:")
		if !ok {
			return 1
		}
		ref = r
	default:
		errf("no agent specified; usage: encave update <owner>/<repo> [--tag vX.Y.Z] | --all")
		return 2
	}

	if err := updateOne(root, ref, *tag); err != nil {
		errf("%v", err)
		return 1
	}
	return 0
}

// updateOne fetches an agent's origin and checks out the requested tag (or the
// latest release tag when tag is empty).
func updateOne(root string, ref AgentRef, tag string) error {
	dir := paths.AgentDir(root, ref.Owner, ref.Repo)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fmt.Errorf("agent %s is not installed (looked in %s)", ref, dir)
	}
	if !gitutil.IsRepo(dir) || !gitutil.RemoteExists(dir, "origin") {
		return fmt.Errorf("agent %s has no 'origin' remote to update from (locally authored?)", ref)
	}

	before := gitutil.CurrentRef(dir)
	fmt.Printf("Fetching %s ...\n", ref)
	if err := gitutil.Fetch(dir, "origin", "--tags", "--prune"); err != nil {
		return fmt.Errorf("fetching %s: %w", ref, err)
	}

	target := tag
	if target == "" {
		latest, err := gitutil.LatestSemverTag(dir)
		if err != nil {
			return fmt.Errorf("resolving latest tag for %s: %w", ref, err)
		}
		if latest == "" {
			return fmt.Errorf("%s has no release tags to update to", ref)
		}
		target = latest
	}
	if !gitutil.TagExists(dir, target) {
		return fmt.Errorf("tag %q not found for %s (after fetch)", target, ref)
	}
	if err := gitutil.CheckoutTag(dir, target); err != nil {
		return fmt.Errorf("checking out %s for %s: %w", target, ref, err)
	}

	if before == target {
		fmt.Printf("%s: already at %s\n", ref, target)
	} else {
		fmt.Printf("%s: %s -> %s\n", ref, before, target)
	}
	return nil
}

// updateAll updates every installed agent that has an origin remote to its
// latest release tag, continuing past per-agent errors and printing a summary.
func updateAll(root string) int {
	agents := findInstalled(root)
	if len(agents) == 0 {
		fmt.Printf("No agents installed under %s\n", root)
		return 0
	}
	var updated, skipped, failed int
	for _, a := range agents {
		ref, err := parseAgentRef(a.ref)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", a.ref, err)
			skipped++
			continue
		}
		dir := paths.AgentDir(root, ref.Owner, ref.Repo)
		if !gitutil.IsRepo(dir) || !gitutil.RemoteExists(dir, "origin") {
			fmt.Printf("%s: skipped (no origin remote)\n", ref)
			skipped++
			continue
		}
		if err := updateOne(root, ref, ""); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", ref, err)
			failed++
			continue
		}
		updated++
	}
	fmt.Printf("\nDone: %d updated, %d skipped, %d failed.\n", updated, skipped, failed)
	if failed > 0 {
		return 1
	}
	return 0
}
