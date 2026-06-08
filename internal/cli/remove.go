package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sert-xx/encave/internal/paths"
)

// cmdRemove deletes an installed agent's directory. It is destructive, so it
// confirms first (or requires --force / a non-interactive caller to pass --force).
//
//	encave remove <owner>/<repo>
//	encave rm <owner>/<repo> --force
func cmdRemove(args []string) int {
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	force := fs.Bool("force", false, "remove without confirmation")
	fs.BoolVar(force, "f", false, "shorthand for --force")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: encave remove [<owner>/<repo>] [--force]")
		fs.PrintDefaults()
	}

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

	root, ok := mustRoot()
	if !ok {
		return 1
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
		r, ok := pickAgentRef(root, "Select an agent to remove:")
		if !ok {
			return 1
		}
		ref = r
	default:
		errf("no agent specified; usage: encave remove <owner>/<repo> [--force]")
		return 2
	}

	dir := paths.AgentDir(root, ref.Owner, ref.Repo)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		errf("agent %s is not installed (looked in %s)", ref, dir)
		return 1
	}

	if !*force {
		if !isInteractive() {
			errf("refusing to remove %s without confirmation; pass --force", ref)
			return 1
		}
		if !confirm(fmt.Sprintf("Remove agent %s at %s?", ref, dir)) {
			fmt.Println("Aborted.")
			return 0
		}
	}

	if err := os.RemoveAll(dir); err != nil {
		errf("removing %s: %v", dir, err)
		return 1
	}

	// Clean up the owner directory if it's now empty.
	ownerDir := filepath.Dir(dir)
	if entries, err := os.ReadDir(ownerDir); err == nil && len(entries) == 0 {
		_ = os.Remove(ownerDir)
	}

	fmt.Printf("Removed %s (%s)\n", ref, dir)
	fmt.Printf("Note: any keyring credential for this agent is left as-is; clear it with `encave auth clear --agent %s` if you stored one.\n", ref)
	return 0
}
