package cli

import (
	"fmt"
	"strings"

	"github.com/sert-xx/encave/internal/adapter"
)

// renderAgentReadme produces a README.md tailored to a freshly scaffolded agent.
// It documents the encave consumer workflow (install / auth / run) using the
// agent's GitHub identity and discovered auth environment variables, lists the
// MCP servers the agent expects (which are not bundled), and leaves clearly
// marked TODOs for the maintainer to describe what the agent does.
//
// owner/repo are the agent's GitHub identity; target is the adapter name (e.g.
// "codex"); authVars are the credential env var names; providers and mcps are the
// model providers and MCP servers the source config referenced (not packaged —
// listed as setup requirements). Any of these may be empty.
func renderAgentReadme(owner, repo, target string, authVars []string, providers []adapter.ProviderInfo, mcps []adapter.MCPServerInfo) string {
	ref := owner + "/" + repo
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", repo)
	b.WriteString("> **TODO:** Describe what this agent is tuned for in one or two sentences\n")
	b.WriteString("> (e.g. \"A thorough code-review agent with security and performance skills\").\n\n")

	b.WriteString("This is an [**encave**](https://github.com/sert-xx/encave) agent: a")
	b.WriteString(" self-contained, isolated agent home (configuration + orchestration +\n")
	fmt.Fprintf(&b, "skills) for the **%s** CLI, distributed via GitHub. encave runs it in its own\n", target)
	b.WriteString("home directory with credentials injected at launch, so it never touches your\n")
	b.WriteString("personal setup and no secrets are stored in this repository.\n\n")

	// Requirements
	b.WriteString("## Requirements\n\n")
	b.WriteString("- [encave](https://github.com/sert-xx/encave):\n")
	b.WriteString("  `go install github.com/sert-xx/encave@latest`\n")
	fmt.Fprintf(&b, "- The target CLI: **%s**\n", target)
	b.WriteString("- On Linux, a running Secret Service (e.g. gnome-keyring) for the keyring\n\n")

	// Install
	b.WriteString("## Install\n\n")
	b.WriteString("```sh\n")
	fmt.Fprintf(&b, "encave install github.com/%s --tag <tag>\n", ref)
	b.WriteString("```\n\n")
	b.WriteString("Pin a released `<tag>` for a byte-for-byte reproducible install.\n\n")

	// Credentials
	b.WriteString("## Credentials\n\n")
	if len(authVars) > 0 {
		b.WriteString("This agent reads its credential from the following environment variable")
		if len(authVars) > 1 {
			b.WriteString("s")
		}
		b.WriteString(", injected at launch from your OS keyring (never committed here):\n\n")
		for _, v := range authVars {
			fmt.Fprintf(&b, "- `%s`\n", v)
		}
		b.WriteString("\nStore the credential once (re-run when it expires, e.g. a rotating PAT):\n\n")
		b.WriteString("```sh\n")
		b.WriteString("encave auth set --global              # shared across all agents\n")
		fmt.Fprintf(&b, "encave auth set --agent %s   # or scope it to just this agent\n", ref)
		b.WriteString("```\n\n")
		b.WriteString("> **TODO:** Document where to obtain this credential (e.g. which proxy/PAT)\n")
		b.WriteString("> and any scope or expiry it needs.\n\n")
	} else {
		b.WriteString("This agent does not declare an environment-based credential in its config.\n")
		b.WriteString("If it needs one, document it here and store it with `encave auth set`.\n\n")
	}

	// Required model provider (not bundled — provider wiring is environment-specific)
	if len(providers) > 0 {
		b.WriteString("## Model provider\n\n")
		b.WriteString("This agent's model provider is **not** bundled (the base URL, auth env var,\n")
		b.WriteString("and wire protocol are environment-specific). Configure a compatible provider\n")
		b.WriteString("in your own `~/.codex/config.toml`. The author built it against:\n\n")
		for _, p := range providers {
			fmt.Fprintf(&b, "- **%s**", p.Name)
			var bits []string
			if p.BaseURL != "" {
				bits = append(bits, "base_url `"+p.BaseURL+"`")
			}
			if p.WireAPI != "" {
				bits = append(bits, "wire_api `"+p.WireAPI+"`")
			}
			if p.EnvKey != "" {
				bits = append(bits, "token env var `"+p.EnvKey+"`")
			}
			if len(bits) > 0 {
				b.WriteString(" — " + strings.Join(bits, ", "))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n> **TODO:** Document how to obtain access to this provider for your environment.\n\n")
	}

	// Required MCP servers (not bundled — must be configured in the user's home)
	if len(mcps) > 0 {
		b.WriteString("## Required MCP servers\n\n")
		b.WriteString("This agent expects the following MCP servers. They are **not** bundled (reusing\n")
		b.WriteString("another person's MCP config is risky), so install and configure them in your\n")
		b.WriteString("own `~/.codex/config.toml` before use:\n\n")
		for _, m := range mcps {
			switch {
			case m.URL != "":
				fmt.Fprintf(&b, "- **%s** (remote) — `%s`\n", m.Name, m.URL)
			case m.Command != "":
				cmd := m.Command
				if len(m.Args) > 0 {
					cmd += " " + strings.Join(m.Args, " ")
				}
				fmt.Fprintf(&b, "- **%s** — `%s`\n", m.Name, cmd)
			default:
				fmt.Fprintf(&b, "- **%s**\n", m.Name)
			}
		}
		b.WriteString("\n> **TODO:** Add install/setup notes for each server (package, auth, env).\n\n")
	}

	// Run
	b.WriteString("## Run\n\n")
	b.WriteString("```sh\n")
	fmt.Fprintf(&b, "encave %s                  # launch (run is the default command)\n", ref)
	b.WriteString("encave run                           # or pick interactively from installed agents\n")
	b.WriteString("```\n\n")
	b.WriteString("Forward arguments to the underlying CLI after `--`, and preview the exact\n")
	b.WriteString("command (with credentials redacted) without launching via `--dry-run`:\n\n")
	b.WriteString("```sh\n")
	fmt.Fprintf(&b, "encave %s --dry-run -- exec \"do the task\"\n", ref)
	b.WriteString("```\n\n")

	// Maintainer section
	b.WriteString("## For maintainers\n\n")
	b.WriteString("This agent is built and published with encave:\n\n")
	b.WriteString("```sh\n")
	fmt.Fprintf(&b, "encave new %s                  # scaffold from your base home (secrets filtered)\n", ref)
	b.WriteString("# ...tune agents/, skills/, config.toml...\n")
	fmt.Fprintf(&b, "encave publish %s --tag <tag> --remote git@github.com:%s.git\n", ref, ref)
	b.WriteString("```\n\n")
	b.WriteString("`encave publish` runs a fail-closed secret scan before committing: credentials\n")
	b.WriteString("must live in the keyring, never in this repository.\n\n")

	b.WriteString("---\n\n")
	b.WriteString("<sub>Generated by `encave new`. Replace the TODOs above to describe this agent.</sub>\n")

	return b.String()
}
