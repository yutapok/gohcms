package content

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yutapok/gohcms/pkg/schema"
)

// ContentService is the core application service handling CMS business logic and mutations.
type ContentService struct {
	uow      UnitOfWork
	repo     ContentRepository
	auditor  AuditRepository
	reviser  RevisionRepository
}

// NewService creates a new ContentService.
func NewService(uow UnitOfWork, repo ContentRepository, auditor AuditRepository, reviser RevisionRepository) *ContentService {
	return &ContentService{
		uow:     uow,
		repo:    repo,
		auditor: auditor,
		reviser: reviser,
	}
}

// Get retrieves a record by ID.
func (s *ContentService) Get(ctx context.Context, def *schema.ResourceDefinition, id string) (*Record, error) {
	if id == "" {
		return nil, fmt.Errorf("id cannot be empty")
	}
	return s.repo.Get(ctx, def, id)
}

// List retrieves records matching filter and pagination.
func (s *ContentService) List(ctx context.Context, def *schema.ResourceDefinition, filter ContentFilter, pagination Pagination) ([]*Record, int64, error) {
	return s.repo.List(ctx, def, filter, pagination)
}

// Create inserts a new record with audit logging and initial revision.
func (s *ContentService) Create(ctx context.Context, def *schema.ResourceDefinition, data map[string]interface{}, mctx MutationContext) (*Record, error) {
	// 1. Validate required fields
	if err := s.validateRequiredFields(def, data); err != nil {
		return nil, err
	}

	// 2. Initialize lifecycle status and version
	var initialStatus ContentStatus
	var initialVersion int64 = 1
	if def.Lifecycle.Mode == schema.LifecycleModeManaged {
		initialStatus = StatusDraft
	}

	record := &Record{
		Data:      data,
		Status:    initialStatus,
		Version:   initialVersion,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if idVal, ok := data["id"].(string); ok && idVal != "" {
		record.ID = idVal
	}

	var createdRecord *Record
	err := s.uow.Execute(ctx, func(txRepo ContentRepository, txAudit AuditRepository, txRev RevisionRepository) error {
		var err error
		createdRecord, err = txRepo.Create(ctx, def, record)
		if err != nil {
			return fmt.Errorf("failed to create content record: %w", err)
		}

		snapshotBytes, _ := json.Marshal(createdRecord)
		snapshotJSON := string(snapshotBytes)

		// Insert initial revision
		if txRev != nil {
			rev := &Revision{
				Resource:     def.Resource,
				ResourceID:   createdRecord.ID,
				Version:      createdRecord.Version,
				SnapshotJSON: snapshotJSON,
				CreatedAt:    time.Now(),
				Actor:        mctx.Actor,
			}
			if err := txRev.Insert(ctx, rev); err != nil {
				return fmt.Errorf("failed to record revision: %w", err)
			}
		}

		// Insert audit log
		if txAudit != nil {
			audit := &AuditLog{
				Actor:       mctx.Actor,
				ActorType:   mctx.ActorType,
				Operation:   OpCreate,
				Resource:    def.Resource,
				ResourceID:  createdRecord.ID,
				RequestID:   mctx.RequestID,
				Timestamp:   time.Now(),
				ChangesJSON: snapshotJSON,
			}
			if err := txAudit.Insert(ctx, audit); err != nil {
				return fmt.Errorf("failed to record audit log: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return createdRecord, nil
}

// Update updates an existing record, increments version, and records audit and revision.
func (s *ContentService) Update(ctx context.Context, def *schema.ResourceDefinition, id string, data map[string]interface{}, mctx MutationContext) (*Record, error) {
	existing, err := s.repo.Get(ctx, def, id)
	if err != nil {
		return nil, fmt.Errorf("failed to find record '%s': %w", id, err)
	}

	if err := s.validateRequiredFields(def, data); err != nil {
		return nil, err
	}

	// Merge data
	for k, v := range data {
		existing.Data[k] = v
	}
	existing.Version++
	existing.UpdatedAt = time.Now()

	var updatedRecord *Record
	err = s.uow.Execute(ctx, func(txRepo ContentRepository, txAudit AuditRepository, txRev RevisionRepository) error {
		var err error
		updatedRecord, err = txRepo.Update(ctx, def, existing)
		if err != nil {
			return fmt.Errorf("failed to update content record: %w", err)
		}

		snapshotBytes, _ := json.Marshal(updatedRecord)
		snapshotJSON := string(snapshotBytes)

		if txRev != nil {
			rev := &Revision{
				Resource:     def.Resource,
				ResourceID:   updatedRecord.ID,
				Version:      updatedRecord.Version,
				SnapshotJSON: snapshotJSON,
				CreatedAt:    time.Now(),
				Actor:        mctx.Actor,
			}
			if err := txRev.Insert(ctx, rev); err != nil {
				return fmt.Errorf("failed to record revision: %w", err)
			}
		}

		if txAudit != nil {
			audit := &AuditLog{
				Actor:       mctx.Actor,
				ActorType:   mctx.ActorType,
				Operation:   OpUpdate,
				Resource:    def.Resource,
				ResourceID:  updatedRecord.ID,
				RequestID:   mctx.RequestID,
				Timestamp:   time.Now(),
				ChangesJSON: snapshotJSON,
			}
			if err := txAudit.Insert(ctx, audit); err != nil {
				return fmt.Errorf("failed to record audit log: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return updatedRecord, nil
}

// Publish transitions a record to 'published' state with dependency checking.
func (s *ContentService) Publish(ctx context.Context, def *schema.ResourceDefinition, id string, mctx MutationContext) (*Record, error) {
	return s.transitionStatus(ctx, def, id, StatusPublished, OpPublish, mctx, true)
}

// Unpublish transitions a record back to 'draft' state.
func (s *ContentService) Unpublish(ctx context.Context, def *schema.ResourceDefinition, id string, mctx MutationContext) (*Record, error) {
	return s.transitionStatus(ctx, def, id, StatusDraft, OpUnpublish, mctx, false)
}

// Finish transitions a record to 'finished' state (content ended/archived).
func (s *ContentService) Finish(ctx context.Context, def *schema.ResourceDefinition, id string, mctx MutationContext) (*Record, error) {
	return s.transitionStatus(ctx, def, id, StatusFinished, OpFinish, mctx, false)
}

// Delete removes a record and records audit log.
func (s *ContentService) Delete(ctx context.Context, def *schema.ResourceDefinition, id string, mctx MutationContext) error {
	existing, err := s.repo.Get(ctx, def, id)
	if err != nil {
		return fmt.Errorf("failed to find record '%s': %w", id, err)
	}

	return s.uow.Execute(ctx, func(txRepo ContentRepository, txAudit AuditRepository, txRev RevisionRepository) error {
		if err := txRepo.Delete(ctx, def, id); err != nil {
			return fmt.Errorf("failed to delete content record: %w", err)
		}

		if txAudit != nil {
			snapshotBytes, _ := json.Marshal(existing)
			audit := &AuditLog{
				Actor:       mctx.Actor,
				ActorType:   mctx.ActorType,
				Operation:   OpDelete,
				Resource:    def.Resource,
				ResourceID:  id,
				RequestID:   mctx.RequestID,
				Timestamp:   time.Now(),
				ChangesJSON: string(snapshotBytes),
			}
			if err := txAudit.Insert(ctx, audit); err != nil {
				return fmt.Errorf("failed to record audit log: %w", err)
			}
		}

		return nil
	})
}

// transitionStatus executes a state machine transition.
func (s *ContentService) transitionStatus(ctx context.Context, def *schema.ResourceDefinition, id string, targetStatus ContentStatus, op AuditOperation, mctx MutationContext, checkDeps bool) (*Record, error) {
	if def.Lifecycle.Mode != schema.LifecycleModeManaged {
		return nil, fmt.Errorf("resource '%s' does not have managed lifecycle enabled", def.Resource)
	}

	existing, err := s.repo.Get(ctx, def, id)
	if err != nil {
		return nil, fmt.Errorf("failed to find record '%s': %w", id, err)
	}

	if err := CanTransition(existing.Status, targetStatus); err != nil {
		return nil, err
	}

	// If publishing, verify dependencies (e.g. depends_on must be published)
	if checkDeps {
		if err := s.checkDependencies(ctx, def, existing); err != nil {
			return nil, err
		}
	}

	existing.Status = targetStatus
	existing.Version++
	existing.UpdatedAt = time.Now()

	var updatedRecord *Record
	err = s.uow.Execute(ctx, func(txRepo ContentRepository, txAudit AuditRepository, txRev RevisionRepository) error {
		var err error
		updatedRecord, err = txRepo.Update(ctx, def, existing)
		if err != nil {
			return fmt.Errorf("failed to update record status: %w", err)
		}

		snapshotBytes, _ := json.Marshal(updatedRecord)
		snapshotJSON := string(snapshotBytes)

		if txRev != nil {
			rev := &Revision{
				Resource:     def.Resource,
				ResourceID:   updatedRecord.ID,
				Version:      updatedRecord.Version,
				SnapshotJSON: snapshotJSON,
				CreatedAt:    time.Now(),
				Actor:        mctx.Actor,
			}
			if err := txRev.Insert(ctx, rev); err != nil {
				return fmt.Errorf("failed to record revision: %w", err)
			}
		}

		if txAudit != nil {
			audit := &AuditLog{
				Actor:       mctx.Actor,
				ActorType:   mctx.ActorType,
				Operation:   op,
				Resource:    def.Resource,
				ResourceID:  updatedRecord.ID,
				RequestID:   mctx.RequestID,
				Timestamp:   time.Now(),
				ChangesJSON: snapshotJSON,
			}
			if err := txAudit.Insert(ctx, audit); err != nil {
				return fmt.Errorf("failed to record audit log: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return updatedRecord, nil
}

func (s *ContentService) validateRequiredFields(def *schema.ResourceDefinition, data map[string]interface{}) error {
	for name, field := range def.Fields {
		if field.Required {
			val, exists := data[name]
			if !exists || val == nil || val == "" {
				return fmt.Errorf("field '%s' is required for resource '%s'", name, def.Resource)
			}
		}
	}
	return nil
}

func (s *ContentService) checkDependencies(ctx context.Context, def *schema.ResourceDefinition, record *Record) error {
	// If a field named 'depends_on' or a reference field is configured, check that the prerequisite is published
	if depVal, ok := record.GetField("depends_on"); ok && depVal != nil {
		depID, ok := depVal.(string)
		if ok && depID != "" {
			depRecord, err := s.repo.Get(ctx, def, depID)
			if err != nil {
				return fmt.Errorf("prerequisite dependency '%s' not found: %w", depID, err)
			}
			if depRecord.Status != StatusPublished {
				return fmt.Errorf("cannot publish '%s': prerequisite dependency '%s' is not published (current status: '%s')", record.ID, depID, depRecord.Status)
			}
		}
	}
	return nil
}
