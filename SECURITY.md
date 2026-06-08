# Security Policy

encave handles credentials, so security reports are taken seriously. This
document explains what is in scope and how to report an issue privately.

## Supported versions

encave is pre-1.0 and ships as a single binary installed from a tagged release.
Only the **latest released tag** is supported. Please reproduce any issue on the
newest release before reporting.

| Version        | Supported |
| -------------- | --------- |
| latest release | ✅        |
| older releases | ❌        |

## Reporting a vulnerability

**Please do not open a public issue for a security vulnerability.**

Use GitHub's private vulnerability reporting instead:

1. Go to the repository's **Security** tab.
2. Click **Report a vulnerability** (GitHub Security Advisories).
3. Describe the issue, the affected version (`encave version`), your OS and
   keyring backend, and a minimal reproduction.

You can expect an initial acknowledgement within a few days. Once a fix is
ready, a new release is tagged and the advisory is published with credit to the
reporter (unless you prefer to remain anonymous).

## Scope and threat model

encave's security posture is described in detail in the README ("Security
model"). In short:

- Credentials live only in the **OS keyring** and are injected into a launched
  child process's environment for that process's lifetime — never written to the
  repository, logs, or stdout.
- There is intentionally **no command that prints a stored credential**.
- `encave publish` runs a **fail-closed secret scan** before committing.

A known, accepted residual risk is that same-user code can read a running
child process's environment — this is the irreducible floor for handing a secret
to a local child process. Reports that rely solely on this are out of scope.

Reports demonstrating any of the following are in scope and welcome:

- A credential leaking into the git repository, a commit, logs, or stdout.
- A way to make `encave publish` commit a secret the scanner should have caught.
- An installed agent escaping its isolated home or reading/modifying your
  personal `~/.codex`.
- Path traversal via an agent reference or a crafted repository.
