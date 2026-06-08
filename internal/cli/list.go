package cli

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/sert-xx/encave/internal/adapter"
	"github.com/sert-xx/encave/internal/agentmeta"
	"github.com/sert-xx/encave/internal/gitutil"
)

// cmdList shows the agents encave knows about (locally authored via `new` and
// fetched via `install` share one location), with the ref each is known by.
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

	agents := findInstalled(root)
	if len(agents) == 0 {
		fmt.Printf("No agents under %s\n", root)
		fmt.Println("Get started:  encave install github.com/<owner>/<repo>")
		fmt.Println("        or:   encave new <owner>/<repo>")
		return 0
	}

	fmt.Println("Agents:")
	for _, a := range agents {
		fmt.Printf("  %-30s [%s] %s\n", a.ref, a.target, a.ref2)
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

// runSelection is the outcome of the interactive launch picker: either an
// installed agent reference, or the user's native default home.
type runSelection struct {
	native bool
	ref    AgentRef
}

// pickLaunchTarget lets the user choose interactively what `encave run` should
// launch: one of the installed agents, or their own default home (no isolation,
// no credential injection). The native-home choice is always offered — even with
// no agents installed — so encave can be the single entry point. It returns
// ok=false when there is no terminal to prompt on, or the user cancels.
func pickLaunchTarget(root string) (runSelection, bool) {
	agents := findInstalled(root)

	if !isInteractive() {
		errf("no agent specified and no terminal available to choose interactively")
		fmt.Fprintln(os.Stderr, "  an installed agent:  encave run <owner>/<repo>")
		fmt.Fprintf(os.Stderr, "  your default home:   encave run %s\n", nativeRef)
		fmt.Fprintln(os.Stderr, "  or list agents:      encave list")
		return runSelection{}, false
	}

	labels := make([]string, 0, len(agents)+1)
	for _, a := range agents {
		labels = append(labels, fmt.Sprintf("%-30s [%s] %s", a.ref, a.target, a.ref2))
	}
	// The native default home is always the last entry.
	labels = append(labels, fmt.Sprintf("%-30s (your own setup; no isolation/injection)",
		"your default "+adapter.DefaultName+" home"))

	idx, ok := selectFromList("Choose what to launch (↑/↓, Enter; q to cancel):", labels)
	if !ok {
		return runSelection{}, false
	}
	if idx == len(agents) { // the native-home entry
		return runSelection{native: true}, true
	}
	ref, perr := parseAgentRef(agents[idx].ref)
	if perr != nil {
		errf("%v", perr)
		return runSelection{}, false
	}
	return runSelection{ref: ref}, true
}

// pickAgentRef lists the installed agents and lets the user choose one
// interactively, returning its reference. Used by commands like `publish` when
// no agent is given. ok=false when there are no agents or the user cancels.
func pickAgentRef(root, header string) (AgentRef, bool) {
	agents := findInstalled(root)
	if len(agents) == 0 {
		errf("no agents found in %s", root)
		fmt.Fprintln(os.Stderr, "  create one first:  encave new <owner>/<repo>")
		return AgentRef{}, false
	}
	labels := make([]string, len(agents))
	for i, a := range agents {
		labels[i] = fmt.Sprintf("%-30s [%s] %s", a.ref, a.target, a.ref2)
	}
	idx, ok := selectFromList(header, labels)
	if !ok {
		return AgentRef{}, false
	}
	ref, perr := parseAgentRef(agents[idx].ref)
	if perr != nil {
		errf("%v", perr)
		return AgentRef{}, false
	}
	return ref, true
}

// numberedSelect is the fallback for selectFromList when an interactive ↑/↓ menu
// can't be shown: it prints a numbered list and reads a choice from stdin.
func numberedSelect(header string, labels []string) (int, bool) {
	fmt.Println(header)
	for i, l := range labels {
		fmt.Printf("  %2d) %s\n", i+1, l)
	}
	reader := bufio.NewReader(os.Stdin)
	for attempts := 0; attempts < 3; attempts++ {
		fmt.Printf("Select [1-%d] (q to cancel): ", len(labels))
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			fmt.Println()
			return 0, false
		}
		idx, cancel, cerr := parseAgentChoice(line, len(labels))
		if cancel {
			return 0, false
		}
		if cerr != nil {
			fmt.Fprintf(os.Stderr, "  %v\n", cerr)
			continue
		}
		return idx, true
	}
	errf("no valid selection made")
	return 0, false
}

// parseAgentChoice interprets one line of picker input against a list of size n.
// It returns the selected 0-based index, whether the user asked to cancel
// (q/quit), or an error describing why the input was not a valid choice.
func parseAgentChoice(input string, n int) (idx int, cancel bool, err error) {
	s := strings.TrimSpace(input)
	switch strings.ToLower(s) {
	case "q", "quit":
		return 0, true, nil
	case "":
		return 0, false, fmt.Errorf("please enter a number between 1 and %d", n)
	}
	v, perr := strconv.Atoi(s)
	if perr != nil {
		return 0, false, fmt.Errorf("invalid choice %q; enter a number between 1 and %d", s, n)
	}
	if v < 1 || v > n {
		return 0, false, fmt.Errorf("choice %d is out of range; enter a number between 1 and %d", v, n)
	}
	return v - 1, false, nil
}
