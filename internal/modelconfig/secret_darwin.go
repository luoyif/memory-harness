//go:build darwin

package modelconfig

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os/exec"
	"strings"
)

type keychainSecretStore struct{ service string }

func (s *keychainSecretStore) Name() string     { return "macOS Keychain" }
func (s *keychainSecretStore) Persistent() bool { return true }

func NewDefaultSecretStore(home string) SecretStore {
	digest := sha256.Sum256([]byte(home))
	return &keychainSecretStore{service: "MemoryOS.ModelProviders." + hex.EncodeToString(digest[:8])}
}

func (s *keychainSecretStore) Set(ctx context.Context, id, value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("API key cannot be empty")
	}
	command := exec.CommandContext(ctx, "/usr/bin/security", "add-generic-password", "-U", "-a", id, "-s", s.service, "-w", value)
	if output, err := command.CombinedOutput(); err != nil {
		return errors.New("could not store API key in macOS Keychain: " + strings.TrimSpace(string(output)))
	}
	return nil
}

func (s *keychainSecretStore) Get(ctx context.Context, id string) (string, error) {
	command := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-a", id, "-s", s.service, "-w")
	output, err := command.Output()
	if err != nil {
		return "", ErrSecretNotFound
	}
	value := strings.TrimSpace(string(output))
	if value == "" {
		return "", ErrSecretNotFound
	}
	return value, nil
}

func (s *keychainSecretStore) Delete(ctx context.Context, id string) error {
	command := exec.CommandContext(ctx, "/usr/bin/security", "delete-generic-password", "-a", id, "-s", s.service)
	if err := command.Run(); err != nil {
		return nil
	}
	return nil
}
