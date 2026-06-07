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
# latest release
go install github.com/sert-xx/encave@latest

# …or pin a specific version
go install github.com/sert-xx/encave@v0.4.0
```

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

Your personal `~/.codex` is never touched.

## Commands

```
encave new <owner>/<repo>      Scaffold a draft agent (secrets filtered; README template generated)
encave publish <owner>/<repo>  Scan (fail-closed), commit, tag, and (with a remote) push a draft
encave install <github-url>    Clone an agent and check out a tag into <root>/<owner>/<repo>
encave run [<owner>/<repo>]    Launch an agent ("default" = your own home; omit to pick)
encave auth set|status|clear   Manage credentials in the OS keyring (values never printed)
encave list                    List installed agents and local drafts
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

Launch by reference, or omit it to pick interactively from a numbered list:

```sh
encave dai/review-agent            # run is the default command
encave run                         # choose from the installed agents
```

Anything after `--` is forwarded verbatim to the target CLI. Preview the exact
command (credentials redacted) without launching:

```sh
encave dai/review-agent --dry-run -- exec "review this diff"
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
these: `new` excludes them and `publish` gitignores them. Instead, at launch it
symlinks the agent's `rules` to your base home's `~/.codex/rules`, so your
personal approvals apply to every agent and any new approvals accumulate in one
place. (An agent that already ships a real `rules/` directory is left untouched.)

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

`encave publish` runs a fail-closed secret scan, commits, and tags. Then:

- **With a remote** (`--remote`, or an existing `origin`): it asks
  `Push to <url> now? [y/N]` and pushes the branch and tag on confirmation.
  `-y`/`--yes` skips the prompt; `--no-push` stops after tagging. Non-interactive
  runs never push unless `--yes` is given.
- **Without a remote**: it stops without pushing and explains how to set one
  (the commit and tag are already created locally).

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
    ├── config.toml              # non-secret config (provider, env_key NAME, wire_api)
    ├── .encave.toml             # non-secret agent metadata (target adapter)
    ├── agents/ skills/ ...       # orchestration + skills, vendored for self-containment
    ├── rules -> ~/.codex/rules  # personal settings: symlinked at launch, never packaged
    └── (auth.json never present; .gitignored)
```

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
