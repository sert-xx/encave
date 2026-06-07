package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/sert-xx/encave/internal/agentmeta"
	"github.com/sert-xx/encave/internal/gitutil"
	"github.com/sert-xx/encave/internal/paths"
)

// cmdList shows installed agents (and local drafts), with the ref encave knows
// them by. It's a convenience for discovering what is launchable.
func cmdList(args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, "usage: encave list") }
	if err := fs.Parse(args); err != nil {
		return 2
	}

	root, ok := mustRoot()
	if !ok {
		return 1
	}

	installed := findInstalled(root)
	drafts := findDrafts(root)

	if len(installed) == 0 && len(drafts) == 0 {
		fmt.Printf("No agents installed and no drafts under %s\n", root)
		fmt.Println("Get started:  encave install github.com/<owner>/<repo>")
		return 0
	}

	if len(installed) > 0 {
		fmt.Println("Installed agents:")
		for _, a := range installed {
			fmt.Printf("  %-30s [%s] %s\n", a.ref, a.target, a.ref2)
		}
	}
	if len(drafts) > 0 {
		if len(installed) > 0 {
			fmt.Println()
		}
		fmt.Println("Drafts (unpublished):")
		for _, d := range drafts {
			fmt.Printf("  %-30s [%s]\n", d.name, d.target)
		}
	}
	return 0
}

type installedAgent struct {
	ref    string // owner/repo
	ref2   string // current git ref
	target string
}

func findInstalled(root string) []installedAgent {
	var out []installedAgent
	owners, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, o := range owners {
		name := o.Name()
		if !o.IsDir() || name == "_drafts" || name[0] == '.' {
			continue
		}
		ownerDir := filepath.Join(root, name)
		repos, err := os.ReadDir(ownerDir)
		if err != nil {
			continue
		}
		for _, r := range repos {
			if !r.IsDir() {
				continue
			}
			agentDir := filepath.Join(ownerDir, r.Name())
			ref := name + "/" + r.Name()
			target := agentmeta.DefaultTargetOr(agentDir)
			gitRef := "-"
			if gitutil.IsRepo(agentDir) {
				gitRef = gitutil.CurrentRef(agentDir)
			}
			out = append(out, installedAgent{ref: ref, ref2: gitRef, target: target})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ref < out[j].ref })
	return out
}

type draftAgent struct {
	name   string
	target string
}

func findDrafts(root string) []draftAgent {
	var out []draftAgent
	entries, err := os.ReadDir(paths.DraftsDir(root))
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(paths.DraftsDir(root), e.Name())
		out = append(out, draftAgent{name: e.Name(), target: agentmeta.DefaultTargetOr(dir)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}
