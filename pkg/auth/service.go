package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/yutapok/gohcms/pkg/content"
)

// Service coordinates API key generation, validation, and audit logging.
type Service struct {
	repo      APIKeyRepository
	auditRepo content.AuditRepository
}

// NewService creates a new AuthService.
func NewService(repo APIKeyRepository, auditRepo content.AuditRepository) *Service {
	return &Service{
		repo:      repo,
		auditRepo: auditRepo,
	}
}

// CreateKey generates a new API token, persists its hash, and logs to audit repository.
func (s *Service) CreateKey(ctx context.Context, name string, perm Permission, mctx content.MutationContext) (string, *APIKey, error) {
	if name == "" {
		return "", nil, fmt.Errorf("api key name is required")
	}
	if perm != PermissionRead && perm != PermissionReadWrite {
		perm = PermissionReadWrite
	}

	rawToken, key, err := GenerateToken(name, perm)
	if err != nil {
		return "", nil, err
	}

	if err := s.repo.Insert(ctx, key); err != nil {
		return "", nil, fmt.Errorf("failed to save api key: %w", err)
	}

	if s.auditRepo != nil {
		changes, _ := json.Marshal(map[string]interface{}{
			"name":       key.Name,
			"permission": string(key.Permission),
		})
		_ = s.auditRepo.Insert(ctx, &content.AuditLog{
			ID:          uuid.New().String(),
			Actor:       mctx.Actor,
			ActorType:   mctx.ActorType,
			Operation:   content.OpCreate,
			Resource:    "cms_api_keys",
			ResourceID:  key.ID,
			RequestID:   mctx.RequestID,
			Timestamp:   time.Now(),
			ChangesJSON: string(changes),
		})
	}

	return rawToken, key, nil
}

// RevokeKey revokes an API key and logs to audit repository.
func (s *Service) RevokeKey(ctx context.Context, id string, mctx content.MutationContext) error {
	if err := s.repo.Revoke(ctx, id); err != nil {
		return err
	}

	if s.auditRepo != nil {
		changes, _ := json.Marshal(map[string]interface{}{
			"is_active": false,
		})
		_ = s.auditRepo.Insert(ctx, &content.AuditLog{
			ID:          uuid.New().String(),
			Actor:       mctx.Actor,
			ActorType:   mctx.ActorType,
			Operation:   content.OpUpdate,
			Resource:    "cms_api_keys",
			ResourceID:  id,
			RequestID:   mctx.RequestID,
			Timestamp:   time.Now(),
			ChangesJSON: string(changes),
		})
	}

	return nil
}

// ListKeys returns all registered API keys.
func (s *Service) ListKeys(ctx context.Context) ([]*APIKey, error) {
	return s.repo.List(ctx)
}

// ValidateToken validates the given token string against stored hashes and checks method permissions.
func (s *Service) ValidateToken(ctx context.Context, token string, method string) (*APIKey, error) {
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}

	hash := HashToken(token)
	key, err := s.repo.GetByHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	if !key.IsActive {
		return nil, fmt.Errorf("api key has been revoked")
	}

	if !key.CanAccessMethod(method) {
		return nil, fmt.Errorf("api key does not have permission for method %s", method)
	}

	return key, nil
}
