package modelconfig

import (
	"context"
	"errors"
	"sync"
)

var ErrSecretNotFound = errors.New("model provider secret not found")

type SecretStore interface {
	Set(context.Context, string, string) error
	Get(context.Context, string) (string, error)
	Delete(context.Context, string) error
	Name() string
	Persistent() bool
}

type MemorySecretStore struct {
	mu     sync.Mutex
	values map[string]string
}

func NewMemorySecretStore() *MemorySecretStore {
	return &MemorySecretStore{values: map[string]string{}}
}

func (s *MemorySecretStore) Name() string     { return "volatile process memory" }
func (s *MemorySecretStore) Persistent() bool { return false }

func (s *MemorySecretStore) Set(_ context.Context, id, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[id] = value
	return nil
}

func (s *MemorySecretStore) Get(_ context.Context, id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[id]
	if !ok {
		return "", ErrSecretNotFound
	}
	return value, nil
}

func (s *MemorySecretStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, id)
	return nil
}
