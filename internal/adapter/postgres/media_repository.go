package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/yutapok/gohcms/pkg/media"
)

// PostgreSQLMediaRepository persists media metadata in cms_media table.
type PostgreSQLMediaRepository struct {
	db DBTX
}

// NewMediaRepository creates a new PostgreSQLMediaRepository.
func NewMediaRepository(db DBTX) *PostgreSQLMediaRepository {
	return &PostgreSQLMediaRepository{db: db}
}

func (r *PostgreSQLMediaRepository) Insert(ctx context.Context, m *media.Media) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}

	query := `
		INSERT INTO cms_media (id, filename, filepath, mime_type, size_bytes, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.db.ExecContext(ctx, query, m.ID, m.Filename, m.Filepath, m.MimeType, m.SizeBytes, m.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert media metadata: %w", err)
	}
	return nil
}

func (r *PostgreSQLMediaRepository) Get(ctx context.Context, id string) (*media.Media, error) {
	query := `
		SELECT id, filename, filepath, mime_type, size_bytes, created_at
		FROM cms_media
		WHERE id = $1
		LIMIT 1
	`
	row := r.db.QueryRowContext(ctx, query, id)

	var m media.Media
	if err := row.Scan(&m.ID, &m.Filename, &m.Filepath, &m.MimeType, &m.SizeBytes, &m.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("media '%s' not found", id)
		}
		return nil, fmt.Errorf("failed to query media: %w", err)
	}

	m.FormatURL()
	return &m, nil
}

func (r *PostgreSQLMediaRepository) List(ctx context.Context) ([]*media.Media, error) {
	query := `
		SELECT id, filename, filepath, mime_type, size_bytes, created_at
		FROM cms_media
		ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list media metadata: %w", err)
	}
	defer rows.Close()

	var list []*media.Media
	for rows.Next() {
		var m media.Media
		if err := rows.Scan(&m.ID, &m.Filename, &m.Filepath, &m.MimeType, &m.SizeBytes, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan media metadata: %w", err)
		}
		m.FormatURL()
		list = append(list, &m)
	}

	return list, nil
}

func (r *PostgreSQLMediaRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM cms_media WHERE id = $1`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete media metadata: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("media '%s' not found for deletion", id)
	}
	return nil
}

var _ media.MediaRepository = (*PostgreSQLMediaRepository)(nil)
