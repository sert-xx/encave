package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sert-xx/encave/internal/adapter"
	"github.com/sert-xx/encave/internal/agentmeta"
	"github.com/sert-xx/encave/internal/gitutil"
	"github.com/sert-xx/encave/internal/paths"
	"github.com/sert-xx/encave/internal/scan"
)

// cmdPublish commits and tags a draft for sharing. Before anything is committed
// it runs a fail-closed scan over the staged content; if a secret is detected
// it aborts (design doc §4.2). It also keeps .gitignore current for the target.
func cmdPublish(args []string) int {
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	tag := fs.String("tag", "", "release tag to create (e.g. v1.0.0)")
	message := fs.String("message", "", "commit message (default: derived from name/tag)")
	remote := fs.String("remote", "", "set 'origin' to this URL if not already configured")
	noTag := fs.Bool("no-tag", false, "commit without creating a tag")
	force := fs.Bool("force", false, "DANGER: commit even if the secret scan finds something")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: encave publish <name> [--tag vX.Y.Z] [--message msg] [--remote url] [--no-tag]")
		fs.PrintDefaults()
	}
	name, ok := parseOnePositional(fs, args)
	if !ok {
		return 2
	}

	if !gitutil.Available() {
		errf("git is required for publish but was not found on PATH")
		return 1
	}
	if *tag == "" && !*noTag {
		errf("a release tag is required for reproducible installs; pass --tag vX.Y.Z (or --no-tag to skip)")
		return 2
	}

	root, ok := mustRoot()
	if !ok {
		return 1
	}
	dir := paths.DraftDir(root, name)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		errf("draft %q not found at %s", name, dir)
		return 1
	}

	// Select the adapter from the draft's metadata so .gitignore matches the
	// target (falls back to the default target if metadata is missing).
	targetName := adapter.DefaultName
	if m, err := agentmeta.Load(dir); err == nil && m != nil && m.Target != "" {
		targetName = m.Target
	}
	ad, err := adapter.Get(targetName)
	if err != nil {
		errf("%v", err)
		return 1
	}

	if err := gitutil.Init(dir); err != nil {
		errf("initializing git repo: %v", err)
		return 1
	}
	if err := ensureGitignore(dir, ad); err != nil {
		errf("updating .gitignore: %v", err)
		return 1
	}
	if err := gitutil.AddAll(dir); err != nil {
		errf("staging changes: %v", err)
		return 1
	}

	staged, err := gitutil.StagedFiles(dir)
	if err != nil {
		errf("listing staged files: %v", err)
		return 1
	}
	if len(staged) == 0 {
		errf("nothing to publish (no changes staged in %s)", dir)
		return 1
	}

	// --- fail-closed secret scan over staged content ---
	findings := scanStaged(dir, staged)
	if len(findings) > 0 {
		fmt.Fprintf(os.Stderr, "\n✋ publish blocked: %d possible secret(s) detected in staged files.\n", len(findings))
		printFindingsTo(os.Stderr, findings, 50)
		fmt.Fprintln(os.Stderr, "\nRemove the secrets (store credentials with `encave auth set` instead),")
		fmt.Fprintln(os.Stderr, "or add false positives to .gitignore, then publish again.")
		if !*force {
			return 1
		}
		fmt.Fprintln(os.Stderr, "\n⚠  --force given: proceeding DESPITE the findings above. This may leak secrets.")
	}

	commitMsg := *message
	if commitMsg == "" {
		if *tag != "" {
			commitMsg = fmt.Sprintf("Publish %s %s", name, *tag)
		} else {
			commitMsg = fmt.Sprintf("Publish %s", name)
		}
	}
	if err := gitutil.Commit(dir, commitMsg); err != nil {
		errf("commit failed: %v", err)
		return 1
	}
	fmt.Printf("Committed: %s\n", commitMsg)

	if *tag != "" {
		if gitutil.TagExists(dir, *tag) {
			errf("tag %q already exists in %s", *tag, dir)
			return 1
		}
		if err := gitutil.Tag(dir, *tag, fmt.Sprintf("encave release %s", *tag)); err != nil {
			errf("tagging failed: %v", err)
			return 1
		}
		fmt.Printf("Tagged:    %s\n", *tag)
	}

	if *remote != "" && !gitutil.RemoteExists(dir, "origin") {
		if err := gitutil.AddRemote(dir, "origin", *remote); err != nil {
			errf("adding remote: %v", err)
			return 1
		}
		fmt.Printf("Remote:    origin -> %s\n", *remote)
	}

	fmt.Println()
	fmt.Println("Publish prepared locally. To share, push the branch and tag:")
	if gitutil.RemoteExists(dir, "origin") {
		fmt.Printf("  git -C %s push -u origin HEAD", dir)
		if *tag != "" {
			fmt.Printf(" && git -C %s push origin %s", dir, *tag)
		}
		fmt.Println()
	} else {
		fmt.Printf("  git -C %s remote add origin <github-url>\n", dir)
		fmt.Printf("  git -C %s push -u origin HEAD\n", dir)
	}
	return 0
}

// scanStaged scans the on-disk content of staged files for secrets.
func scanStaged(dir string, staged []string) []scan.Finding {
	var all []scan.Finding
	for _, rel := range staged {
		abs := filepath.Join(dir, rel)
		f, err := scan.File(rel, abs)
		if err != nil {
			// File may have been removed/renamed; still check the name.
			all = append(all, scan.FilenameFindings(rel)...)
			continue
		}
		all = append(all, f...)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].File != all[j].File {
			return all[i].File < all[j].File
		}
		return all[i].Line < all[j].Line
	})
	return all
}

// ensureGitignore creates or augments .gitignore with the adapter's recommended
// entries, preserving any lines already present.
func ensureGitignore(dir string, ad adapter.Adapter) error {
	path := filepath.Join(dir, ".gitignore")
	existing := map[string]bool{}
	var lines []string
	if data, err := os.ReadFile(path); err == nil {
		for _, l := range strings.Split(string(data), "\n") {
			lines = append(lines, l)
			existing[strings.TrimSpace(l)] = true
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	var added []string
	for _, l := range ad.GitignoreLines() {
		if !existing[strings.TrimSpace(l)] {
			added = append(added, l)
		}
	}
	if len(added) == 0 {
		return nil
	}
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
		lines = append(lines, "")
	}
	lines = append(lines, added...)
	out := strings.Join(lines, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return os.WriteFile(path, []byte(out), 0o644)
}

// printFindings prints up to max findings to stdout.
func printFindings(findings []scan.Finding, max int) { printFindingsTo(os.Stdout, findings, max) }

// printFindingsTo prints up to max findings to w.
func printFindingsTo(w *os.File, findings []scan.Finding, max int) {
	for i, f := range findings {
		if i >= max {
			fmt.Fprintf(w, "  ... and %d more\n", len(findings)-max)
			break
		}
		loc := f.File
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		if f.Sample != "" {
			fmt.Fprintf(w, "  • %s — %s [%s]\n", loc, f.Reason, f.Sample)
		} else {
			fmt.Fprintf(w, "  • %s — %s\n", loc, f.Reason)
		}
	}
}
