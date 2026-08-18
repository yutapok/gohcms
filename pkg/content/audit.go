package content

import (
	"context"
	"time"
)

// AuditOperation represents the type of mutation.
type AuditOperation string

const (
	OpCreate    AuditOperation = "create"
	OpUpdate    AuditOperation = "update"
	OpDelete    AuditOperation = "delete"
	OpPublish   AuditOperation = "publish"
	OpUnpublish AuditOperation = "unpublish"
	OpFinish    AuditOperation = "finish"
)

// AuditLog records a mutation performed on a resource.
type AuditLog struct {
	ID          string         `json:"id"`
	Actor       string         `json:"actor"`
	ActorType   ActorType      `json:"actor_type"`
	Operation   AuditOperation `json:"operation"`
	Resource    string         `json:"resource"`
	ResourceID  string         `json:"resource_id"`
	RequestID   string         `json:"request_id"`
	Timestamp   time.Time      `json:"timestamp"`
	ChangesJSON string         `json:"changes_json,omitempty"`
}

// AuditRepository defines the interface for storing and querying audit logs.
type AuditRepository interface {
	Insert(ctx context.Context, log *AuditLog) error
	List(ctx context.Context, resource string, resourceID string) ([]*AuditLog, error)
}
