// Package adapter isolates everything that differs between target coding-agent
// CLIs (Codex today; Claude Code and others later) behind one interface. The
// rest of encave — install, run, publish, new — is target-agnostic and talks
// only to this interface.
//
// An adapter knows four things (see the design doc, §5.3):
//   - the name of the environment variable that points the target CLI at its
//     home directory (Codex: CODEX_HOME);
//   - which environment variable names the agent's config expects auth values
//     to arrive in (so the launcher can inject keyring values into them);
//   - how to assemble the launch command line (binary, args, config overrides);
//   - what to exclude when scaffolding from a user's home, and what belongs in
//     .gitignore.
package adapter

import "fmt"

// LaunchRequest is the target-agnostic description of "run this agent now".
// The adapter turns it into a concrete command line.
type LaunchRequest struct {
	// AgentDir is the resolved agent home directory.
	AgentDir string
	// UserArgs are the arguments the user passed after `--`, forwarded verbatim
	// to the target CLI (e.g. ["exec", "review this diff"]).
	UserArgs []string
	// Model, when non-empty, is an encave-level request to override the model at
	// launch. The adapter formats it in the target's override syntax.
	Model string
	// Sandbox, when non-empty, overrides the sandbox/approval mode at launch.
	Sandbox string
	// RawConfig holds raw, target-native config override strings passed through
	// from `--config`/`-c`. For Codex these are TOML `key=value` strings.
	RawConfig []string
}

// LaunchSpec is the resolved command an adapter wants encave to exec. The
// launcher is responsible for building the process environment (home variable +
// injected auth); an adapter may contribute additional non-secret env via Env.
type LaunchSpec struct {
	// Bin is the executable to look up on PATH and exec.
	Bin string
	// Args are the arguments to pass to Bin.
	Args []string
	// Env holds extra non-secret environment variables the adapter wants set
	// (rarely needed; auth and the home variable are handled by the launcher).
	Env map[string]string
}

// Adapter abstracts a single target coding-agent CLI.
type Adapter interface {
	// Name is the stable identifier persisted in agent metadata (e.g. "codex").
	Name() string

	// HomeEnvVar is the env var that points the target CLI at its home dir.
	HomeEnvVar() string

	// BaseHome returns the user's personal home directory for this target, used
	// as the source when scaffolding a new agent.
	BaseHome() (string, error)

	// Validate checks that a directory looks like a plausible home for this
	// target before launching.
	Validate(agentDir string) error

	// AuthEnvVars inspects the agent's committed config and returns the names of
	// the environment variables that should receive the injected auth secret.
	// An empty result means the agent declares no env-based auth.
	AuthEnvVars(agentDir string) ([]string, error)

	// BuildLaunch turns a LaunchRequest into a concrete command.
	BuildLaunch(req LaunchRequest) (LaunchSpec, error)

	// ScaffoldExcludes returns path/glob patterns to drop when copying a user's
	// home into a new agent (best-effort initial cleaning; the real gate is the
	// publish-time scan).
	ScaffoldExcludes() []string

	// PersonalSubdirs returns home subdirectories that hold the *user's own*
	// settings rather than the agent author's — e.g. Codex "rules"
	// (locally-approved commands). These are never packaged; instead the launcher
	// symlinks each one from the user's base home into the agent home at launch,
	// so every agent uses (and updates) the user's personal settings.
	PersonalSubdirs() []string

	// GitignoreLines returns recommended .gitignore entries for a published
	// agent of this target.
	GitignoreLines() []string
}

// Registry maps adapter names to their implementations.
var registry = map[string]Adapter{}

// register adds an adapter to the registry. Called from adapter init funcs.
func register(a Adapter) { registry[a.Name()] = a }

// Get returns the adapter for the given target name.
func Get(name string) (Adapter, error) {
	a, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown target %q (known: %v)", name, Names())
	}
	return a, nil
}

// Names lists the registered adapter names.
func Names() []string {
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	return out
}

// DefaultName is the target assumed when an agent declares none.
const DefaultName = "codex"
