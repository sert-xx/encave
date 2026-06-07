package secrets

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestSetHasResolveDelete(t *testing.T) {
	keyring.MockInit() // in-memory keyring, no OS dependency

	// Nothing set yet.
	if has, _ := Has("global"); has {
		t.Fatal("expected global unset initially")
	}

	// Global credential resolves for any agent lacking its own.
	if err := Set(GlobalScope, "global-token"); err != nil {
		t.Fatal(err)
	}
	v, scope, err := Resolve("dai/review-agent")
	if err != nil {
		t.Fatal(err)
	}
	if v != "global-token" || scope != GlobalScope {
		t.Fatalf("resolve = %q/%q, want global-token/global", v, scope)
	}

	// Agent-specific credential takes precedence.
	if err := Set("dai/review-agent", "agent-token"); err != nil {
		t.Fatal(err)
	}
	v, scope, err = Resolve("dai/review-agent")
	if err != nil {
		t.Fatal(err)
	}
	if v != "agent-token" || scope != "dai/review-agent" {
		t.Fatalf("resolve = %q/%q, want agent-token/dai/review-agent", v, scope)
	}

	// Delete agent-specific; should fall back to global again.
	if err := Delete("dai/review-agent"); err != nil {
		t.Fatal(err)
	}
	v, scope, _ = Resolve("dai/review-agent")
	if v != "global-token" || scope != GlobalScope {
		t.Fatalf("after delete, resolve = %q/%q, want global", v, scope)
	}
}

func TestResolveNotFound(t *testing.T) {
	keyring.MockInit()
	_, _, err := Resolve("nobody/here")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteMissingIsNoError(t *testing.T) {
	keyring.MockInit()
	if err := Delete("nobody/here"); err != nil {
		t.Fatalf("deleting missing entry should be a no-op, got %v", err)
	}
}
