package content

import (
	"context"

	"github.com/yutapok/gohcms/pkg/schema"
)

// ContentRepository defines the low-level database operations for CMS content tables.
type ContentRepository interface {
	Get(ctx context.Context, def *schema.ResourceDefinition, id string) (*Record, error)
	List(ctx context.Context, def *schema.ResourceDefinition, filter ContentFilter, pagination Pagination) ([]*Record, int64, error)
	Create(ctx context.Context, def *schema.ResourceDefinition, record *Record) (*Record, error)
	Update(ctx context.Context, def *schema.ResourceDefinition, record *Record) (*Record, error)
	Delete(ctx context.Context, def *schema.ResourceDefinition, id string) error
}

// UnitOfWork runs content mutations, audit logging, and revisions within a single database transaction.
type UnitOfWork interface {
	Execute(ctx context.Context, fn func(repo ContentRepository, auditRepo AuditRepository, revRepo RevisionRepository) error) error
}
