package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/yutapok/gohcms/pkg/introspection"
)

// Inspector inspects a PostgreSQL database to extract schema metadata.
type Inspector struct {
	db *sql.DB
}

// NewInspector creates a new Inspector with a database connection string.
func NewInspector(databaseURL string) (*Inspector, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	return &Inspector{db: db}, nil
}

// NewInspectorWithDB creates an Inspector using an existing *sql.DB.
func NewInspectorWithDB(db *sql.DB) *Inspector {
	return &Inspector{db: db}
}

// Close closes the underlying database connection.
func (i *Inspector) Close() error {
	if i.db != nil {
		return i.db.Close()
	}
	return nil
}

// ReadSchema reads schema metadata for specified tables (or all tables if empty) from PostgreSQL.
func (i *Inspector) ReadSchema(ctx context.Context, tables []string) (*introspection.DatabaseSchema, error) {
	if i.db == nil {
		return nil, fmt.Errorf("database connection is not initialized")
	}

	query := `
		SELECT 
			table_name,
			column_name,
			data_type,
			udt_name,
			is_nullable,
			column_default
		FROM information_schema.columns
		WHERE table_schema = current_schema()
	`

	var args []interface{}
	if len(tables) > 0 {
		placeholders := make([]string, len(tables))
		for idx, t := range tables {
			placeholders[idx] = fmt.Sprintf("$%d", idx+1)
			args = append(args, t)
		}
		query += fmt.Sprintf(" AND table_name IN (%s)", strings.Join(placeholders, ", "))
	}
	query += " ORDER BY table_name, ordinal_position;"

	rows, err := i.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query database columns: %w", err)
	}
	defer rows.Close()

	schema := introspection.NewDatabaseSchema()

	for rows.Next() {
		var tableName, columnName, dataType, udtName, isNullable string
		var columnDefault sql.NullString

		if err := rows.Scan(&tableName, &columnName, &dataType, &udtName, &isNullable, &columnDefault); err != nil {
			return nil, fmt.Errorf("failed to scan column row: %w", err)
		}

		table, exists := schema.GetTable(tableName)
		if !exists {
			table = introspection.TableSchema{
				Name:    tableName,
				Columns: make(map[string]introspection.ColumnSchema),
			}
		}

		var defaultVal *string
		if columnDefault.Valid {
			defaultVal = &columnDefault.String
		}

		table.Columns[columnName] = introspection.ColumnSchema{
			Name:         columnName,
			DataType:     dataType,
			UDTName:      udtName,
			IsNullable:   strings.ToUpper(isNullable) == "YES",
			DefaultValue: defaultVal,
		}

		schema.AddTable(table)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating column rows: %w", err)
	}

	return schema, nil
}
