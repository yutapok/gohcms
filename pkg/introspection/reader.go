package introspection

import (
	"context"
)

// DatabaseSchemaReader reads database schema metadata.
type DatabaseSchemaReader interface {
	ReadSchema(ctx context.Context, tables []string) (*DatabaseSchema, error)
}

// InMemorySchemaReader is an in-memory implementation of DatabaseSchemaReader for testing and standalone usage.
type InMemorySchemaReader struct {
	Schema *DatabaseSchema
}

// NewInMemorySchemaReader creates a new InMemorySchemaReader.
func NewInMemorySchemaReader(schema *DatabaseSchema) *InMemorySchemaReader {
	return &InMemorySchemaReader{Schema: schema}
}

// ReadSchema returns the configured in-memory schema, optionally filtered by table names.
func (r *InMemorySchemaReader) ReadSchema(ctx context.Context, tables []string) (*DatabaseSchema, error) {
	if len(tables) == 0 {
		return r.Schema, nil
	}

	filtered := NewDatabaseSchema()
	for _, tableName := range tables {
		if table, exists := r.Schema.GetTable(tableName); exists {
			filtered.AddTable(table)
		}
	}
	return filtered, nil
}
