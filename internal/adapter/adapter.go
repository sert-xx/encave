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

	// AuthEnvVars inspects a target config and returns the names of the
	// environment variables that should receive the injected auth secret (read
	// from a model provider's env_key / env_http_headers). An empty result means
	// no env-based auth. It takes the resolved/effective config bytes, since the
	// provider config may come from the user's home at launch.
	AuthEnvVars(configData []byte) ([]string, error)

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

	// ConfigLayout reports the committed "base" config filename and the generated
	// "effective" config filename the target CLI actually reads. Both empty if the
	// adapter does not split config this way (then `new`/`run` skip config
	// transformation for it).
	ConfigLayout() (base string, effective string)

	// BuildBaseConfig filters a full target config (read from the user's home)
	// down to the keys an agent should own and ship. Used by `new`. Returns
	// (nil, nil) for empty input.
	BuildBaseConfig(fullConfig []byte) ([]byte, error)

	// BuildEffectiveConfig merges the committed base config over the user's home
	// config so the user's environment-specific settings apply while the agent's
	// keys win. Used by `run`; homeConfig may be empty.
	BuildEffectiveConfig(baseConfig, homeConfig []byte) ([]byte, error)

	// MCPServers parses a full target config and returns the MCP servers it
	// declares. These are not packaged (reusing someone else's MCP server config
	// is risky), so `new` lists them in the generated README as setup
	// requirements for the user's own home config. Returns nil if none / N/A.
	MCPServers(configData []byte) ([]MCPServerInfo, error)

	// ModelProviders parses a full target config and returns the model providers
	// it declares. Like MCP servers, providers are not packaged (they are
	// environment-specific — base URL, auth env var, wire protocol); `new` lists
	// them in the README as setup requirements for the user's own home config.
	ModelProviders(configData []byte) ([]ProviderInfo, error)
}

// ProviderInfo describes a model provider an agent's source config referenced,
// for documenting setup requirements in the generated README.
type ProviderInfo struct {
	Name    string // provider id/name
	BaseURL string // endpoint base URL
	WireAPI string // wire protocol (e.g. "responses", "chat")
	EnvKey  string // env var the provider reads its bearer token from
}

// MCPServerInfo describes an MCP server an agent's source config referenced, for
// documenting install requirements in the generated README.
type MCPServerInfo struct {
	Name    string   // server id/name
	Command string   // launch command (local servers)
	Args    []string // launch command arguments
	URL     string   // endpoint (remote/HTTP servers)
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
