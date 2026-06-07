package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/sert-xx/encave/internal/adapter"
	"github.com/sert-xx/encave/internal/agentmeta"
	"github.com/sert-xx/encave/internal/paths"
	"github.com/sert-xx/encave/internal/secrets"
)

// multiFlag collects a repeatable string flag (used for -c overrides).
type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// cmdRun launches an installed agent in its isolated home with credentials
// injected from the keyring into the child process environment (design doc §4.4,
// §5.2). It is also the implicit default command.
//
// Argument shape: an optional agent reference comes first; encave flags follow;
// and everything after a literal `--` is forwarded verbatim to the target CLI.
// When no agent reference is given, encave shows the installed agents and lets
// the user pick one interactively.
//
//	encave run [<owner>/<repo>] [--model M] [--sandbox S] [-c k=v ...] [--dry-run] [-- <agent-args...>]
func cmdRun(args []string) int {
	// Split off the verbatim agent args after the first "--".
	pre := args
	var agentArgs []string
	for i, a := range args {
		if a == "--" {
			pre = args[:i]
			agentArgs = args[i+1:]
			break
		}
	}

	// The agent reference, when present, is the first token (before any flags).
	// If it is absent (no tokens, or the first token is a flag), encave will
	// offer an interactive picker over the installed agents.
	var refArg string
	var flagArgs []string
	if len(pre) > 0 && !strings.HasPrefix(pre[0], "-") {
		refArg = pre[0]
		flagArgs = pre[1:]
	} else {
		flagArgs = pre
	}

	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	model := fs.String("model", "", "override the model at launch")
	sandbox := fs.String("sandbox", "", "override the sandbox/approval mode at launch")
	var overrides multiFlag
	fs.Var(&overrides, "c", "raw target config override (repeatable; Codex TOML key=value)")
	dryRun := fs.Bool("dry-run", false, "print the resolved command and environment (secrets redacted) without launching")
	noAuth := fs.Bool("no-auth", false, "launch without injecting any credential")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: encave run [<owner>/<repo>|default] [--model M] [--sandbox S] [-c k=v] [--dry-run] [-- <agent-args...>]")
		fmt.Fprintln(os.Stderr, "  No target: choose interactively. 'default': your own home (e.g. ~/.codex), no isolation/injection.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		errf("unexpected arguments %v (the agent reference must come first; pass target args after `--`)", fs.Args())
		return 2
	}

	root, ok := mustRoot()
	if !ok {
		return 1
	}

	// Resolve what to launch: the reserved native-home keyword, an explicit
	// agent reference, or an interactive selection.
	var sel runSelection
	switch {
	case refArg == nativeRef:
		sel = runSelection{native: true}
	case refArg != "":
		r, err := parseAgentRef(refArg)
		if err != nil {
			errf("%v", err)
			return 2
		}
		sel = runSelection{ref: r}
	default:
		s, ok := pickLaunchTarget(root)
		if !ok {
			return 1
		}
		sel = s
	}

	if sel.native {
		return launchNative(agentArgs, *model, *sandbox, overrides, *dryRun)
	}
	return launchAgent(root, sel.ref, agentArgs, *model, *sandbox, overrides, *dryRun, *noAuth)
}

// nativeRef is the reserved `encave run` argument that launches the user's own
// default home for the default target (e.g. ~/.codex) directly — no isolation,
// no credential injection — so encave can serve as the single entry point.
const nativeRef = "default"

// launchNative runs the target CLI against the user's own default home with the
// environment passed through unchanged: CODEX_HOME is not overridden and no
// keyring credential is injected, so it behaves exactly like running the target
// CLI directly. encave is a pure passthrough launcher here.
func launchNative(agentArgs []string, model, sandbox string, overrides []string, dryRun bool) int {
	ad, err := adapter.Get(adapter.DefaultName)
	if err != nil {
		errf("%v", err)
		return 1
	}

	spec, err := ad.BuildLaunch(adapter.LaunchRequest{
		UserArgs:  agentArgs,
		Model:     model,
		Sandbox:   sandbox,
		RawConfig: overrides,
		// AgentDir is intentionally empty: use the target's own default home.
	})
	if err != nil {
		errf("building launch command: %v", err)
		return 1
	}

	if dryRun {
		home, _ := ad.BaseHome()
		fmt.Printf("agent:    (your default %s home)\n", ad.Name())
		fmt.Printf("target:   %s\n", ad.Name())
		fmt.Printf("home:     %s  (%s not overridden)\n", home, ad.HomeEnvVar())
		fmt.Println("auth:     (none injected — uses your own login/credentials)")
		fmt.Println("command:")
		fmt.Printf("  %s %s\n", spec.Bin, strings.Join(spec.Args, " "))
		return 0
	}

	binPath, err := exec.LookPath(spec.Bin)
	if err != nil {
		errf("target CLI %q not found on PATH: %v", spec.Bin, err)
		return 1
	}

	// Pass the current environment through unchanged.
	env := os.Environ()
	for k, v := range spec.Env {
		env = append(env, k+"="+v)
	}
	if err := execProcess(binPath, append([]string{spec.Bin}, spec.Args...), env); err != nil {
		errf("launching %s: %v", spec.Bin, err)
		return 1
	}
	return 0 // unreachable on success (process image replaced)
}

// launchAgent resolves the adapter and credentials for an installed agent and
// either prints the resolved command (dry run) or execs the target CLI.
func launchAgent(root string, ref AgentRef, agentArgs []string, model, sandbox string, overrides []string, dryRun, noAuth bool) int {
	agentDir := paths.AgentDir(root, ref.Owner, ref.Repo)
	if info, err := os.Stat(agentDir); err != nil || !info.IsDir() {
		errf("agent %s is not installed (looked in %s)", ref, agentDir)
		fmt.Fprintf(os.Stderr, "  install it first:  encave install github.com/%s\n", ref)
		return 1
	}

	// Select the adapter from agent metadata (default target if absent).
	targetName := adapter.DefaultName
	if m, merr := agentmeta.Load(agentDir); merr == nil && m != nil && m.Target != "" {
		targetName = m.Target
	}
	ad, err := adapter.Get(targetName)
	if err != nil {
		errf("%v", err)
		return 1
	}
	if err := ad.Validate(agentDir); err != nil {
		errf("%v", err)
		return 1
	}

	// Determine which env vars the agent's config expects auth in.
	authVars, err := ad.AuthEnvVars(agentDir)
	if err != nil {
		errf("inspecting auth configuration: %v", err)
		return 1
	}

	// Resolve the credential (agent-specific, then global) unless suppressed.
	var secret, secretScope string
	if !noAuth && len(authVars) > 0 {
		secret, secretScope, err = secrets.Resolve(ref.Scope())
		if err != nil {
			if errors.Is(err, secrets.ErrNotFound) {
				errf("no credential found for %s (or global)", ref)
				fmt.Fprintf(os.Stderr, "  store one:  encave auth set --agent %s\n", ref)
				fmt.Fprintf(os.Stderr, "         or:  encave auth set --global\n")
				fmt.Fprintf(os.Stderr, "  (use --no-auth to launch without a credential)\n")
				return 1
			}
			errf("reading keyring: %v", err)
			return 1
		}
	}

	// Build the child environment: inherit the user's env, then layer the home
	// variable and the injected auth values on top.
	env := envToMap(os.Environ())
	env[ad.HomeEnvVar()] = agentDir
	for _, name := range authVars {
		if secret != "" {
			env[name] = secret
		}
	}

	spec, err := ad.BuildLaunch(adapter.LaunchRequest{
		AgentDir:  agentDir,
		UserArgs:  agentArgs,
		Model:     model,
		Sandbox:   sandbox,
		RawConfig: overrides,
	})
	if err != nil {
		errf("building launch command: %v", err)
		return 1
	}
	for k, v := range spec.Env {
		env[k] = v
	}

	if dryRun {
		printDryRun(ref, ad, agentDir, spec, authVars, secretScope, env)
		return 0
	}

	binPath, err := exec.LookPath(spec.Bin)
	if err != nil {
		errf("target CLI %q not found on PATH: %v", spec.Bin, err)
		return 1
	}

	// Replace the current process with the target CLI. The child's lifetime is
	// the only window in which the credential lives in an environment.
	if err := execProcess(binPath, append([]string{spec.Bin}, spec.Args...), mapToEnv(env)); err != nil {
		errf("launching %s: %v", spec.Bin, err)
		return 1
	}
	return 0 // unreachable on success (process image replaced)
}

func envToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok {
			m[k] = v
		}
	}
	return m
}

func mapToEnv(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

// printDryRun shows exactly what would run, with auth values redacted so the
// command can be inspected safely.
func printDryRun(ref AgentRef, ad adapter.Adapter, agentDir string, spec adapter.LaunchSpec, authVars []string, secretScope string, env map[string]string) {
	fmt.Printf("agent:    %s\n", ref)
	fmt.Printf("target:   %s\n", ad.Name())
	fmt.Printf("home:     %s=%s\n", ad.HomeEnvVar(), agentDir)
	if len(authVars) == 0 {
		fmt.Println("auth:     (agent declares no env-based credential)")
	} else {
		src := secretScope
		if src == "" {
			src = "(none injected)"
		}
		fmt.Printf("auth:     %s  (from keyring scope: %s)\n", strings.Join(authVars, ", "), src)
	}
	authSet := map[string]bool{}
	for _, a := range authVars {
		authSet[a] = true
	}
	fmt.Println("command:")
	fmt.Printf("  %s %s\n", spec.Bin, strings.Join(spec.Args, " "))
	fmt.Println("environment (auth values redacted):")
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := env[k]
		if authSet[k] {
			v = "***redacted***"
		}
		// Only surface encave-relevant vars to keep output readable.
		if k == ad.HomeEnvVar() || authSet[k] {
			fmt.Printf("  %s=%s\n", k, v)
		}
	}
}
