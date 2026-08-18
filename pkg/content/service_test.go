package content_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/yutapok/gohcms/pkg/content"
	"github.com/yutapok/gohcms/pkg/schema"
)

// In-memory mock repositories for testing
type mockRepo struct {
	records map[string]*content.Record
}

func newMockRepo() *mockRepo {
	return &mockRepo{records: make(map[string]*content.Record)}
}

func (m *mockRepo) Get(ctx context.Context, def *schema.ResourceDefinition, id string) (*content.Record, error) {
	r, exists := m.records[id]
	if !exists {
		return nil, fmt.Errorf("record not found")
	}
	// return a copy
	copied := *r
	copied.Data = make(map[string]interface{})
	for k, v := range r.Data {
		copied.Data[k] = v
	}
	return &copied, nil
}

func (m *mockRepo) List(ctx context.Context, def *schema.ResourceDefinition, filter content.ContentFilter, pagination content.Pagination) ([]*content.Record, int64, error) {
	var list []*content.Record
	for _, r := range m.records {
		if filter.Status != nil && r.Status != *filter.Status {
			continue
		}
		list = append(list, r)
	}
	return list, int64(len(list)), nil
}

func (m *mockRepo) Create(ctx context.Context, def *schema.ResourceDefinition, record *content.Record) (*content.Record, error) {
	if record.ID == "" {
		record.ID = fmt.Sprintf("rec-%d", len(m.records)+1)
	}
	m.records[record.ID] = record
	return record, nil
}

func (m *mockRepo) Update(ctx context.Context, def *schema.ResourceDefinition, record *content.Record) (*content.Record, error) {
	m.records[record.ID] = record
	return record, nil
}

func (m *mockRepo) Delete(ctx context.Context, def *schema.ResourceDefinition, id string) error {
	delete(m.records, id)
	return nil
}

type mockAuditRepo struct {
	logs []*content.AuditLog
}

func (m *mockAuditRepo) Insert(ctx context.Context, log *content.AuditLog) error {
	m.logs = append(m.logs, log)
	return nil
}

func (m *mockAuditRepo) List(ctx context.Context, resource string, resourceID string) ([]*content.AuditLog, error) {
	return m.logs, nil
}

type mockRevRepo struct {
	revisions []*content.Revision
}

func (m *mockRevRepo) Insert(ctx context.Context, rev *content.Revision) error {
	m.revisions = append(m.revisions, rev)
	return nil
}

func (m *mockRevRepo) Get(ctx context.Context, resource string, resourceID string, version int64) (*content.Revision, error) {
	for _, r := range m.revisions {
		if r.Resource == resource && r.ResourceID == resourceID && r.Version == version {
			return r, nil
		}
	}
	return nil, fmt.Errorf("revision not found")
}

func (m *mockRevRepo) List(ctx context.Context, resource string, resourceID string) ([]*content.Revision, error) {
	return m.revisions, nil
}

type mockUOW struct {
	repo  *mockRepo
	audit *mockAuditRepo
	rev   *mockRevRepo
}

func (u *mockUOW) Execute(ctx context.Context, fn func(repo content.ContentRepository, auditRepo content.AuditRepository, revRepo content.RevisionRepository) error) error {
	return fn(u.repo, u.audit, u.rev)
}

func setupTestService() (*content.ContentService, *mockRepo, *mockAuditRepo, *mockRevRepo, *schema.ResourceDefinition) {
	repo := newMockRepo()
	audit := &mockAuditRepo{}
	rev := &mockRevRepo{}
	uow := &mockUOW{repo: repo, audit: audit, rev: rev}

	svc := content.NewService(uow, repo, audit, rev)

	def := &schema.ResourceDefinition{
		Resource: "article",
		Storage:  schema.StorageConfig{Table: "articles"},
		Lifecycle: schema.LifecycleConfig{
			Mode:          schema.LifecycleModeManaged,
			StatusColumn:  "cms_status",
			VersionColumn: "cms_version",
		},
		Fields: map[string]schema.FieldDefinition{
			"title": {Type: schema.FieldTypeString, Column: "title", Required: true},
			"body":  {Type: schema.FieldTypeText, Column: "body"},
		},
	}

	return svc, repo, audit, rev, def
}

func TestService_CreateAndGet(t *testing.T) {
	svc, _, audit, rev, def := setupTestService()
	ctx := context.Background()
	mctx := content.MutationContext{Actor: "user-1", ActorType: content.ActorTypeUser, RequestID: "req-1"}

	data := map[string]interface{}{
		"id":    "art-1",
		"title": "First Article",
		"body":  "Hello World",
	}

	record, err := svc.Create(ctx, def, data, mctx)
	if err != nil {
		t.Fatalf("unexpected error creating record: %v", err)
	}

	if record.ID != "art-1" {
		t.Errorf("expected ID 'art-1', got '%s'", record.ID)
	}
	if record.Status != content.StatusDraft {
		t.Errorf("expected initial status 'draft', got '%s'", record.Status)
	}
	if record.Version != 1 {
		t.Errorf("expected version 1, got %d", record.Version)
	}

	// Verify revision and audit log
	if len(rev.revisions) != 1 {
		t.Errorf("expected 1 revision, got %d", len(rev.revisions))
	}
	if len(audit.logs) != 1 || audit.logs[0].Operation != content.OpCreate {
		t.Errorf("expected 1 create audit log, got %v", audit.logs)
	}
}

// 3-State Lifecycle test: draft -> published -> finished -> draft
func TestService_LifecycleTransitions_3State(t *testing.T) {
	svc, _, audit, rev, def := setupTestService()
	ctx := context.Background()
	mctx := content.MutationContext{Actor: "editor", ActorType: content.ActorTypeUser}

	// 1. Create (draft)
	rec, err := svc.Create(ctx, def, map[string]interface{}{"id": "art-1", "title": "Article 1"}, mctx)
	if err != nil {
		t.Fatalf("failed to create: %v", err)
	}
	if rec.Status != content.StatusDraft {
		t.Fatalf("expected draft, got %s", rec.Status)
	}

	// 2. Publish (draft -> published)
	published, err := svc.Publish(ctx, def, "art-1", mctx)
	if err != nil {
		t.Fatalf("failed to publish: %v", err)
	}
	if published.Status != content.StatusPublished {
		t.Errorf("expected published status, got %s", published.Status)
	}
	if published.Version != 2 {
		t.Errorf("expected version 2, got %d", published.Version)
	}

	// 3. Finish (published -> finished)
	finished, err := svc.Finish(ctx, def, "art-1", mctx)
	if err != nil {
		t.Fatalf("failed to finish: %v", err)
	}
	if finished.Status != content.StatusFinished {
		t.Errorf("expected finished status, got %s", finished.Status)
	}
	if finished.Version != 3 {
		t.Errorf("expected version 3, got %d", finished.Version)
	}

	// 4. Invalid direct transition: finished -> published
	_, err = svc.Publish(ctx, def, "art-1", mctx)
	if err == nil {
		t.Error("expected error for invalid transition finished -> published, but succeeded")
	}

	// 5. Re-open (finished -> draft)
	reopened, err := svc.Unpublish(ctx, def, "art-1", mctx)
	if err != nil {
		t.Fatalf("failed to unpublish/reopen back to draft: %v", err)
	}
	if reopened.Status != content.StatusDraft {
		t.Errorf("expected draft status, got %s", reopened.Status)
	}

	// Verify revision history length
	if len(rev.revisions) != 4 {
		t.Errorf("expected 4 revisions, got %d", len(rev.revisions))
	}
	if len(audit.logs) != 4 {
		t.Errorf("expected 4 audit logs, got %d", len(audit.logs))
	}
}

// Dependency validation test
func TestService_DependencyValidation(t *testing.T) {
	svc, _, _, _, def := setupTestService()
	ctx := context.Background()
	mctx := content.MutationContext{Actor: "editor", ActorType: content.ActorTypeUser}

	// 1. Create Prerequisite Article 1 (draft)
	_, err := svc.Create(ctx, def, map[string]interface{}{"id": "dep-1", "title": "Prerequisite Article"}, mctx)
	if err != nil {
		t.Fatalf("failed to create dep-1: %v", err)
	}

	// 2. Create Dependent Article 2 (depends_on: dep-1)
	_, err = svc.Create(ctx, def, map[string]interface{}{"id": "dep-2", "title": "Dependent Article", "depends_on": "dep-1"}, mctx)
	if err != nil {
		t.Fatalf("failed to create dep-2: %v", err)
	}

	// 3. Try to publish Article 2 when Prerequisite is still draft -> Should fail
	_, err = svc.Publish(ctx, def, "dep-2", mctx)
	if err == nil {
		t.Fatal("expected publish to fail because dependency is still in draft, but it succeeded")
	}

	// 4. Publish Prerequisite Article 1 -> Should succeed
	_, err = svc.Publish(ctx, def, "dep-1", mctx)
	if err != nil {
		t.Fatalf("failed to publish dep-1: %v", err)
	}

	// 5. Now publish Dependent Article 2 -> Should succeed
	pub2, err := svc.Publish(ctx, def, "dep-2", mctx)
	if err != nil {
		t.Fatalf("failed to publish dep-2 after prerequisite was published: %v", err)
	}
	if pub2.Status != content.StatusPublished {
		t.Errorf("expected published status for dep-2, got %s", pub2.Status)
	}
}

func TestService_RequiredFieldValidation(t *testing.T) {
	svc, _, _, _, def := setupTestService()
	ctx := context.Background()
	mctx := content.MutationContext{Actor: "user"}

	// Missing title field (which is required)
	_, err := svc.Create(ctx, def, map[string]interface{}{"body": "No title"}, mctx)
	if err == nil {
		t.Fatal("expected validation error for missing required field, got nil")
	}
}
