package media

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/yutapok/gohcms/pkg/content"
)

// Service coordinates media uploads, physical storage, metadata persistence, and audit logging.
type Service struct {
	storage   Storage
	repo      MediaRepository
	auditRepo content.AuditRepository
}

// NewService creates a new Media Service.
func NewService(storage Storage, repo MediaRepository, auditRepo content.AuditRepository) *Service {
	return &Service{
		storage:   storage,
		repo:      repo,
		auditRepo: auditRepo,
	}
}

// Upload saves the physical file and metadata, logging to audit trail.
func (s *Service) Upload(ctx context.Context, filename string, mimeType string, sizeBytes int64, r io.Reader, mctx content.MutationContext) (*Media, error) {
	if filename == "" {
		return nil, fmt.Errorf("filename is required")
	}

	mediaID := uuid.New().String()
	storagePath, err := s.storage.Save(ctx, mediaID, filename, r)
	if err != nil {
		return nil, fmt.Errorf("failed to save media file: %w", err)
	}

	item := &Media{
		ID:        mediaID,
		Filename:  filename,
		Filepath:  storagePath,
		MimeType:  mimeType,
		SizeBytes: sizeBytes,
		CreatedAt: time.Now(),
	}

	if err := s.repo.Insert(ctx, item); err != nil {
		_ = s.storage.Delete(ctx, storagePath)
		return nil, fmt.Errorf("failed to save media metadata: %w", err)
	}

	item.FormatURL()

	if s.auditRepo != nil {
		changes, _ := json.Marshal(map[string]interface{}{
			"filename":   item.Filename,
			"mime_type":  item.MimeType,
			"size_bytes": item.SizeBytes,
		})
		_ = s.auditRepo.Insert(ctx, &content.AuditLog{
			ID:          uuid.New().String(),
			Actor:       mctx.Actor,
			ActorType:   mctx.ActorType,
			Operation:   content.OpCreate,
			Resource:    "cms_media",
			ResourceID:  item.ID,
			RequestID:   mctx.RequestID,
			Timestamp:   time.Now(),
			ChangesJSON: string(changes),
		})
	}

	return item, nil
}

// Get retrieves media metadata by ID.
func (s *Service) Get(ctx context.Context, id string) (*Media, error) {
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	item.FormatURL()
	return item, nil
}

// List returns all uploaded media items.
func (s *Service) List(ctx context.Context) ([]*Media, error) {
	list, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range list {
		m.FormatURL()
	}
	return list, nil
}

// Delete removes both physical file and metadata, logging to audit trail.
func (s *Service) Delete(ctx context.Context, id string, mctx content.MutationContext) error {
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}

	if err := s.storage.Delete(ctx, item.Filepath); err != nil {
		return fmt.Errorf("failed to delete media file from storage: %w", err)
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete media metadata: %w", err)
	}

	if s.auditRepo != nil {
		changes, _ := json.Marshal(map[string]interface{}{
			"filename": item.Filename,
		})
		_ = s.auditRepo.Insert(ctx, &content.AuditLog{
			ID:          uuid.New().String(),
			Actor:       mctx.Actor,
			ActorType:   mctx.ActorType,
			Operation:   content.OpDelete,
			Resource:    "cms_media",
			ResourceID:  id,
			RequestID:   mctx.RequestID,
			Timestamp:   time.Now(),
			ChangesJSON: string(changes),
		})
	}

	return nil
}

// OpenFile retrieves the readable stream for media delivery.
func (s *Service) OpenFile(ctx context.Context, id string) (io.ReadSeekCloser, *Media, error) {
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	rsc, err := s.storage.Open(ctx, item.Filepath)
	if err != nil {
		return nil, nil, err
	}

	item.FormatURL()
	return rsc, item, nil
}
