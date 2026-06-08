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
	yes := fs.Bool("yes", false, "skip the push confirmation prompt and push (for automation)")
	fs.BoolVar(yes, "y", false, "shorthand for --yes")
	noPush := fs.Bool("no-push", false, "commit and tag only; never push")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: encave publish [<owner>/<repo>] [--tag vX.Y.Z] [--message msg] [--remote url] [--no-tag] [--no-push] [-y]")
		fmt.Fprintln(os.Stderr, "  On a terminal, missing values (agent, tag, remote) are prompted for.")
		fs.PrintDefaults()
	}
	// The agent reference, when present, is the first token (before any flags);
	// it is optional so `encave publish` can prompt interactively.
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
		errf("git is required for publish but was not found on PATH")
		return 1
	}

	root, ok := mustRoot()
	if !ok {
		return 1
	}

	interactive := isInteractive()

	// Resolve the agent: explicit reference, or interactive selection.
	var ref AgentRef
	switch {
	case refArg != "":
		r, err := parseAgentRef(refArg)
		if err != nil {
			errf("%v", err)
			fmt.Fprintln(os.Stderr, "  agents are identified by their GitHub identity, e.g.  encave publish dai/review-agent")
			return 2
		}
		ref = r
	case interactive:
		r, ok := pickAgentRef(root, "Select an agent to publish:")
		if !ok {
			return 1
		}
		ref = r
	default:
		errf("no agent specified; usage: encave publish <owner>/<repo> [flags]")
		return 2
	}

	// Resolve the tag: explicit, opted out, or prompted.
	if *tag == "" && !*noTag {
		if interactive {
			in := promptLine("Release tag (e.g. v1.0.0; blank = no tag)", "")
			if in == "" {
				*noTag = true
			} else {
				*tag = in
			}
		} else {
			errf("a release tag is required for reproducible installs; pass --tag vX.Y.Z (or --no-tag to skip)")
			return 2
		}
	}

	dir := paths.AgentDir(root, ref.Owner, ref.Repo)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		errf("agent %s not found at %s", ref, dir)
		fmt.Fprintf(os.Stderr, "  create it first:  encave new %s\n", ref)
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

	if len(staged) > 0 {
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
				commitMsg = fmt.Sprintf("Publish %s %s", ref, *tag)
			} else {
				commitMsg = fmt.Sprintf("Publish %s", ref)
			}
		}
		if err := gitutil.Commit(dir, commitMsg); err != nil {
			errf("commit failed: %v", err)
			return 1
		}
		fmt.Printf("Committed: %s\n", commitMsg)
	} else if !gitutil.HasCommits(dir) {
		// Truly empty: nothing staged and no existing commit to tag/push.
		errf("nothing to publish (no commits and no changes staged in %s)", dir)
		fmt.Fprintf(os.Stderr, "  add some agent files first, then publish.\n")
		return 1
	} else {
		// Clean tree but an existing commit — still allow tagging/pushing it.
		fmt.Println("No new changes to commit; tagging/pushing the current commit.")
	}

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

	if !gitutil.RemoteExists(dir, "origin") {
		url := *remote
		if url == "" && interactive {
			// No default: in a corporate setting, accidentally accepting a
			// public github.com default could leak the agent. The user must
			// type the full remote URL (blank to skip).
			if confirm("No git remote is set. Add 'origin' now?") {
				url = promptLine("Remote URL (blank to skip)", "")
			}
		}
		if url != "" {
			if err := gitutil.AddRemote(dir, "origin", url); err != nil {
				errf("adding remote: %v", err)
				return 1
			}
			fmt.Printf("Remote:    origin -> %s\n", url)
		}
	}

	fmt.Println()
	return finishPublish(dir, ref, *tag, *noPush, *yes)
}

// finishPublish handles the post-commit/tag step: when an origin remote is
// configured, it offers (or, with --yes, performs) the push; when none is
// configured, it stops and explains how to set one — using the agent's GitHub
// identity to suggest the exact remote URLs. The commit and tag already exist
// regardless, so this never undoes work — it only decides about pushing.
func finishPublish(dir string, ref AgentRef, tag string, noPush, yes bool) int {
	if !gitutil.RemoteExists(dir, "origin") {
		errf("no git remote configured, so nothing was pushed.")
		fmt.Fprintln(os.Stderr, "Set a remote, then re-run publish (or push manually). For example:")
		fmt.Fprintf(os.Stderr, "  encave publish %s --tag <tag> --remote git@github.com:%s.git\n", ref, ref)
		fmt.Fprintf(os.Stderr, "  (HTTPS: https://github.com/%s.git)\n", ref)
		fmt.Fprintf(os.Stderr, "  or:  git -C %s remote add origin git@github.com:%s.git\n", dir, ref)
		fmt.Fprintln(os.Stderr, "(The commit and tag were created locally and are ready to push.)")
		return 1
	}

	url, _ := gitutil.RemoteURL(dir, "origin")

	switch pushPlan(noPush, yes, isInteractive()) {
	case pushSkip:
		fmt.Println("Skipping push. To push manually:")
		fmt.Printf("  git -C %s push -u origin HEAD", dir)
		if tag != "" {
			fmt.Printf(" && git -C %s push origin %s", dir, tag)
		}
		fmt.Println()
		return 0

	case pushConfirm:
		prompt := fmt.Sprintf("Push to %s now?", url)
		if !confirm(prompt) {
			fmt.Println("Not pushed. To push later:")
			fmt.Printf("  git -C %s push -u origin HEAD", dir)
			if tag != "" {
				fmt.Printf(" && git -C %s push origin %s", dir, tag)
			}
			fmt.Println()
			return 0
		}
	case pushAuto:
		// proceed without prompting
	}

	return doPush(dir, tag, url)
}

// doPush pushes the current branch (and the tag, if any) to origin.
func doPush(dir, tag, url string) int {
	branch := gitutil.CurrentBranch(dir)
	if branch == "" {
		errf("cannot push: HEAD is detached (no current branch)")
		return 1
	}
	fmt.Printf("Pushing %s to %s ...\n", branch, url)
	if err := gitutil.Push(dir, "-u", "origin", "HEAD"); err != nil {
		errf("pushing branch: %v", err)
		return 1
	}
	if tag != "" {
		if err := gitutil.Push(dir, "origin", tag); err != nil {
			errf("pushing tag %s: %v", tag, err)
			return 1
		}
		fmt.Printf("Pushed branch %s and tag %s.\n", branch, tag)
	} else {
		fmt.Printf("Pushed branch %s.\n", branch)
	}
	return 0
}

// pushMode is the resolved decision about whether/how to push.
type pushMode int

const (
	pushSkip    pushMode = iota // do not push (print manual instructions)
	pushConfirm                 // ask the user interactively
	pushAuto                    // push without prompting
)

// pushPlan decides how to handle pushing when a remote exists. Precedence:
// --no-push wins; then --yes; then, if interactive, prompt; otherwise (a
// non-interactive session without --yes) skip for safety.
func pushPlan(noPush, yes, interactive bool) pushMode {
	switch {
	case noPush:
		return pushSkip
	case yes:
		return pushAuto
	case interactive:
		return pushConfirm
	default:
		return pushSkip
	}
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

// ensureGitignore creates or updates .gitignore, appending the adapter's
// recommended entries to whatever is already there (e.g. a .gitignore copied
// from the user's home). Existing content is preserved and ordered first; the
// adapter's lines follow. Duplicate non-blank lines are removed (first
// occurrence wins) and consecutive blank lines are collapsed, so re-running is
// idempotent.
func ensureGitignore(dir string, ad adapter.Adapter) error {
	path := filepath.Join(dir, ".gitignore")
	var existing []string
	if data, err := os.ReadFile(path); err == nil {
		existing = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	} else if !os.IsNotExist(err) {
		return err
	}

	var out []string
	seen := map[string]bool{}
	add := func(raw string) {
		t := strings.TrimSpace(raw)
		if t == "" {
			// Keep a single blank separator; drop leading/duplicate blanks.
			if len(out) == 0 || strings.TrimSpace(out[len(out)-1]) == "" {
				return
			}
			out = append(out, "")
			return
		}
		if seen[t] {
			return
		}
		seen[t] = true
		out = append(out, raw)
	}

	for _, l := range existing {
		add(l)
	}
	add("") // blank separator before the encave block (no-op if not needed)
	for _, l := range ad.GitignoreLines() {
		add(l)
	}
	// Trim trailing blank lines.
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}

	return os.WriteFile(path, []byte(strings.Join(out, "\n")+"\n"), 0o644)
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
