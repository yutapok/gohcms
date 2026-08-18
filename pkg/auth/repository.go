package auth

import (
	"context"
)

// APIKeyRepository defines storage operations for API keys.
type APIKeyRepository interface {
	Insert(ctx context.Context, key *APIKey) error
	GetByHash(ctx context.Context, hash string) (*APIKey, error)
	List(ctx context.Context) ([]*APIKey, error)
	Revoke(ctx context.Context, id string) error
}
