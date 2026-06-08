package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sert-xx/encave/internal/adapter"
	"github.com/sert-xx/encave/internal/agentmeta"
	"github.com/sert-xx/encave/internal/fsutil"
	"github.com/sert-xx/encave/internal/gitutil"
	"github.com/sert-xx/encave/internal/paths"
	"github.com/sert-xx/encave/internal/scan"
)

// cmdNew scaffolds a new agent from the user's base home into the shared agent
// location (<root>/<owner>/<repo>, the same place `install` uses), applying the
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

	dst := paths.AgentDir(root, ref.Owner, ref.Repo)
	if _, err := os.Stat(dst); err == nil {
		if !*force {
			errf("agent %s already exists at %s (use --force to overwrite)", ref, dst)
			return 1
		}
		if err := os.RemoveAll(dst); err != nil {
			errf("removing existing agent: %v", err)
			return 1
		}
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		errf("creating agent directory: %v", err)
		return 1
	}

	res, err := fsutil.CopyTree(src, dst, ad.ScaffoldExcludes())
	if err != nil {
		errf("copying base home: %v", err)
		return 1
	}

	// Write the whitelist-filtered base config. The raw config.toml is excluded
	// from the copy; the effective config.toml is generated at launch by merging
	// this base with the user's own home config.
	var srcConfig []byte // the user's full config, used for README MCP listing
	configStatus := "n/a"
	if base, eff := ad.ConfigLayout(); base != "" {
		full, ferr := os.ReadFile(filepath.Join(src, eff))
		if ferr != nil && !os.IsNotExist(ferr) {
			errf("reading source config: %v", ferr)
			return 1
		}
		srcConfig = full
		baseData, berr := ad.BuildBaseConfig(full)
		if berr != nil {
			errf("filtering config to a whitelist: %v", berr)
			return 1
		}
		if err := os.WriteFile(filepath.Join(dst, base), baseData, 0o644); err != nil {
			errf("writing %s: %v", base, err)
			return 1
		}
		configStatus = fmt.Sprintf("%s (agent-owned keys only)", base)
	}

	// Record which target this agent is for, so publish/run need no extra state.
	if err := agentmeta.Save(dst, agentmeta.Meta{Target: ad.Name()}); err != nil {
		errf("writing agent metadata: %v", err)
		return 1
	}

	// Set up .gitignore now (appending the adapter's entries to any .gitignore
	// copied from the user's home, de-duplicated) so generated/runtime files like
	// config.toml are ignored even before the first publish.
	if err := ensureGitignore(dst, ad); err != nil {
		errf("writing .gitignore: %v", err)
		return 1
	}

	// Create the personal-subdir symlinks now (not only at launch) so they're
	// visible while you edit the agent — avoiding accidental copies of, e.g.,
	// your rules. They point at this machine's home and are gitignored.
	linkStatus := "none"
	if links := ensurePersonalLinks(ad, dst); len(links) > 0 {
		var parts []string
		for _, l := range links {
			parts = append(parts, fmt.Sprintf("%s -> %s", filepath.Base(l.dst), l.src))
		}
		linkStatus = strings.Join(parts, ", ")
	}

	// Generate a README template documenting the encave install/auth/run flow,
	// unless the user opted out or the copied home already has a README.
	readmeStatus := "skipped (--no-readme)"
	if !*noReadme {
		readmeStatus = maybeWriteReadme(dst, ref, ad, srcConfig)
	}

	// Initialize a git repo and make an initial commit containing only the
	// README. The rest of the agent is committed later by `publish`, after the
	// fail-closed secret scan — so nothing unscanned lands in a commit here.
	gitStatus := ""
	if fileExists(filepath.Join(dst, "README.md")) {
		gitStatus = gitInitCommitReadme(dst)
	}

	fmt.Printf("Created agent %s at %s\n", ref, dst)
	fmt.Printf("  target:   %s\n", ad.Name())
	fmt.Printf("  source:   %s\n", src)
	fmt.Printf("  copied:   %d files\n", res.FilesCopied)
	if len(res.Excluded) > 0 {
		fmt.Printf("  excluded: %d entries (secrets/state/logs filtered)\n", len(res.Excluded))
	}
	fmt.Printf("  config:   %s\n", configStatus)
	fmt.Printf("  links:    %s\n", linkStatus)
	fmt.Printf("  README:   %s\n", readmeStatus)
	if gitStatus != "" {
		fmt.Printf("  git:      %s\n", gitStatus)
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
	fmt.Printf("  1. Edit the agent (agents/, skills/, config.toml) under %s\n", dst)
	fmt.Printf("  2. Try it locally:  encave %s\n", ref)
	fmt.Printf("  3. Publish it:      encave publish %s --tag v1.0.0\n", ref)
	return 0
}

// maybeWriteReadme writes the README.md template into the agent, always
// overwriting any README copied from the base home (that generic ~/.codex README
// rarely applies to a specific agent; copy it back manually if you want it). It
// returns a short status string for the summary output. Failures are reported
// but non-fatal — scaffolding succeeds regardless.
func maybeWriteReadme(dst string, ref AgentRef, ad adapter.Adapter, srcConfig []byte) string {
	path := filepath.Join(dst, "README.md")
	replaced := false
	if _, err := os.Stat(path); err == nil {
		replaced = true
	}

	// Best-effort: surface, from the author's full source config, the auth env
	// vars, the model providers, and the MCP servers the agent expects (none of
	// which are packaged) so the README can list them as setup requirements.
	authVars, _ := ad.AuthEnvVars(srcConfig)
	providers, _ := ad.ModelProviders(srcConfig)
	mcps, _ := ad.MCPServers(srcConfig)

	content := renderAgentReadme(ref.Owner, ref.Repo, ad.Name(), authVars, providers, mcps)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Sprintf("not written (%v)", err)
	}
	if replaced {
		return "generated README.md (replaced the copied one; edit the TODOs)"
	}
	return "generated README.md (edit the TODOs)"
}

// fileExists reports whether path exists (as any kind of file).
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// gitInitCommitReadme initializes a git repo in dir and makes an initial commit
// containing only README.md. It is best-effort: if git is unavailable it skips,
// and any git error is reported in the returned status without failing `new`.
func gitInitCommitReadme(dir string) string {
	if !gitutil.Available() {
		return "skipped (git not found)"
	}
	if err := gitutil.Init(dir); err != nil {
		return fmt.Sprintf("skipped (init failed: %v)", err)
	}
	if err := gitutil.AddPaths(dir, "README.md"); err != nil {
		return fmt.Sprintf("init only (add failed: %v)", err)
	}
	if err := gitutil.Commit(dir, "Initial commit"); err != nil {
		return fmt.Sprintf("init only (commit failed: %v)", err)
	}
	return "git init + initial commit (README.md)"
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
