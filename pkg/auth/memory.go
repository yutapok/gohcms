package auth

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryAPIKeyRepository provides thread-safe in-memory storage for API keys.
type MemoryAPIKeyRepository struct {
	mu   sync.RWMutex
	keys map[string]*APIKey // ID -> APIKey
}

// NewMemoryAPIKeyRepository creates a new in-memory API key repository.
func NewMemoryAPIKeyRepository() *MemoryAPIKeyRepository {
	return &MemoryAPIKeyRepository{
		keys: make(map[string]*APIKey),
	}
}

func (m *MemoryAPIKeyRepository) Insert(ctx context.Context, key *APIKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cp := *key
	m.keys[key.ID] = &cp
	return nil
}

func (m *MemoryAPIKeyRepository) GetByHash(ctx context.Context, hash string) (*APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, k := range m.keys {
		if k.TokenHash == hash {
			cp := *k
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("api key with hash '%s' not found", hash)
}

func (m *MemoryAPIKeyRepository) List(ctx context.Context) ([]*APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []*APIKey
	for _, k := range m.keys {
		cp := *k
		list = append(list, &cp)
	}
	return list, nil
}

func (m *MemoryAPIKeyRepository) Revoke(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	k, exists := m.keys[id]
	if !exists {
		return fmt.Errorf("api key '%s' not found", id)
	}

	now := time.Now()
	k.IsActive = false
	k.RevokedAt = &now
	return nil
}

var _ APIKeyRepository = (*MemoryAPIKeyRepository)(nil)
