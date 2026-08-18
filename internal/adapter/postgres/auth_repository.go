package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/yutapok/gohcms/pkg/auth"
)

// PostgreSQLAPIKeyRepository handles API key persistence in cms_api_keys table.
type PostgreSQLAPIKeyRepository struct {
	db DBTX
}

// NewAPIKeyRepository creates a new PostgreSQLAPIKeyRepository.
func NewAPIKeyRepository(db DBTX) *PostgreSQLAPIKeyRepository {
	return &PostgreSQLAPIKeyRepository{db: db}
}

func (r *PostgreSQLAPIKeyRepository) Insert(ctx context.Context, key *auth.APIKey) error {
	query := `
		INSERT INTO cms_api_keys (id, name, token_hash, permission, is_active, created_at, revoked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.db.ExecContext(ctx, query, key.ID, key.Name, key.TokenHash, string(key.Permission), key.IsActive, key.CreatedAt, key.RevokedAt)
	if err != nil {
		return fmt.Errorf("failed to insert api key: %w", err)
	}
	return nil
}

func (r *PostgreSQLAPIKeyRepository) GetByHash(ctx context.Context, hash string) (*auth.APIKey, error) {
	query := `
		SELECT id, name, token_hash, permission, is_active, created_at, revoked_at
		FROM cms_api_keys
		WHERE token_hash = $1
		LIMIT 1
	`
	row := r.db.QueryRowContext(ctx, query, hash)

	var k auth.APIKey
	var perm string
	var revokedAt sql.NullTime

	if err := row.Scan(&k.ID, &k.Name, &k.TokenHash, &perm, &k.IsActive, &k.CreatedAt, &revokedAt); err != nil {
		return nil, fmt.Errorf("failed to get api key: %w", err)
	}

	k.Permission = auth.Permission(perm)
	if revokedAt.Valid {
		k.RevokedAt = &revokedAt.Time
	}

	return &k, nil
}

func (r *PostgreSQLAPIKeyRepository) List(ctx context.Context) ([]*auth.APIKey, error) {
	query := `
		SELECT id, name, token_hash, permission, is_active, created_at, revoked_at
		FROM cms_api_keys
		ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list api keys: %w", err)
	}
	defer rows.Close()

	var list []*auth.APIKey
	for rows.Next() {
		var k auth.APIKey
		var perm string
		var revokedAt sql.NullTime

		if err := rows.Scan(&k.ID, &k.Name, &k.TokenHash, &perm, &k.IsActive, &k.CreatedAt, &revokedAt); err != nil {
			return nil, fmt.Errorf("failed to scan api key: %w", err)
		}

		k.Permission = auth.Permission(perm)
		if revokedAt.Valid {
			k.RevokedAt = &revokedAt.Time
		}
		list = append(list, &k)
	}

	return list, nil
}

func (r *PostgreSQLAPIKeyRepository) Revoke(ctx context.Context, id string) error {
	now := time.Now()
	query := `
		UPDATE cms_api_keys
		SET is_active = FALSE, revoked_at = $1
		WHERE id = $2
	`
	res, err := r.db.ExecContext(ctx, query, now, id)
	if err != nil {
		return fmt.Errorf("failed to revoke api key: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("api key '%s' not found for revocation", id)
	}
	return nil
}

var _ auth.APIKeyRepository = (*PostgreSQLAPIKeyRepository)(nil)
