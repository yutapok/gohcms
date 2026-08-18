package media

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
)

// MemoryStorage stores media files in an in-memory byte map for demo/tests.
type MemoryStorage struct {
	mu    sync.RWMutex
	files map[string][]byte
}

// NewMemoryStorage creates a new MemoryStorage.
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		files: make(map[string][]byte),
	}
}

type nopCloserReadSeeker struct {
	*bytes.Reader
}

func (n *nopCloserReadSeeker) Close() error {
	return nil
}

func (s *MemoryStorage) Save(ctx context.Context, id string, filename string, r io.Reader) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("failed to read data: %w", err)
	}

	path := fmt.Sprintf("memory://%s_%s", id, filename)
	s.files[path] = data
	return path, nil
}

func (s *MemoryStorage) Open(ctx context.Context, storagePath string) (io.ReadSeekCloser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, exists := s.files[storagePath]
	if !exists {
		return nil, fmt.Errorf("file not found in memory: %s", storagePath)
	}

	return &nopCloserReadSeeker{Reader: bytes.NewReader(data)}, nil
}

func (s *MemoryStorage) Delete(ctx context.Context, storagePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.files, storagePath)
	return nil
}

var _ Storage = (*MemoryStorage)(nil)
