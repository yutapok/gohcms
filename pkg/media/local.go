package media

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalStorage stores media files on the local filesystem under a base directory.
type LocalStorage struct {
	baseDir string
}

// NewLocalStorage creates a new LocalStorage.
func NewLocalStorage(baseDir string) (*LocalStorage, error) {
	if baseDir == "" {
		baseDir = "./uploads"
	}

	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create media base directory '%s': %w", baseDir, err)
	}

	return &LocalStorage{baseDir: baseDir}, nil
}

func (s *LocalStorage) Save(ctx context.Context, id string, filename string, r io.Reader) (string, error) {
	cleanFilename := filepath.Base(filename)
	storedName := fmt.Sprintf("%s_%s", id, cleanFilename)
	fullPath := filepath.Join(s.baseDir, storedName)

	file, err := os.Create(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to create destination file '%s': %w", fullPath, err)
	}
	defer file.Close()

	if _, err := io.Copy(file, r); err != nil {
		return "", fmt.Errorf("failed to write media file: %w", err)
	}

	return fullPath, nil
}

func (s *LocalStorage) Open(ctx context.Context, storagePath string) (io.ReadSeekCloser, error) {
	file, err := os.Open(storagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open media file '%s': %w", storagePath, err)
	}
	return file, nil
}

func (s *LocalStorage) Delete(ctx context.Context, storagePath string) error {
	if err := os.Remove(storagePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete media file '%s': %w", storagePath, err)
	}
	return nil
}

var _ Storage = (*LocalStorage)(nil)
