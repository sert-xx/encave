# encave

> Distribute and run tuned coding-agent configurations as **isolated**,
> **reproducible**, self-contained agent homes — shared over GitHub, without
> touching the receiver's own environment.

The name **encave** comes from *en-* + *cave* / *enclave*: each agent is sealed
into its own isolated box (*encaved*) and run there. The canonical command is
`encave`; there is intentionally no official short alias.

The first target is the **Codex CLI**, but the core ideas — an isolated agent
home + launch-time credential injection + git-clone-and-tag distribution — are
target-agnostic and live behind an adapter interface, so other agent CLIs can be
added later.

## Why

People who use a coding agent seriously tune their entire home directory
(`~/.codex` / `CODEX_HOME`): not just model settings but the full
*orchestration* layer — sub-agent role definitions, profiles, review skills, MCP
servers. Sharing that with a teammate by copying config files clobbers their
setup; plugins can't carry the orchestration layer and merge into the receiver's
config instead of staying isolated.

encave distributes a **whole agent home as one reproducible unit**, installs it
in isolation, and launches it with secrets injected at runtime — so the
receiver's personal home is never modified.

## How it works

- **Reproducible**: agents are distributed via `git clone` + tag checkout. A tag
  reproduces the provider's configuration byte-for-byte.
- **Isolated**: each installed agent lives under `<root>/<owner>/<repo>` and is
  launched with its own home directory (Codex: `CODEX_HOME`). Your `~/.codex` is
  never touched.
- **Secrets stay out of git**: non-secret config (provider `base_url`, the
  *name* of the auth env var, `wire_api`, …) is committed. Real credentials live
  in the **OS keyring** and are injected into the launched child process's
  environment only — never written to the repo, logs, or stdout.

There is deliberately **no command that prints a stored credential**: that would
be a standing credential-dump oracle. Credentials only ever leave the keyring as
environment for one launched child process, for that process's lifetime.

## Installation

### Prerequisites

- **Go 1.25.11+** — 1.25.10+ ships standard-library security fixes encave's code
  paths rely on. With `GOTOOLCHAIN=auto` (the Go default), `go install` fetches a
  suitable toolchain automatically.
- **git** on your `PATH` (used by `install` and `publish`).
- The **target agent CLI** you want to run — currently the **Codex CLI**.
- A working **OS keyring**: macOS Keychain and Windows Credential Manager work
  out of the box; on **Linux** you need a running **Secret Service** such as
  `gnome-keyring` or KeePassXC's Secret Service.

### Install with `go install` (recommended)

```sh
# install a specific release — use the newest tag from the Releases page:
# https://github.com/sert-xx/encave/releases
go install github.com/sert-xx/encave@v0.7.0
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

## Quick start (using a shared agent)

```sh
# 1. Store your credential once (e.g. a proxy PAT). Re-run when it expires.
encave auth set --global

# 2. Install an agent, pinned to a released version.
encave install github.com/dai/review-agent --tag v1.0.0

# 3. Launch it — in its own isolated home, credential injected at launch.
encave dai/review-agent
#    (or run `encave run` and pick from the list)
```

Your personal `~/.codex` is never touched. `install` verifies the repo is an
encave-managed agent (it has a valid `.encave.toml`, written by `encave new`) and
refuses otherwise; pass `--no-verify` to override for a repo you trust.

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
encave new dai/review-agent        # copy ~/.codex into a new agent, filtering secrets/state
encave dai/review-agent            # try it locally before publishing
# ... tune agents/, skills/, config.toml ...
```

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

- `internal/adapter` — target-CLI abstraction; `codex.go` is the first adapter
  (home env var `CODEX_HOME`, reads `env_key` / `env_http_headers` to know which
  env vars to inject, builds `codex -c key=value` overrides).
- `internal/secrets` — keyring wrapper; the only value-returning call,
  `Resolve`, is used solely on the launch path.
- `internal/scan` — the fail-closed secret scanner used by `publish`.
- `internal/fsutil` — recursive copy with exclusions, used by `new`.
- `internal/gitutil` — thin wrappers over the `git` CLI.
- `internal/cli` — command dispatch (including the implicit `run`) and handlers.

## Status

v1: Codex CLI, single custom provider with a static/long-lived credential (e.g.
a 30-day PAT) injected at launch. Planned next: generic Codex auth (ChatGPT
login / API key), a Claude Code adapter, and per-agent multiple credentials.
