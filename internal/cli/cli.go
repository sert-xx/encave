// Package cli implements encave's command-line surface: argument dispatch, the
// "run is the default subcommand" hot-path, and the individual command
// handlers. The design keeps a single canonical binary (`encave`) with no
// official short alias; typing is reduced by making `run` implicit (design doc
// §4.0).
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"

	"github.com/sert-xx/encave/internal/paths"
)

// Version is the encave version. It is normally left empty and resolved from the
// module's build info at runtime (so `go install github.com/sert-xx/encave@vX`
// reports vX). A release build can still pin it explicitly via
// -ldflags "-X github.com/sert-xx/encave/internal/cli.Version=vX.Y.Z".
var Version = ""

// version returns the best available version string: an explicit build-time
// override if set, otherwise the module version recorded in the binary's build
// info (e.g. "v0.1.0" or a pseudo-version), falling back to "(devel)" for local
// builds within the module.
func version() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" {
			return v
		}
	}
	return "(devel)"
}

// knownCommands lists the explicit subcommands. Anything else on the command
// line is treated as the agent reference for an implicit `run`.
var knownCommands = map[string]func([]string) int{
	"new":     cmdNew,
	"publish": cmdPublish,
	"install": cmdInstall,
	"run":     cmdRun,
	"auth":    cmdAuth,
	"list":    cmdList,
	"version": cmdVersion,
	"help":    cmdHelp,
}

// Main is the entry point invoked by package main. It returns a process exit
// code.
func Main(args []string) int {
	if len(args) == 0 {
		printUsage(os.Stdout)
		return 0
	}

	switch args[0] {
	case "-h", "--help":
		printUsage(os.Stdout)
		return 0
	case "-v", "--version":
		return cmdVersion(nil)
	}

	if fn, ok := knownCommands[args[0]]; ok {
		return fn(args[1:])
	}

	// Default subcommand: treat the whole argument list as `run ...`.
	return cmdRun(args)
}

func cmdVersion(_ []string) int {
	fmt.Printf("encave %s\n", version())
	return 0
}

func cmdHelp(_ []string) int {
	printUsage(os.Stdout)
	return 0
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `encave — distribute and run isolated, reproducible coding-agent homes.

USAGE:
  encave <command> [args]
  encave <owner>/<repo> [-- <agent-args...>]    # implicit "run"

COMMANDS:
  new <name>              Scaffold a draft agent (secrets filtered, README template generated)
  publish <name>          Scan (fail-closed), commit, tag, and (with a remote) push a draft
  install <github-url>    Clone an agent and check out a tag into <root>/<owner>/<repo>
  run [<owner>/<repo>]    Launch an agent ("default" = your own home; no ref = pick)
  auth set|status|clear   Manage credentials in the OS keyring (values never printed)
  list                    List installed agents and local drafts
  version                 Print the encave version
  help                    Show this help

Run "encave <command> -h" for command-specific options.

ENVIRONMENT:
  ENCAVE_ROOT             Override the root directory (default: ~/.encave)
`)
	return
}

// parseOnePositional parses a flag set for a command that takes exactly one
// positional argument, tolerating the positional appearing either before the
// flags (the natural `encave new <name> --flag` order) or after them. Go's flag
// package otherwise stops at the first non-flag token, which would leave
// trailing flags unparsed.
//
// It returns the positional value. If parsing fails or the argument count is
// wrong, it returns ("", false) after the flag set has printed its usage.
func parseOnePositional(fs *flag.FlagSet, args []string) (string, bool) {
	// Case 1: positional comes first (most common). Pull it off, parse the rest.
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		if err := fs.Parse(args[1:]); err != nil {
			return "", false
		}
		if fs.NArg() != 0 {
			fs.Usage()
			return "", false
		}
		return args[0], true
	}
	// Case 2: flags first, positional last (or no positional).
	if err := fs.Parse(args); err != nil {
		return "", false
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return "", false
	}
	return fs.Arg(0), true
}

// errf prints a formatted error to stderr prefixed with "encave:".
func errf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "encave: "+format+"\n", a...)
}

// mustRoot resolves the encave root or returns ("",false) after printing an
// error.
func mustRoot() (string, bool) {
	root, err := paths.Root()
	if err != nil {
		errf("%v", err)
		return "", false
	}
	return root, true
}

// AgentRef identifies an installed agent as owner/repo.
type AgentRef struct {
	Owner string
	Repo  string
}

// String renders the reference as "owner/repo".
func (a AgentRef) String() string { return a.Owner + "/" + a.Repo }

// Scope is the keyring scope key for this agent ("owner/repo").
func (a AgentRef) Scope() string { return a.Owner + "/" + a.Repo }

// parseAgentRef parses an "owner/repo" reference, rejecting anything that would
// escape the root (path separators beyond one slash, "..", etc.).
func parseAgentRef(s string) (AgentRef, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "/")
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return AgentRef{}, fmt.Errorf("expected agent reference as <owner>/<repo>, got %q", s)
	}
	for _, p := range parts {
		if p == "." || p == ".." || strings.ContainsAny(p, `\`) {
			return AgentRef{}, fmt.Errorf("invalid agent reference %q", s)
		}
	}
	return AgentRef{Owner: parts[0], Repo: parts[1]}, nil
}
