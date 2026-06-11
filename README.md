# encave

*Read this in [日本語](README.ja.md).*

> Distribute and run tuned coding-agent configurations as **isolated**,
> **reproducible**, self-contained agent homes — shared over GitHub, without
> touching the receiver's own environment.

The name **encave** comes from *en-* + *cave* / *enclave*: each agent is sealed
into its own isolated box (*encaved*) and run there. The canonical command is
`encave`; there is intentionally no official short alias.

The supported targets are the **Codex CLI** and **Claude Code**. The core ideas —
an isolated agent home + git-clone-and-tag distribution, with credentials kept
out of the repo — are target-agnostic and live behind an adapter interface, so
more agent CLIs can be added later. How the credential is supplied differs per
target (encave-injected for Codex; the target's own login for Claude Code) — see
[Targets](#targets).

## Why

People who use a coding agent seriously tune their entire home directory
(`~/.codex` / `CODEX_HOME`): not just model settings but the full
*orchestration* layer — sub-agent role definitions, profiles, review skills, MCP
servers. Sharing that with a teammate by copying config files clobbers their
setup; plugins can't carry the orchestration layer and merge into the receiver's
config instead of staying isolated.

encave distributes a **whole agent home as one reproducible unit**, installs it
in isolation, and launches it without ever modifying the receiver's personal
home — keeping credentials out of the shared repo.

## How it works

- **Reproducible**: agents are distributed via `git clone` + tag checkout. A tag
  reproduces the provider's configuration byte-for-byte.
- **Isolated**: each installed agent lives under `<root>/<owner>/<repo>` and is
  launched with its own home directory (Codex: `CODEX_HOME`; Claude Code:
  `CLAUDE_CONFIG_DIR`). Your `~/.codex` / `~/.claude` is never touched.
- **Secrets stay out of git**: only non-secret config is committed. For the Codex
  target, real credentials live in the **OS keyring** and are injected into the
  launched child process's environment only — never written to the repo, logs, or
  stdout. The Claude Code target keeps using Claude Code's own login and encave
  injects nothing (see [Targets](#targets)).

For the Codex target there is deliberately **no command that prints a stored
credential**: that would be a standing credential-dump oracle. Credentials only
ever leave the keyring as environment for one launched child process, for that
process's lifetime.

## Installation

### Prerequisites

- **Go 1.25.11+** — 1.25.10+ ships standard-library security fixes encave's code
  paths rely on. With `GOTOOLCHAIN=auto` (the Go default), `go install` fetches a
  suitable toolchain automatically.
- **git** on your `PATH` (used by `install` and `publish`).
- The **target agent CLI** you want to run — the **Codex CLI** or **Claude Code**.
- For the **Codex** target, a working **OS keyring** (encave injects your
  credential from it): macOS Keychain and Windows Credential Manager work out of
  the box; on **Linux** you need a running **Secret Service** such as
  `gnome-keyring` or KeePassXC's Secret Service. The **Claude Code** target does
  not use the keyring — see [Targets](#targets).

### Install with `go install` (recommended)

```sh
# install a specific release — use the newest tag from the Releases page:
# https://github.com/sert-xx/encave/releases
go install github.com/sert-xx/encave@v0.9.2
```

> Prefer the latest release tag over `@latest`: the module proxy's `@latest`
> often lags behind the newest tag.

`go install` places the `encave` binary in `$(go env GOBIN)`, falling back to
`$(go env GOPATH)/bin`. Make sure that directory is on your `PATH`:

```sh
export PATH="$(go env GOPATH)/bin:$PATH"   # add this to your shell profile
```

### Build from source

```sh
git clone https://github.com/sert-xx/encave && cd encave
make build      # produces ./encave
make install    # go install into $GOBIN
make test       # run the test suite
```

### Verify

```sh
encave version
```

### Staying up to date

encave keeps itself and your agents current with light-touch prompts — never a
silent background install:

- **encave itself:** when you run a command and a newer encave release exists,
  encave offers to install that exact version with `go install` (the resolved
  tag, never `@latest`). The freshly installed binary applies to your next
  invocation.
- **agents:** see [Using a shared agent](#using-a-shared-agent) — `encave run`
  offers to update an agent when its origin has a newer release tag.

Both checks read **git release tags** directly (encave's own from its repository,
an agent's from its `origin`), so a freshly pushed tag is detected promptly —
unlike the module proxy's `@latest`, which lags. The network is consulted at most
about once an hour per target, but once an update is known it is re-offered on
every run until you install it or decline that specific version (a newer version
re-prompts). Checks only run when attached to a terminal, the self-check needs a
release version (not a `make build` dev binary), they never block, and any
network error is silently skipped. Set `ENCAVE_NO_UPDATE_CHECK=1` to turn all of
this off.

## Quick start (using a shared agent)

```sh
# 1. (Codex agents) Store your credential once (e.g. a proxy PAT); re-run when it
#    expires. Claude Code agents skip this — they use Claude Code's own login.
encave auth set --global

# 2. Install an agent, pinned to a released version.
encave install github.com/dai/review-agent --tag v1.0.0

# 3. Launch it — in its own isolated home.
encave dai/review-agent
#    (or run `encave run` and pick from the list)
```

Your personal `~/.codex` / `~/.claude` is never touched. `install` verifies the
repo is an encave-managed agent (it has a valid `.encave.toml`, written by
`encave new`) and refuses otherwise; pass `--no-verify` to override for a repo you
trust. How each target gets its credential differs — see [Targets](#targets).

## Commands

```
encave new <owner>/<repo>      Scaffold a draft agent (secrets filtered; README template generated)
encave publish [<owner>/<repo>]  Scan (fail-closed), commit, tag, and (with a remote) push an agent
encave install <github-url>    Clone+checkout an agent (verifies it's encave-managed)
encave update [<owner>/<repo>] Update an agent to a tag (default: latest); --all for every agent
encave run [<owner>/<repo>]    Launch an agent ("default" = your own home; omit to pick)
encave auth set|status|clear   Manage credentials in the OS keyring (values never printed)
encave list                    List installed agents and local drafts
encave remove [<owner>/<repo>] Delete an installed agent's directory (alias: rm)
encave version | help
```

`run` is the **default command**: `encave dai/review-agent` is the same as
`encave run dai/review-agent`. Anything after `--` is forwarded verbatim to the
target CLI.

## Usage

### Using a shared agent

See what's installed (and any local drafts):

```sh
encave list
```

Launch by reference, or omit it to pick from an interactive menu (navigate with
↑/↓, Enter to choose, `q` to cancel):

```sh
encave dai/review-agent            # run is the default command
encave run                         # arrow-key menu of installed agents (+ your own home)
```

Anything after `--` is forwarded verbatim to the target CLI. Preview the exact
command (credentials redacted) without launching:

```sh
encave dai/review-agent --dry-run -- exec "review this diff"
```

Keep agents current with `update` (fetches the agent's origin and checks out the
tag):

```sh
encave update dai/review-agent              # latest release tag
encave update dai/review-agent --tag v1.2.0 # a specific tag
encave update --all                         # every installed agent to its latest
```

You usually don't have to remember to: when you `encave run` an agent and a newer
release tag exists on its origin, encave offers to update it first (accept and it
runs `update` for you, decline and it launches the version you have).

Remove an agent you no longer want (deletes its directory; confirms first, or
pass `--force`):

```sh
encave remove dai/review-agent   # alias: encave rm
```

### Launching your own (non-encave) home

So encave can be your single entry point, `default` launches your own default
home for the target (e.g. `~/.codex`) directly — no isolation, no credential
injection, `CODEX_HOME` left untouched. It behaves exactly like running the
target CLI yourself, and also appears as a choice in `encave run`'s picker:

```sh
encave default                     # same as `encave run default`
encave run default -- exec "quick one-off in my own setup"
```

### Managing credentials

Credentials live in the OS keyring, resolved agent-specific entry first, then the
global one, and injected only into the launched (isolated) process:

```sh
encave auth set --agent dai/review-agent   # scope to one agent
encave auth set --global                   # or share across agents
encave auth status --global                # shows "set" / "not set" only — never the value
encave auth clear --global
```

### Personal settings (`rules`)

Some home subdirectories hold *your own* settings rather than the agent author's
— for Codex, `rules` (your locally-approved commands). encave never packages
these: `new` excludes them and `publish` gitignores them. Instead it symlinks the
agent's `rules` to your base home's `~/.codex/rules`, so your personal approvals
apply to every agent and any new approvals accumulate in one place. (An agent
that already ships a real `rules/` directory is left untouched.)

The symlink is (re)created by `new`, `install`, and `run` — so it's effective
immediately after you scaffold or install an agent, and visible while you edit
(no accidentally copying your rules in). It is **not** committed: a symlink can't
be portable across machines (the OS does not expand `~` or `$HOME` in link
targets, and an absolute path would point at the author's home), so encave
recreates it per-machine with your real home path instead.

### Creating and sharing an agent

An agent's name **is** its GitHub identity (`<owner>/<repo>`), so `new` and
`publish` take it in that form. `new` scaffolds straight into
`<root>/<owner>/<repo>` — the same place `install` uses — so there is no separate
"drafts" area, and you can run and iterate on your own agent before publishing it.

```sh
encave new dai/review-agent        # prompts for the target, then copies that home, filtering secrets/state
encave dai/review-agent            # try it locally before publishing
# ... tune agents/, skills/, the base config ...
```

When you omit `--target`, `new` prompts you to choose the target CLI (Codex or
Claude Code) on a terminal; pass `--target claude-code` (or `--target codex`) to
skip the prompt, e.g. in scripts. Off a terminal it falls back to the default
target (`codex`). The source home copied follows the chosen target
(`~/.codex` / `~/.claude`), overridable with `--from`.

`encave new` also generates a `README.md` template in the agent (unless you pass
`--no-readme`), replacing any README copied from your base home — that generic
`~/.codex` README rarely fits a specific agent. It documents the install/auth/run
flow for consumers, filled in with the agent's `<owner>/<repo>` and the credential
env var(s) discovered from its config.

`new` then runs `git init` and makes an initial commit containing **only** the
README (skipped if git isn't installed). The rest of the agent is committed later
by `publish`, after the secret scan — so nothing unscanned lands in a commit.

```sh
# Create the empty repo on GitHub first, then publish with a remote:
encave publish dai/review-agent --tag v1.0.0 --remote git@github.com:dai/review-agent.git
```

On a terminal you can just run `encave publish` and it prompts for anything
missing — which agent (chosen from a list), the release tag, and the remote
(defaulting to `git@github.com:<owner>/<repo>.git`). Off a terminal these must be
passed as flags.

`encave publish` runs a fail-closed secret scan, commits, and tags. Then:

- **With a remote** (`--remote`, or an existing `origin`): it asks
  `Push to <url> now? [y/N]` and pushes the branch and tag on confirmation.
  `-y`/`--yes` skips the prompt; `--no-push` stops after tagging. Non-interactive
  runs never push unless `--yes` is given.
- **Without a remote**: it stops without pushing and explains how to set one
  (the commit and tag are already created locally).

After a tag is pushed, if the [GitHub CLI](https://cli.github.com/) (`gh`) is
installed and can access the repo, encave offers to create a **GitHub release**
for that tag (prompted; `--yes` creates it without asking). If `gh` is missing or
the remote isn't a GitHub repo it can reach, this step is silently skipped.

## Targets

A *target* is the agent CLI an agent home is built for. Choose it with
`encave new <owner>/<repo> --target <name>`; it is recorded in `.encave.toml` so
`run` selects the right behavior automatically. The two differ mainly in **where
the credential comes from**:

| | **Codex** (`--target codex`, default) | **Claude Code** (`--target claude-code`) |
|---|---|---|
| Home variable | `CODEX_HOME` | `CLAUDE_CONFIG_DIR` |
| Config file | `config.toml` (TOML) | `settings.json` (JSON) |
| Packaged base | `config_base.toml` | `settings_base.json` |
| Credential | **encave-managed**: stored in the OS keyring, injected at launch | **not managed by encave** — uses Claude Code's own login (see below) |

**Codex** needs encave to inject a credential because Codex ties its stored login
to `CODEX_HOME`; isolating the home would otherwise lose it. So you
`encave auth set` a token once and encave injects it at launch (`encave auth …`).

**Claude Code** stores its credential outside the config directory on macOS (the
**Keychain**, which is global), so an isolated `CLAUDE_CONFIG_DIR` stays logged in
with your normal `claude /login` — encave injects nothing. On **Linux/Windows**
the credential file lives *inside* the config directory, so the isolated home
starts logged out: just authenticate once inside it (run `claude` and `/login`,
or set `CLAUDE_CODE_OAUTH_TOKEN` from `claude setup-token`). Either way encave
never stores or prints a Claude credential. The agent's connection endpoint
(`ANTHROPIC_BASE_URL`) is environment-specific and comes from your own
environment, not the packaged agent.

## Security model

1. **Secrets never enter the repo** — keyring + `.gitignore` + a fail-closed
   publish scan (known credential filenames, token-shaped strings, and
   high-entropy values). `encave publish` aborts if anything is detected.
2. **No standing dump oracle** — no command prints a keyring value; injection is
   confined to the launched child's environment.
3. **Credentials are ephemeral** — a token exists in an environment only for the
   lifetime of the launched process.
4. **Residual risk is acknowledged** — same-user code can read a running child's
   environment; this is the irreducible floor for handing a secret to a local
   child process, and is far smaller than a standing oracle.
5. **Enterprise policy wins** — MDM/managed policy can override launch-time
   config; encave does not assume its overrides always take effect.
6. **Claude Code caveat** — the effective `settings.json` encave generates in a
   Claude agent home merges your own `~/.claude/settings.json`, including any
   `env` block. If you keep credential *values* there, they are written to that
   file (gitignored, mode 0600, never committed by `publish`). Prefer keeping
   credentials in your shell or Keychain rather than `settings.json` `env`.

## Layout

```
<root>/                          # ~/.encave (override with ENCAVE_ROOT)
└── <owner>/<repo>/              # one agent = one isolated agent home
    │                            #   (authored via `new` or fetched via `install`)
    ├── config_base.toml         # agent-owned config (whitelisted keys) — committed
    ├── config.toml              # generated at launch (base ⊕ your ~/.codex) — gitignored
    ├── .encave.toml             # non-secret agent metadata (target adapter)
    ├── agents/ prompts/ skills/ # orchestration, prompts, skills — vendored, committed
    ├── AGENTS.md                # author instructions — committed
    ├── rules -> ~/.codex/rules  # personal settings: symlinked at launch, never packaged
    └── (ignored: auth.json, history.jsonl, sessions/, *.sqlite state/log DBs,
         logs, caches, version.json — Codex's machine-generated state)
```

Only the author's tuned configuration is packaged. Codex's machine-generated
state — credentials, history, session transcripts, the `state_*.sqlite` /
`logs_*.sqlite` databases (and their WAL/SHM sidecars), logs, caches and
`version.json` — is excluded by `new` and gitignored by `publish`.

### config: agent-owned vs. environment

Codex's `config.toml` mixes the agent author's settings (model, providers,
sandbox/permissions, agents, …) with the executor's own, environment-specific
state — most importantly `[projects]` trust levels, which record absolute local
paths Codex auto-appends as you approve projects, plus UI, notifications,
telemetry and local paths. To keep these on the right side:

- `new` writes **`config_base.toml`** containing only a **whitelist of
  agent-owned top-level keys**; everything else is left to the user's home.
- At launch, `run` merges `config_base.toml` over the user's own
  `~/.codex/config.toml` into the **generated `config.toml`** that Codex reads:
  the agent's keys win, while project trust, UI and other personal settings come
  from the user. Because `config.toml` is gitignored, Codex appending new trust
  at runtime never shows up as a diff.

This means each executor's existing project-trust decisions apply automatically,
and no author's local paths/trust ship inside an agent.

**MCP servers and the model provider are not packaged.** `mcp_servers` and
`model_provider`/`model_providers` (plus `sandbox_workspace_write`, which carries
local paths) are intentionally left out of the whitelist — reusing another
person's MCP config or provider wiring (internal base URLs, auth env vars) is
risky and environment-specific. They come from the user's own
`~/.codex/config.toml` at launch, and `new` lists the author's MCP servers and
model providers in the generated README as setup requirements.

**Auth wiring is owned by encave.** When generating the effective `config.toml`,
encave drops Codex's own credential store (`cli_auth_credentials_store`) and
forces every model provider's `env_key` to a fixed variable it injects the
keyring token into. So a launch is authenticated even when the user's provider
config declares no `env_key`, and the agent never depends on Codex's stored
login — the token always comes from `encave auth set`.

Credentials live only in the OS keyring under the `encave` service.

## Architecture

- `internal/adapter` — target-CLI abstraction. `codex.go` (home env var
  `CODEX_HOME`, reads `env_key` / `env_http_headers` to know which env vars to
  inject, builds `codex -c key=value` overrides) and `claude.go` (home env var
  `CLAUDE_CONFIG_DIR`, JSON settings, injects no credential — see Targets).
- `internal/secrets` — keyring wrapper; the only value-returning call,
  `Resolve`, is used solely on the launch path.
- `internal/scan` — the fail-closed secret scanner used by `publish`.
- `internal/fsutil` — recursive copy with exclusions, used by `new`.
- `internal/gitutil` — thin wrappers over the `git` CLI (including the
  `ls-remote`/`fetch` tag lookups behind the update checks).
- `internal/semver` — parse/compare `vX.Y.Z` versions (release tags and encave's
  own version).
- `internal/cli` — command dispatch (including the implicit `run`) and handlers.

## Status

Targets: **Codex CLI** (single custom provider with a static/long-lived
credential, e.g. a 30-day PAT, injected at launch) and **Claude Code** (isolated
`CLAUDE_CONFIG_DIR`, using Claude Code's own login — encave injects nothing).
Planned next: generic Codex auth (ChatGPT login / API key), per-agent multiple
credentials, and consumer-side trust (provenance, update diffs).

## Contributing & security

Bug reports and feature requests are welcome via
[issues](https://github.com/sert-xx/encave/issues). For a security
vulnerability, please use private reporting instead of a public issue — see
[SECURITY.md](SECURITY.md).

## License

Released under the [MIT License](LICENSE).
