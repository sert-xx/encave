package cli

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sert-xx/encave/internal/adapter"
	"github.com/sert-xx/encave/internal/agentmeta"
	"github.com/sert-xx/encave/internal/paths"
	"github.com/sert-xx/encave/internal/secrets"
	"golang.org/x/term"
)

// cmdAuth manages credentials in encave's keyring (design doc §4.5). It exposes
// set / status / clear. There is deliberately no subcommand that prints a stored
// value (§3.4): credentials only ever leave the keyring as injected environment
// for a launched child process.
func cmdAuth(args []string) int {
	if len(args) == 0 {
		errf("usage: encave auth <set|status|clear> [--agent <owner/repo>|--global]")
		return 2
	}
	sub := args[0]
	rest := args[1:]

	switch sub {
	case "set":
		return authSet(rest)
	case "status":
		return authStatus(rest)
	case "clear":
		return authClear(rest)
	default:
		errf("unknown auth subcommand %q (want set|status|clear)", sub)
		return 2
	}
}

// scopeFlags adds the mutually-exclusive --agent/--global selectors to a flag
// set and returns a resolver for the chosen scope (defaulting to global) plus a
// pointer to the raw --agent value (for target-specific warnings).
func scopeFlags(fs *flag.FlagSet) (func() (string, error), *string) {
	agent := fs.String("agent", "", "scope the credential to a single agent (<owner>/<repo>)")
	global := fs.Bool("global", false, "scope the credential to all agents (default)")
	return func() (string, error) {
		if *agent != "" && *global {
			return "", fmt.Errorf("--agent and --global are mutually exclusive")
		}
		if *agent != "" {
			ref, err := parseAgentRef(*agent)
			if err != nil {
				return "", err
			}
			return ref.Scope(), nil
		}
		return secrets.GlobalScope, nil
	}, agent
}

// warnIfUnmanagedTarget prints a note when the credential is scoped to an
// installed agent whose target does not use encave-injected credentials (e.g.
// Claude Code), so the user isn't left thinking a stored-but-never-injected token
// authenticates the agent. It is best-effort and silent for global scope,
// not-yet-installed agents, and managed targets.
func warnIfUnmanagedTarget(agentFlag string) {
	target, warn := unmanagedAuthTarget(agentFlag)
	if !warn {
		return
	}
	fmt.Fprintf(os.Stderr, "encave: note: %s targets %q, which manages its own login; encave will NOT inject this credential at launch.\n", agentFlag, target)
	fmt.Fprintln(os.Stderr, "  the stored value is unused for this target — see the agent's README for how it authenticates.")
}

// unmanagedAuthTarget reports the target of the installed agent named by the
// --agent flag and whether a "credential won't be injected" warning is
// warranted. It returns warn=false for an empty flag (global scope), an invalid
// or not-yet-installed agent, or a target whose auth encave manages.
func unmanagedAuthTarget(agentFlag string) (target string, warn bool) {
	if agentFlag == "" {
		return "", false
	}
	ref, err := parseAgentRef(agentFlag)
	if err != nil {
		return "", false
	}
	root, err := paths.Root()
	if err != nil {
		return "", false
	}
	dir := paths.AgentDir(root, ref.Owner, ref.Repo)
	if info, serr := os.Stat(dir); serr != nil || !info.IsDir() {
		return "", false // not installed; can't know its target yet
	}
	target = agentmeta.DefaultTargetOr(dir)
	ad, err := adapter.Get(target)
	if err != nil || ad.ManagedAuth() {
		return target, false
	}
	return target, true
}

func authSet(args []string) int {
	fs := flag.NewFlagSet("auth set", flag.ContinueOnError)
	resolve, agent := scopeFlags(fs)
	stdinFlag := fs.Bool("stdin", false, "read the secret from stdin instead of prompting (no trailing newline kept)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: encave auth set [--agent <owner/repo>|--global] [--stdin]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	scope, err := resolve()
	if err != nil {
		errf("%v", err)
		return 2
	}

	secret, err := readSecret(*stdinFlag)
	if err != nil {
		errf("%v", err)
		return 1
	}
	if secret == "" {
		errf("empty credential; nothing stored")
		return 1
	}

	if err := secrets.Set(scope, secret); err != nil {
		errf("storing credential in keyring: %v", err)
		return 1
	}
	fmt.Printf("Stored credential for scope %q in the OS keyring.\n", scope)
	warnIfUnmanagedTarget(*agent)
	return 0
}

func authStatus(args []string) int {
	fs := flag.NewFlagSet("auth status", flag.ContinueOnError)
	resolve, _ := scopeFlags(fs)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: encave auth status [--agent <owner/repo>|--global]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	scope, err := resolve()
	if err != nil {
		errf("%v", err)
		return 2
	}
	has, err := secrets.Has(scope)
	if err != nil {
		errf("querying keyring: %v", err)
		return 1
	}
	if has {
		fmt.Printf("scope %q: credential is set\n", scope)
	} else {
		fmt.Printf("scope %q: not set\n", scope)
	}
	return 0
}

func authClear(args []string) int {
	fs := flag.NewFlagSet("auth clear", flag.ContinueOnError)
	resolve, _ := scopeFlags(fs)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: encave auth clear [--agent <owner/repo>|--global]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	scope, err := resolve()
	if err != nil {
		errf("%v", err)
		return 2
	}
	if err := secrets.Delete(scope); err != nil {
		errf("deleting credential: %v", err)
		return 1
	}
	fmt.Printf("Cleared credential for scope %q.\n", scope)
	return 0
}

// readSecret obtains the credential without echoing it to the terminal and
// without ever accepting it as a command-line argument (which would leak via
// the process table and shell history).
func readSecret(fromStdin bool) (string, error) {
	if fromStdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading secret from stdin: %w", err)
		}
		return strings.TrimRight(string(data), "\r\n"), nil
	}

	// If stdin isn't a terminal, fall back to reading a single line so the
	// command remains scriptable, but warn that input may be visible.
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(os.Stderr, "encave: stdin is not a TTY; reading one line (input may be visible). Prefer --stdin from a secure source.")
		r := bufio.NewReader(os.Stdin)
		line, err := r.ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("reading secret: %w", err)
		}
		return strings.TrimRight(line, "\r\n"), nil
	}

	fmt.Fprint(os.Stderr, "Enter credential (input hidden): ")
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("reading secret: %w", err)
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}
