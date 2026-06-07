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
	noReadme := fs.Bool("no-readme", false, "do not generate a README.md template")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: encave new <owner>/<repo> [--target codex] [--from <dir>] [--force] [--no-readme]")
		fs.PrintDefaults()
	}
	pos, ok := parseOnePositional(fs, args)
	if !ok {
		return 2
	}
	ref, err := parseAgentRef(pos)
	if err != nil {
		errf("%v", err)
		fmt.Fprintln(os.Stderr, "  the agent name is its GitHub identity, e.g.  encave new dai/review-agent")
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

	dst := paths.DraftDir(root, ref.Owner, ref.Repo)
	if _, err := os.Stat(dst); err == nil {
		if !*force {
			errf("draft %s already exists at %s (use --force to overwrite)", ref, dst)
			return 1
		}
		if err := os.RemoveAll(dst); err != nil {
			errf("removing existing draft: %v", err)
			return 1
		}
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
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

	// Generate a README template documenting the encave install/auth/run flow,
	// unless the user opted out or the copied home already has a README.
	readmeStatus := "skipped (--no-readme)"
	if !*noReadme {
		readmeStatus = maybeWriteReadme(dst, ref, ad)
	}

	fmt.Printf("Created draft %s at %s\n", ref, dst)
	fmt.Printf("  target:   %s\n", ad.Name())
	fmt.Printf("  source:   %s\n", src)
	fmt.Printf("  copied:   %d files\n", res.FilesCopied)
	if len(res.Excluded) > 0 {
		fmt.Printf("  excluded: %d entries (secrets/state/logs filtered)\n", len(res.Excluded))
	}
	fmt.Printf("  README:   %s\n", readmeStatus)

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
	fmt.Printf("  2. Publish it:  encave publish %s --tag v1.0.0\n", ref)
	return 0
}

// maybeWriteReadme writes a README.md template into the draft unless one already
// exists (an existing README from the copied home is never clobbered). It
// returns a short status string for the summary output. Failures are reported
// but non-fatal — scaffolding succeeds regardless.
func maybeWriteReadme(dst string, ref AgentRef, ad adapter.Adapter) string {
	path := filepath.Join(dst, "README.md")
	if _, err := os.Stat(path); err == nil {
		return "skipped (README.md already present)"
	} else if !os.IsNotExist(err) {
		return fmt.Sprintf("skipped (%v)", err)
	}

	// Best-effort: surface the agent's auth env vars in the template.
	authVars, _ := ad.AuthEnvVars(dst)

	content := renderAgentReadme(ref.Owner, ref.Repo, ad.Name(), authVars)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Sprintf("not written (%v)", err)
	}
	return "generated README.md (edit the TODOs)"
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
