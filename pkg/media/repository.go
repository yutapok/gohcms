package media

import (
	"context"
)

// MediaRepository defines operations for media metadata persistence.
type MediaRepository interface {
	Insert(ctx context.Context, m *Media) error
	Get(ctx context.Context, id string) (*Media, error)
	List(ctx context.Context) ([]*Media, error)
	Delete(ctx context.Context, id string) error
}
