package main

import (
	"context"
	"fmt"
	"log"

	"github.com/yutapok/gohcms/pkg/content"
	"github.com/yutapok/gohcms/pkg/introspection"
	"github.com/yutapok/gohcms/pkg/schema"
	"github.com/yutapok/gohcms/pkg/validator"
)

// Standalone in-memory repositories for demonstrating the core library without external DB
type memoryRepo struct {
	records map[string]*content.Record
}

func (m *memoryRepo) Get(ctx context.Context, def *schema.ResourceDefinition, id string) (*content.Record, error) {
	r, exists := m.records[id]
	if !exists {
		return nil, fmt.Errorf("record '%s' not found", id)
	}
	cp := *r
	cp.Data = make(map[string]interface{})
	for k, v := range r.Data {
		cp.Data[k] = v
	}
	return &cp, nil
}

func (m *memoryRepo) List(ctx context.Context, def *schema.ResourceDefinition, filter content.ContentFilter, pagination content.Pagination) ([]*content.Record, int64, error) {
	var list []*content.Record
	for _, r := range m.records {
		if filter.Status != nil && r.Status != *filter.Status {
			continue
		}
		list = append(list, r)
	}
	return list, int64(len(list)), nil
}

func (m *memoryRepo) Create(ctx context.Context, def *schema.ResourceDefinition, record *content.Record) (*content.Record, error) {
	m.records[record.ID] = record
	return record, nil
}

func (m *memoryRepo) Update(ctx context.Context, def *schema.ResourceDefinition, record *content.Record) (*content.Record, error) {
	m.records[record.ID] = record
	return record, nil
}

func (m *memoryRepo) Delete(ctx context.Context, def *schema.ResourceDefinition, id string) error {
	delete(m.records, id)
	return nil
}

type memoryAuditRepo struct {
	logs []*content.AuditLog
}

func (m *memoryAuditRepo) Insert(ctx context.Context, log *content.AuditLog) error {
	m.logs = append(m.logs, log)
	return nil
}

func (m *memoryAuditRepo) List(ctx context.Context, resource string, resourceID string) ([]*content.AuditLog, error) {
	return m.logs, nil
}

type memoryRevRepo struct {
	revisions []*content.Revision
}

func (m *memoryRevRepo) Insert(ctx context.Context, rev *content.Revision) error {
	m.revisions = append(m.revisions, rev)
	return nil
}

func (m *memoryRevRepo) Get(ctx context.Context, resource string, resourceID string, version int64) (*content.Revision, error) {
	for _, r := range m.revisions {
		if r.Resource == resource && r.ResourceID == resourceID && r.Version == version {
			return r, nil
		}
	}
	return nil, fmt.Errorf("revision not found")
}

func (m *memoryRevRepo) List(ctx context.Context, resource string, resourceID string) ([]*content.Revision, error) {
	return m.revisions, nil
}

type memoryUOW struct {
	repo  *memoryRepo
	audit *memoryAuditRepo
	rev   *memoryRevRepo
}

func (u *memoryUOW) Execute(ctx context.Context, fn func(repo content.ContentRepository, auditRepo content.AuditRepository, revRepo content.RevisionRepository) error) error {
	return fn(u.repo, u.audit, u.rev)
}

func main() {
	ctx := context.Background()

	// 1. Load Resource Definition from YAML
	def, err := schema.ParseFile("examples/article/article.yaml")
	if err != nil {
		log.Fatalf("failed to parse resource definition: %v", err)
	}
	fmt.Printf("Loaded resource: %s (table: %s)\n", def.Resource, def.Storage.Table)

	// 2. Validate Schema against Mock Database Schema
	mockDB := introspection.NewDatabaseSchema()
	mockDB.AddTable(introspection.TableSchema{
		Name: "articles",
		Columns: map[string]introspection.ColumnSchema{
			"id":            {Name: "id", DataType: "uuid", UDTName: "uuid", IsNullable: false},
			"title":          {Name: "title", DataType: "character varying", UDTName: "varchar", IsNullable: false},
			"body":           {Name: "body", DataType: "text", UDTName: "text", IsNullable: true},
			"cover_image_id": {Name: "cover_image_id", DataType: "uuid", UDTName: "uuid", IsNullable: true},
			"category_id":    {Name: "category_id", DataType: "uuid", UDTName: "uuid", IsNullable: true},
			"depends_on_id": {Name: "depends_on_id", DataType: "uuid", UDTName: "uuid", IsNullable: true},
			"published_at":  {Name: "published_at", DataType: "timestamp with time zone", UDTName: "timestamptz", IsNullable: true},
			"finished_at":   {Name: "finished_at", DataType: "timestamp with time zone", UDTName: "timestamptz", IsNullable: true},
			"cms_status":    {Name: "cms_status", DataType: "text", UDTName: "text", IsNullable: false},
			"cms_version":   {Name: "cms_version", DataType: "bigint", UDTName: "int8", IsNullable: false},
		},
	})

	v := validator.New()
	valResult := v.Validate(def, mockDB)
	fmt.Println("\n=== Schema Validation ===")
	fmt.Print(valResult.FormatReport())

	// 3. Initialize Core Content Service
	repo := &memoryRepo{records: make(map[string]*content.Record)}
	audit := &memoryAuditRepo{}
	rev := &memoryRevRepo{}
	uow := &memoryUOW{repo: repo, audit: audit, rev: rev}
	svc := content.NewService(uow, repo, audit, rev)

	mctx := content.MutationContext{
		Actor:     "editor-yuta",
		ActorType: content.ActorTypeUser,
		RequestID: "req-demo-1",
	}

	fmt.Println("\n=== Content Service: 3-State Lifecycle & Dependency Flow ===")

	// Step A: Create Prerequisite Article (Chapter 1) -> status: draft
	ch1, err := svc.Create(ctx, def, map[string]interface{}{
		"id":    "art-ch1",
		"title": "Chapter 1: The Beginning",
		"body":  "Introductory content...",
	}, mctx)
	if err != nil {
		log.Fatalf("failed to create ch1: %v", err)
	}
	fmt.Printf("[1] Created Chapter 1: ID=%s, Status=%s, Version=%d\n", ch1.ID, ch1.Status, ch1.Version)

	// Step B: Create Dependent Article (Chapter 2, depends_on: Chapter 1) -> status: draft
	ch2, err := svc.Create(ctx, def, map[string]interface{}{
		"id":         "art-ch2",
		"title":      "Chapter 2: The Adventure",
		"body":       "Next chapter content...",
		"depends_on": "art-ch1",
	}, mctx)
	if err != nil {
		log.Fatalf("failed to create ch2: %v", err)
	}
	fmt.Printf("[2] Created Chapter 2 (depends_on: Chapter 1): ID=%s, Status=%s, Version=%d\n", ch2.ID, ch2.Status, ch2.Version)

	// Step C: Try to publish Chapter 2 while Chapter 1 is still draft -> Expect validation error
	fmt.Printf("[3] Attempting to publish Chapter 2 before Chapter 1 is published...\n")
	_, err = svc.Publish(ctx, def, "art-ch2", mctx)
	if err != nil {
		fmt.Printf("    -> Guardrail caught error as expected: %v\n", err)
	} else {
		log.Fatal("ERROR: publish should have failed but succeeded!")
	}

	// Step D: Publish Chapter 1 -> status: published
	ch1Pub, err := svc.Publish(ctx, def, "art-ch1", mctx)
	if err != nil {
		log.Fatalf("failed to publish ch1: %v", err)
	}
	fmt.Printf("[4] Published Chapter 1: Status=%s, Version=%d\n", ch1Pub.Status, ch1Pub.Version)

	// Step E: Now publish Chapter 2 -> status: published
	ch2Pub, err := svc.Publish(ctx, def, "art-ch2", mctx)
	if err != nil {
		log.Fatalf("failed to publish ch2: %v", err)
	}
	fmt.Printf("[5] Published Chapter 2: Status=%s, Version=%d\n", ch2Pub.Status, ch2Pub.Version)

	// Step F: Finish Chapter 1 (End of campaign/publication) -> status: finished
	ch1Fin, err := svc.Finish(ctx, def, "art-ch1", mctx)
	if err != nil {
		log.Fatalf("failed to finish ch1: %v", err)
	}
	fmt.Printf("[6] Finished Chapter 1: Status=%s, Version=%d (Archived/Ended)\n", ch1Fin.Status, ch1Fin.Version)

	// Step G: Inspect Audit Logs
	fmt.Println("\n=== Audit Trail Log ===")
	logs, _ := audit.List(ctx, "article", "")
	for i, l := range logs {
		fmt.Printf("  [%d] op=%-9s resource=%-8s id=%-8s actor=%-12s\n", i+1, l.Operation, l.Resource, l.ResourceID, l.Actor)
	}

	// Step H: Inspect Revision History for Chapter 1
	fmt.Println("\n=== Revision History for Chapter 1 ===")
	revs, _ := rev.List(ctx, "article", "art-ch1")
	for i, r := range revs {
		fmt.Printf("  [%d] version=%d actor=%-12s snapshot=%s\n", i+1, r.Version, r.Actor, r.SnapshotJSON)
	}

	fmt.Println("\n🎉 All 3-State Lifecycle and Dependency operations succeeded smoothly!")
}
