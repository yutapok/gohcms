package job_test

import (
	"context"
	"testing"
	"time"

	"github.com/yutapok/gohcms/pkg/content"
	"github.com/yutapok/gohcms/pkg/introspection"
	"github.com/yutapok/gohcms/pkg/job"
	"github.com/yutapok/gohcms/pkg/schema"
)

func TestJob_ValidateJob_Success(t *testing.T) {
	def := &schema.ResourceDefinition{
		Resource: "article",
		Storage:  schema.StorageConfig{Table: "articles"},
		Fields: map[string]schema.FieldDefinition{
			"title": {Type: schema.FieldTypeString, Column: "title", Required: true},
		},
	}

	dbSchema := introspection.NewDatabaseSchema()
	dbSchema.AddTable(introspection.TableSchema{
		Name: "articles",
		Columns: map[string]introspection.ColumnSchema{
			"title": {Name: "title", DataType: "text", UDTName: "text"},
		},
	})

	vJob := job.NewValidateJob([]*schema.ResourceDefinition{def}, dbSchema)
	code, err := vJob.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestJob_ValidateJob_Drift(t *testing.T) {
	def := &schema.ResourceDefinition{
		Resource: "article",
		Storage:  schema.StorageConfig{Table: "articles"},
		Fields: map[string]schema.FieldDefinition{
			"title": {Type: schema.FieldTypeString, Column: "title", Required: true},
		},
	}

	// Empty DB schema -> Missing table
	dbSchema := introspection.NewDatabaseSchema()

	vJob := job.NewValidateJob([]*schema.ResourceDefinition{def}, dbSchema)
	code, err := vJob.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 1 {
		t.Errorf("expected exit code 1 on drift, got %d", code)
	}
}

func TestJob_PublishScheduledJob(t *testing.T) {
	memRepo := content.NewMemoryContentRepository()
	memAudit := content.NewMemoryAuditRepository()
	memRev := content.NewMemoryRevisionRepository()
	uow := content.NewMemoryUnitOfWork(memRepo, memAudit, memRev)
	svc := content.NewService(uow, memRepo, memAudit, memRev)

	def := &schema.ResourceDefinition{
		Resource: "article",
		Storage:  schema.StorageConfig{Table: "articles"},
		Lifecycle: schema.LifecycleConfig{
			Mode:          schema.LifecycleModeManaged,
			StatusColumn:  "cms_status",
			VersionColumn: "cms_version",
		},
		Fields: map[string]schema.FieldDefinition{
			"title":        {Type: schema.FieldTypeString, Column: "title", Required: true},
			"published_at": {Type: schema.FieldTypeDateTime, Column: "published_at"},
		},
	}

	ctx := context.Background()
	mctx := content.MutationContext{Actor: "editor", ActorType: content.ActorTypeUser}

	// 1. Create a draft article with past published_at (should be published)
	pastTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	rec1, err := svc.Create(ctx, def, map[string]interface{}{
		"title":        "Past Scheduled Article",
		"published_at": pastTime,
	}, mctx)
	if err != nil {
		t.Fatalf("failed to create rec1: %v", err)
	}

	// 2. Create a draft article with future published_at (should remain draft)
	futureTime := time.Now().Add(10 * time.Hour).Format(time.RFC3339)
	rec2, err := svc.Create(ctx, def, map[string]interface{}{
		"title":        "Future Scheduled Article",
		"published_at": futureTime,
	}, mctx)
	if err != nil {
		t.Fatalf("failed to create rec2: %v", err)
	}

	// Run PublishScheduledJob
	pJob := job.NewPublishScheduledJob(svc, []*schema.ResourceDefinition{def})
	code, err := pJob.Run(ctx, nil)
	if err != nil || code != 0 {
		t.Fatalf("job failed: code=%d, err=%v", code, err)
	}

	// Check rec1 is published
	updatedRec1, _ := svc.Get(ctx, def, rec1.ID)
	if updatedRec1.Status != content.StatusPublished {
		t.Errorf("expected rec1 to be published, got %s", updatedRec1.Status)
	}

	// Check rec2 is still draft
	updatedRec2, _ := svc.Get(ctx, def, rec2.ID)
	if updatedRec2.Status != content.StatusDraft {
		t.Errorf("expected rec2 to remain draft, got %s", updatedRec2.Status)
	}

	// Check audit log for rec1 (actor='system')
	logs, _ := memAudit.List(ctx, "article", rec1.ID)
	foundSystemPublish := false
	for _, l := range logs {
		if l.Operation == content.OpPublish && l.Actor == "system" && l.ActorType == "system" {
			foundSystemPublish = true
			break
		}
	}
	if !foundSystemPublish {
		t.Error("expected audit log with operation=publish and actor=system")
	}
}

func TestJob_Registry(t *testing.T) {
	reg := job.NewRegistry()
	vJob := job.NewValidateJob(nil, nil)
	pJob := job.NewPublishScheduledJob(nil, nil)
	reg.Register(vJob)
	reg.Register(pJob)

	if len(reg.List()) != 2 {
		t.Errorf("expected 2 jobs in registry, got %d", len(reg.List()))
	}
}
