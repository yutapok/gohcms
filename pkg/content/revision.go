package content

import (
	"context"
	"time"
)

// Revision represents an immutable snapshot of a record version.
type Revision struct {
	ID            string    `json:"id"`
	Resource      string    `json:"resource"`
	ResourceID    string    `json:"resource_id"`
	Version       int64     `json:"version"`
	SchemaVersion string    `json:"schema_version,omitempty"`
	SnapshotJSON  string    `json:"snapshot_json"`
	CreatedAt     time.Time `json:"created_at"`
	Actor         string    `json:"actor"`
}

// RevisionRepository defines the interface for storing and querying revisions.
type RevisionRepository interface {
	Insert(ctx context.Context, rev *Revision) error
	Get(ctx context.Context, resource string, resourceID string, version int64) (*Revision, error)
	List(ctx context.Context, resource string, resourceID string) ([]*Revision, error)
}
