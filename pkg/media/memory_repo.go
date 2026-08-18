package media

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryMediaRepository provides thread-safe in-memory metadata storage.
type MemoryMediaRepository struct {
	mu    sync.RWMutex
	media map[string]*Media
}

// NewMemoryMediaRepository creates a new in-memory MediaRepository.
func NewMemoryMediaRepository() *MemoryMediaRepository {
	return &MemoryMediaRepository{
		media: make(map[string]*Media),
	}
}

func (m *MemoryMediaRepository) Insert(ctx context.Context, item *Media) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if item.ID == "" {
		item.ID = uuid.New().String()
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now()
	}

	cp := *item
	m.media[item.ID] = &cp
	return nil
}

func (m *MemoryMediaRepository) Get(ctx context.Context, id string) (*Media, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, exists := m.media[id]
	if !exists {
		return nil, fmt.Errorf("media '%s' not found", id)
	}

	cp := *item
	return &cp, nil
}

func (m *MemoryMediaRepository) List(ctx context.Context) ([]*Media, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []*Media
	for _, item := range m.media {
		cp := *item
		list = append(list, &cp)
	}
	return list, nil
}

func (m *MemoryMediaRepository) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.media, id)
	return nil
}

var _ MediaRepository = (*MemoryMediaRepository)(nil)
