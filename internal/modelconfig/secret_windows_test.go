//go:build windows

package modelconfig

import (
	"context"
	"errors"
	"testing"
)

func TestWindowsCredentialManagerRoundTrip(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	store := NewDefaultSecretStore(home)
	if store.Name() != "Windows Credential Manager" || !store.Persistent() {
		t.Fatalf("unexpected secret store: name=%q persistent=%v", store.Name(), store.Persistent())
	}

	const id = "credential-roundtrip"
	const secret = "memory-harness-windows-credential-acceptance"
	t.Cleanup(func() { _ = store.Delete(ctx, id) })
	if err := store.Set(ctx, id, secret); err != nil {
		t.Fatal(err)
	}

	// A new store instance must read the credential from Windows rather than
	// from process-local state.
	reopened := NewDefaultSecretStore(home)
	value, err := reopened.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if value != secret {
		t.Fatal("Windows Credential Manager returned a different secret")
	}
	if err := reopened.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, id); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("deleted credential remained readable: %v", err)
	}
}
