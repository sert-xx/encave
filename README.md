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

## Install / build

Requires Go 1.25+.

```sh
make build      # produces ./encave
make test       # run the test suite
make install    # go install into $GOBIN
```

On **Linux**, the keyring needs a running **Secret Service** (e.g.
`gnome-keyring` or KeePassXC's Secret Service). macOS uses the Keychain and
Windows uses the Credential Manager out of the box.

## Commands

```
encave new <name>              Scaffold a draft agent from your base home (secrets filtered)
encave publish <name>          Scan (fail-closed), commit, tag, and (with a remote) push a draft
encave install <github-url>    Clone an agent and check out a tag into <root>/<owner>/<repo>
encave run <owner>/<repo>      Launch an agent in its isolated home with injected auth
encave auth set|status|clear   Manage credentials in the OS keyring (values never printed)
encave list                    List installed agents and local drafts
encave version | help
```

`run` is the **default command**: `encave dai/review-agent` is the same as
`encave run dai/review-agent`. Anything after `--` is forwarded verbatim to the
target CLI.

### Provider side (the person with the tuned setup)

```sh
encave new review-agent                 # copy ~/.codex into a draft, filtering secrets/state
# ... tune agents/, skills/, config.toml ...

# Create the empty repo on GitHub first, then publish with a remote:
encave publish review-agent --tag v1.0.0 --remote git@github.com:dai/review-agent.git
# fail-closed secret scan -> commit -> tag -> prompt to push to origin.
```

`encave publish` runs the secret scan, commits, and tags. Then, for pushing:

- **A remote is configured** (via `--remote`, or an existing `origin`): it asks
  `Push to <url> now? [y/N]` and pushes the branch and tag on confirmation.
  Use `-y`/`--yes` to skip the prompt (automation), or `--no-push` to stop after
  tagging.
- **No remote is configured**: it stops without pushing and tells you to set one
  (the commit and tag are already created locally and ready to push).

In non-interactive contexts (no TTY), publish never pushes unless `--yes` is
given.

### Consumer side (a teammate)

```sh
encave auth set --global                          # store the proxy PAT in the keyring (once; refresh on expiry)
encave install github.com/dai/review-agent --tag v1.0.0
encave dai/review-agent                            # launch with isolated home + injected auth
```

Inspect exactly what would run, with credentials redacted, without launching:

```sh
encave dai/review-agent --dry-run -- exec "review this diff"
```

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
├── _drafts/<name>/              # `encave new` working area (unpublished)
└── <owner>/<repo>/              # an installed agent = one isolated agent home
    ├── config.toml              # non-secret config (provider, env_key NAME, wire_api)
    ├── .encave.toml             # non-secret agent metadata (target adapter)
    ├── agents/ skills/ ...       # orchestration + skills, vendored for self-containment
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
