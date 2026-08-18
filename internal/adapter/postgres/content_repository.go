package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yutapok/gohcms/pkg/content"
	"github.com/yutapok/gohcms/pkg/schema"
)

// DBTX is an interface matching both *sql.DB and *sql.Tx
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// PostgreSQLContentRepository handles CRUD operations on user content tables.
type PostgreSQLContentRepository struct {
	db DBTX
}

// NewContentRepository creates a new PostgreSQLContentRepository.
func NewContentRepository(db DBTX) *PostgreSQLContentRepository {
	return &PostgreSQLContentRepository{db: db}
}

// Get retrieves a record by ID.
func (r *PostgreSQLContentRepository) Get(ctx context.Context, def *schema.ResourceDefinition, id string) (*content.Record, error) {
	query := fmt.Sprintf("SELECT * FROM %s WHERE id = $1 LIMIT 1", quoteIdent(def.Storage.Table))
	rows, err := r.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("failed to query record: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("record '%s' not found in table '%s'", id, def.Storage.Table)
	}

	return scanRecord(rows, def)
}

// List retrieves records matching filters and pagination.
func (r *PostgreSQLContentRepository) List(ctx context.Context, def *schema.ResourceDefinition, filter content.ContentFilter, pagination content.Pagination) ([]*content.Record, int64, error) {
	var whereClauses []string
	var args []interface{}
	argIdx := 1

	if filter.Status != nil && def.Lifecycle.Mode == schema.LifecycleModeManaged {
		whereClauses = append(whereClauses, fmt.Sprintf("%s = $%d", quoteIdent(def.Lifecycle.StatusColumn), argIdx))
		args = append(args, string(*filter.Status))
		argIdx++
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s%s", quoteIdent(def.Storage.Table), whereSQL)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count records: %w", err)
	}

	// Fetch records
	query := fmt.Sprintf("SELECT * FROM %s%s ORDER BY created_at DESC", quoteIdent(def.Storage.Table), whereSQL)
	if pagination.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", pagination.Limit)
	}
	if pagination.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", pagination.Offset)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list records: %w", err)
	}
	defer rows.Close()

	var records []*content.Record
	for rows.Next() {
		rec, err := scanRecord(rows, def)
		if err != nil {
			return nil, 0, err
		}
		records = append(records, rec)
	}

	return records, total, nil
}

// Create inserts a new record.
func (r *PostgreSQLContentRepository) Create(ctx context.Context, def *schema.ResourceDefinition, record *content.Record) (*content.Record, error) {
	if record.ID == "" {
		record.ID = uuid.New().String()
	}

	cols := []string{quoteIdent("id")}
	vals := []interface{}{record.ID}
	placeholders := []string{"$1"}
	idx := 2

	for fieldName, field := range def.Fields {
		if fieldName == "id" {
			continue
		}
		if val, ok := record.Data[fieldName]; ok {
			cols = append(cols, quoteIdent(field.Column))
			vals = append(vals, val)
			placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
			idx++
		}
	}

	if def.Lifecycle.Mode == schema.LifecycleModeManaged {
		cols = append(cols, quoteIdent(def.Lifecycle.StatusColumn), quoteIdent(def.Lifecycle.VersionColumn))
		vals = append(vals, string(record.Status), record.Version)
		placeholders = append(placeholders, fmt.Sprintf("$%d", idx), fmt.Sprintf("$%d", idx+1))
		idx += 2
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING *",
		quoteIdent(def.Storage.Table),
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)

	rows, err := r.db.QueryContext(ctx, query, vals...)
	if err != nil {
		return nil, fmt.Errorf("failed to insert record into table '%s': %w", def.Storage.Table, err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("failed to return inserted record from table '%s'", def.Storage.Table)
	}

	return scanRecord(rows, def)
}

// Update updates an existing record.
func (r *PostgreSQLContentRepository) Update(ctx context.Context, def *schema.ResourceDefinition, record *content.Record) (*content.Record, error) {
	var sets []string
	var vals []interface{}
	idx := 1

	for fieldName, field := range def.Fields {
		if fieldName == "id" || field.Readonly {
			continue
		}
		if val, ok := record.Data[fieldName]; ok {
			sets = append(sets, fmt.Sprintf("%s = $%d", quoteIdent(field.Column), idx))
			vals = append(vals, val)
			idx++
		}
	}

	if def.Lifecycle.Mode == schema.LifecycleModeManaged {
		sets = append(sets, fmt.Sprintf("%s = $%d", quoteIdent(def.Lifecycle.StatusColumn), idx))
		vals = append(vals, string(record.Status))
		idx++

		sets = append(sets, fmt.Sprintf("%s = $%d", quoteIdent(def.Lifecycle.VersionColumn), idx))
		vals = append(vals, record.Version)
		idx++
	}

	sets = append(sets, "updated_at = NOW()")

	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = $%d RETURNING *",
		quoteIdent(def.Storage.Table),
		strings.Join(sets, ", "),
		idx,
	)
	vals = append(vals, record.ID)

	rows, err := r.db.QueryContext(ctx, query, vals...)
	if err != nil {
		return nil, fmt.Errorf("failed to update record in table '%s': %w", def.Storage.Table, err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("record '%s' not found for update in table '%s'", record.ID, def.Storage.Table)
	}

	return scanRecord(rows, def)
}

// Delete removes a record by ID.
func (r *PostgreSQLContentRepository) Delete(ctx context.Context, def *schema.ResourceDefinition, id string) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1", quoteIdent(def.Storage.Table))
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete record '%s' from table '%s': %w", id, def.Storage.Table, err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("record '%s' not found for deletion in table '%s'", id, def.Storage.Table)
	}
	return nil
}

func scanRecord(rows *sql.Rows, def *schema.ResourceDefinition) (*content.Record, error) {
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, fmt.Errorf("failed to get column types: %w", err)
	}

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	values := make([]interface{}, len(cols))
	valuePtrs := make([]interface{}, len(cols))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	if err := rows.Scan(valuePtrs...); err != nil {
		return nil, fmt.Errorf("failed to scan row: %w", err)
	}

	rec := &content.Record{
		Data: make(map[string]interface{}),
	}

	for i, colName := range cols {
		val := values[i]
		if b, ok := val.([]byte); ok {
			val = string(b)
		}

		if colName == "id" {
			if strVal, ok := val.(string); ok {
				rec.ID = strVal
			}
		}

		if def.Lifecycle.Mode == schema.LifecycleModeManaged {
			if colName == def.Lifecycle.StatusColumn {
				if strVal, ok := val.(string); ok {
					rec.Status = content.ContentStatus(strVal)
				}
			}
			if colName == def.Lifecycle.VersionColumn {
				if intVal, ok := val.(int64); ok {
					rec.Version = intVal
				}
			}
		}

		if colName == "created_at" {
			if t, ok := val.(time.Time); ok {
				rec.CreatedAt = t
			}
		}
		if colName == "updated_at" {
			if t, ok := val.(time.Time); ok {
				rec.UpdatedAt = t
			}
		}

		// Map to resource field names
		for fieldName, field := range def.Fields {
			if field.Column == colName {
				rec.Data[fieldName] = val
			}
		}
		rec.Data[colName] = val
	}

	_ = colTypes
	return rec, nil
}

// PostgreSQLAuditRepository stores audit logs in cms_audit_logs table.
type PostgreSQLAuditRepository struct {
	db DBTX
}

func NewAuditRepository(db DBTX) *PostgreSQLAuditRepository {
	return &PostgreSQLAuditRepository{db: db}
}

func (r *PostgreSQLAuditRepository) Insert(ctx context.Context, log *content.AuditLog) error {
	if log.ID == "" {
		log.ID = uuid.New().String()
	}
	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now()
	}

	query := `
		INSERT INTO cms_audit_logs (id, actor, actor_type, operation, resource, resource_id, request_id, timestamp, changes_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := r.db.ExecContext(ctx, query, log.ID, log.Actor, string(log.ActorType), string(log.Operation), log.Resource, log.ResourceID, log.RequestID, log.Timestamp, log.ChangesJSON)
	if err != nil {
		return fmt.Errorf("failed to insert audit log: %w", err)
	}
	return nil
}

func (r *PostgreSQLAuditRepository) List(ctx context.Context, resource string, resourceID string) ([]*content.AuditLog, error) {
	query := `
		SELECT id, actor, actor_type, operation, resource, resource_id, request_id, timestamp, changes_json
		FROM cms_audit_logs
		WHERE resource = $1 AND resource_id = $2
		ORDER BY timestamp DESC
	`
	rows, err := r.db.QueryContext(ctx, query, resource, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit logs: %w", err)
	}
	defer rows.Close()

	var logs []*content.AuditLog
	for rows.Next() {
		var l content.AuditLog
		var actorType, operation string
		if err := rows.Scan(&l.ID, &l.Actor, &actorType, &operation, &l.Resource, &l.ResourceID, &l.RequestID, &l.Timestamp, &l.ChangesJSON); err != nil {
			return nil, fmt.Errorf("failed to scan audit log: %w", err)
		}
		l.ActorType = content.ActorType(actorType)
		l.Operation = content.AuditOperation(operation)
		logs = append(logs, &l)
	}
	return logs, nil
}

// PostgreSQLRevisionRepository stores revisions in cms_revisions table.
type PostgreSQLRevisionRepository struct {
	db DBTX
}

func NewRevisionRepository(db DBTX) *PostgreSQLRevisionRepository {
	return &PostgreSQLRevisionRepository{db: db}
}

func (r *PostgreSQLRevisionRepository) Insert(ctx context.Context, rev *content.Revision) error {
	if rev.ID == "" {
		rev.ID = uuid.New().String()
	}
	if rev.CreatedAt.IsZero() {
		rev.CreatedAt = time.Now()
	}

	query := `
		INSERT INTO cms_revisions (id, resource, resource_id, version, schema_version, snapshot_json, created_at, actor)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.db.ExecContext(ctx, query, rev.ID, rev.Resource, rev.ResourceID, rev.Version, rev.SchemaVersion, rev.SnapshotJSON, rev.CreatedAt, rev.Actor)
	if err != nil {
		return fmt.Errorf("failed to insert revision: %w", err)
	}
	return nil
}

func (r *PostgreSQLRevisionRepository) Get(ctx context.Context, resource string, resourceID string, version int64) (*content.Revision, error) {
	query := `
		SELECT id, resource, resource_id, version, schema_version, snapshot_json, created_at, actor
		FROM cms_revisions
		WHERE resource = $1 AND resource_id = $2 AND version = $3
		LIMIT 1
	`
	row := r.db.QueryRowContext(ctx, query, resource, resourceID, version)
	var rev content.Revision
	if err := row.Scan(&rev.ID, &rev.Resource, &rev.ResourceID, &rev.Version, &rev.SchemaVersion, &rev.SnapshotJSON, &rev.CreatedAt, &rev.Actor); err != nil {
		return nil, fmt.Errorf("revision not found: %w", err)
	}
	return &rev, nil
}

func (r *PostgreSQLRevisionRepository) List(ctx context.Context, resource string, resourceID string) ([]*content.Revision, error) {
	query := `
		SELECT id, resource, resource_id, version, schema_version, snapshot_json, created_at, actor
		FROM cms_revisions
		WHERE resource = $1 AND resource_id = $2
		ORDER BY version DESC
	`
	rows, err := r.db.QueryContext(ctx, query, resource, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list revisions: %w", err)
	}
	defer rows.Close()

	var list []*content.Revision
	for rows.Next() {
		var rev content.Revision
		if err := rows.Scan(&rev.ID, &rev.Resource, &rev.ResourceID, &rev.Version, &rev.SchemaVersion, &rev.SnapshotJSON, &rev.CreatedAt, &rev.Actor); err != nil {
			return nil, fmt.Errorf("failed to scan revision: %w", err)
		}
		list = append(list, &rev)
	}
	return list, nil
}

// PostgreSQLUnitOfWork executes functions inside a PostgreSQL transaction.
type PostgreSQLUnitOfWork struct {
	db *sql.DB
}

func NewUnitOfWork(db *sql.DB) *PostgreSQLUnitOfWork {
	return &PostgreSQLUnitOfWork{db: db}
}

func (u *PostgreSQLUnitOfWork) Execute(ctx context.Context, fn func(repo content.ContentRepository, auditRepo content.AuditRepository, revRepo content.RevisionRepository) error) error {
	tx, err := u.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	txRepo := NewContentRepository(tx)
	txAudit := NewAuditRepository(tx)
	txRev := NewRevisionRepository(tx)

	if err := fn(txRepo, txAudit, txRev); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// Compile-time interface checks
var (
	_ content.ContentRepository  = (*PostgreSQLContentRepository)(nil)
	_ content.AuditRepository    = (*PostgreSQLAuditRepository)(nil)
	_ content.RevisionRepository = (*PostgreSQLRevisionRepository)(nil)
	_ content.UnitOfWork         = (*PostgreSQLUnitOfWork)(nil)
)
