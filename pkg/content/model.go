package content

import (
	"time"
)

// Record represents a single content entity as a dynamic map of fields.
type Record struct {
	ID        string                 `json:"id"`
	Data      map[string]interface{} `json:"data"`
	Status    ContentStatus          `json:"status,omitempty"`
	Version   int64                  `json:"version,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// GetField retrieves a field value from the record.
func (r *Record) GetField(name string) (interface{}, bool) {
	if r.Data == nil {
		return nil, false
	}
	val, ok := r.Data[name]
	return val, ok
}

// SetField sets a field value in the record.
func (r *Record) SetField(name string, val interface{}) {
	if r.Data == nil {
		r.Data = make(map[string]interface{})
	}
	r.Data[name] = val
}

// ActorType represents who performed a mutation.
type ActorType string

const (
	ActorTypeUser      ActorType = "user"
	ActorTypeAPIClient ActorType = "api_client"
	ActorTypeSystem    ActorType = "system"
	ActorTypeAgent     ActorType = "agent"
)

// MutationContext carries metadata about an operation for auditing and revision tracking.
type MutationContext struct {
	Actor     string
	ActorType ActorType
	RequestID string
	Timestamp time.Time
}

// Pagination parameters for listing records.
type Pagination struct {
	Limit  int
	Offset int
}

// ContentFilter parameters for listing records.
type ContentFilter struct {
	Status *ContentStatus
	Fields map[string]interface{}
}
