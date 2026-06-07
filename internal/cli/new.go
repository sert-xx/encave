package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sert-xx/encave/internal/adapter"
	"github.com/sert-xx/encave/internal/agentmeta"
	"github.com/sert-xx/encave/internal/fsutil"
	"github.com/sert-xx/encave/internal/paths"
	"github.com/sert-xx/encave/internal/scan"
)

// cmdNew scaffolds a new draft agent from the user's base home, applying the
// adapter's best-effort exclusion list (design doc §4.1). The static filter is
// only initial cleaning; the real gate is the publish-time scan.
func cmdNew(args []string) int {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	target := fs.String("target", adapter.DefaultName, "target CLI the agent is built for")
	from := fs.String("from", "", "source home to copy (default: target's base home, e.g. ~/.codex)")
	force := fs.Bool("force", false, "overwrite an existing draft of the same name")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: encave new <name> [--target codex] [--from <dir>] [--force]")
		fs.PrintDefaults()
	}
	name, ok := parseOnePositional(fs, args)
	if !ok {
		return 2
	}
	if name == "" || filepath.Base(name) != name {
		errf("invalid draft name %q", name)
		return 2
	}

	ad, err := adapter.Get(*target)
	if err != nil {
		errf("%v", err)
		return 2
	}

	root, ok := mustRoot()
	if !ok {
		return 1
	}

	src := *from
	if src == "" {
		src, err = ad.BaseHome()
		if err != nil {
			errf("%v", err)
			return 1
		}
	}
	if _, err := os.Stat(src); err != nil {
		errf("base home %q not found: %v", src, err)
		return 1
	}

	dst := paths.DraftDir(root, name)
	if _, err := os.Stat(dst); err == nil {
		if !*force {
			errf("draft %q already exists at %s (use --force to overwrite)", name, dst)
			return 1
		}
		if err := os.RemoveAll(dst); err != nil {
			errf("removing existing draft: %v", err)
			return 1
		}
	}
	if err := os.MkdirAll(paths.DraftsDir(root), 0o755); err != nil {
		errf("creating drafts dir: %v", err)
		return 1
	}

	res, err := fsutil.CopyTree(src, dst, ad.ScaffoldExcludes())
	if err != nil {
		errf("copying base home: %v", err)
		return 1
	}

	// Record which target this agent is for, so publish/run need no extra state.
	if err := agentmeta.Save(dst, agentmeta.Meta{Target: ad.Name()}); err != nil {
		errf("writing agent metadata: %v", err)
		return 1
	}

	fmt.Printf("Created draft %q at %s\n", name, dst)
	fmt.Printf("  target:   %s\n", ad.Name())
	fmt.Printf("  source:   %s\n", src)
	fmt.Printf("  copied:   %d files\n", res.FilesCopied)
	if len(res.Excluded) > 0 {
		fmt.Printf("  excluded: %d entries (secrets/state/logs filtered)\n", len(res.Excluded))
	}

	// Best-effort early warning: run the same scanner publish will use, but only
	// to inform — `new` never blocks.
	if findings := scanDraft(dst); len(findings) > 0 {
		fmt.Println()
		fmt.Printf("⚠  %d possible secret(s) remain in the draft (publish will block until resolved):\n", len(findings))
		printFindings(findings, 10)
	}

	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. Edit the draft (agents/, skills/, config.toml) under %s\n", dst)
	fmt.Printf("  2. Publish it:  encave publish %s --tag v1.0.0\n", name)
	return 0
}

// scanDraft scans every regular file in a draft directory and returns findings.
func scanDraft(dir string) []scan.Finding {
	var all []scan.Finding
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		if f, ferr := scan.File(rel, path); ferr == nil {
			all = append(all, f...)
		} else {
			all = append(all, scan.FilenameFindings(rel)...)
		}
		return nil
	})
	return all
}
