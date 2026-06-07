// Package secrets is encave's thin, security-shaped wrapper over the OS keyring
// (macOS Keychain / Windows Credential Manager / Linux Secret Service).
//
// Design constraints (see design doc §3.3, §3.4, §6):
//   - This keyring is encave's *own* vault, distinct from any auth store the
//     target CLI uses. The target receives credentials only as environment
//     variables at launch (see internal/cli/run.go).
//   - There is deliberately NO function that prints a secret to stdout, and the
//     CLI exposes no such command. The only consumer of Get is the in-process
//     launch path, which writes the value straight into the child's environment.
//     This avoids creating a standing credential-dump oracle.
package secrets

import (
	"errors"
	"fmt"
	"strings"

	"github.com/zalando/go-keyring"
)

// Service is the keyring service name under which encave stores its entries.
const Service = "encave"

// GlobalScope is the keyring key used for the account-wide credential that
// applies to any agent lacking a dedicated entry.
const GlobalScope = "global"

// ErrNotFound is returned when no credential exists for a scope.
var ErrNotFound = keyring.ErrNotFound

// keyFor maps an encave scope to a keyring key. An empty scope means global.
func keyFor(scope string) string {
	if scope == "" {
		return GlobalScope
	}
	return scope
}

// Set stores value under the given scope (an agent "owner/repo" or "global").
func Set(scope, value string) error {
	return annotate(keyring.Set(Service, keyFor(scope), value))
}

// Has reports whether a credential exists for scope without revealing its value.
func Has(scope string) (bool, error) {
	_, err := keyring.Get(Service, keyFor(scope))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, keyring.ErrNotFound) {
		return false, nil
	}
	return false, annotate(err)
}

// Delete removes the credential for scope. Deleting a non-existent entry is not
// an error.
func Delete(scope string) error {
	err := keyring.Delete(Service, keyFor(scope))
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return annotate(err)
}

// annotate wraps low-level keyring errors with guidance when the OS credential
// service appears to be unavailable (common on headless Linux/CI hosts that lack
// a running Secret Service).
func annotate(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "dbus") ||
		strings.Contains(msg, "Secret Service") ||
		strings.Contains(msg, "org.freedesktop.secrets") {
		return fmt.Errorf("%w\n  the OS keyring is unavailable: on Linux, encave needs a running Secret Service "+
			"(e.g. gnome-keyring or KeePassXC's Secret Service); on a headless host, unlock one in your session", err)
	}
	return err
}

// Resolve fetches the credential for an agent, falling back from the
// agent-specific entry to the global entry. It returns the value, the scope it
// came from, and any error. ErrNotFound means neither scope had a credential.
//
// Resolve is intentionally the only value-returning function and is meant to be
// called solely on the launch path.
func Resolve(agent string) (value string, scope string, err error) {
	if agent != "" {
		v, gerr := keyring.Get(Service, agent)
		if gerr == nil {
			return v, agent, nil
		}
		if !errors.Is(gerr, keyring.ErrNotFound) {
			return "", "", gerr
		}
	}
	v, gerr := keyring.Get(Service, GlobalScope)
	if gerr == nil {
		return v, GlobalScope, nil
	}
	if errors.Is(gerr, keyring.ErrNotFound) {
		return "", "", ErrNotFound
	}
	return "", "", gerr
}
