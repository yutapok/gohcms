package media

import (
	"context"
	"io"
)

// Storage defines the minimal abstraction for saving and retrieving physical media files.
type Storage interface {
	Save(ctx context.Context, id string, filename string, r io.Reader) (string, error)
	Open(ctx context.Context, storagePath string) (io.ReadSeekCloser, error)
	Delete(ctx context.Context, storagePath string) error
}
