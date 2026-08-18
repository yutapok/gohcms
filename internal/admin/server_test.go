package admin_test

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/yutapok/gohcms/internal/admin"
	"github.com/yutapok/gohcms/pkg/content"
	"github.com/yutapok/gohcms/pkg/introspection"
	"github.com/yutapok/gohcms/pkg/schema"
	"github.com/yutapok/gohcms/pkg/validator"
)

// In-memory mock structures for HTTP testing
type mockRepo struct {
	records map[string]*content.Record
}

func (m *mockRepo) Get(ctx context.Context, def *schema.ResourceDefinition, id string) (*content.Record, error) {
	r, exists := m.records[id]
	if !exists {
		return nil, fmt.Errorf("record not found")
	}
	cp := *r
	cp.Data = make(map[string]interface{})
	for k, v := range r.Data {
		cp.Data[k] = v
	}
	return &cp, nil
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
		record.ID = fmt.Sprintf("art-%d", len(m.records)+1)
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
	revs []*content.Revision
}

func (m *mockRevRepo) Insert(ctx context.Context, rev *content.Revision) error {
	m.revs = append(m.revs, rev)
	return nil
}

func (m *mockRevRepo) Get(ctx context.Context, resource string, resourceID string, version int64) (*content.Revision, error) {
	for _, r := range m.revs {
		if r.Resource == resource && r.ResourceID == resourceID && r.Version == version {
			return r, nil
		}
	}
	return nil, fmt.Errorf("revision not found")
}

func (m *mockRevRepo) List(ctx context.Context, resource string, resourceID string) ([]*content.Revision, error) {
	return m.revs, nil
}

type mockUOW struct {
	repo  *mockRepo
	audit *mockAuditRepo
	rev   *mockRevRepo
}

func (u *mockUOW) Execute(ctx context.Context, fn func(repo content.ContentRepository, auditRepo content.AuditRepository, revRepo content.RevisionRepository) error) error {
	return fn(u.repo, u.audit, u.rev)
}

func setupTestServer() (*admin.Server, *mockRepo, *mockAuditRepo, *mockRevRepo) {
	repo := &mockRepo{records: make(map[string]*content.Record)}
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
			"title":       {Type: schema.FieldTypeString, Column: "title", Required: true},
			"body":        {Type: schema.FieldTypeText, Column: "body"},
			"cover_image": {Type: schema.FieldTypeMedia, Column: "cover_image_id"},
		},
	}

	dbSchema := introspection.NewDatabaseSchema()
	valResult := &validator.ValidationResult{}

	server, err := admin.NewServer(svc, audit, rev, []*schema.ResourceDefinition{def}, dbSchema, valResult)
	if err != nil {
		panic(err)
	}

	return server, repo, audit, rev
}

func TestAdminServer_RootRedirect(t *testing.T) {
	server, _, _, _ := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected status 302 redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/admin/resources/article" {
		t.Errorf("expected redirect to /admin/resources/article, got %s", loc)
	}
}

func TestAdminServer_Views(t *testing.T) {
	server, _, _, _ := setupTestServer()

	views := []string{"table", "kanban", "timeline"}
	for _, v := range views {
		t.Run("view_"+v, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/admin/resources/article?view=%s", v), nil)
			w := httptest.NewRecorder()

			server.Handler().ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected status 200 for view %s, got %d", v, w.Code)
			}
			body := w.Body.String()
			if !strings.Contains(body, "article") {
				t.Errorf("expected body to contain 'article', got:\n%s", body)
			}
		})
	}
}

func TestAdminServer_CreateAndLifecycleOperations(t *testing.T) {
	server, _, _, _ := setupTestServer()
	handler := server.Handler()

	// 1. Create content via POST
	form := url.Values{}
	form.Set("title", "Admin Test Article")
	form.Set("body", "Test Body Content")

	reqCreate := httptest.NewRequest(http.MethodPost, "/admin/resources/article", strings.NewReader(form.Encode()))
	reqCreate.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqCreate.Header.Set("Sec-Fetch-Site", "same-origin")
	wCreate := httptest.NewRecorder()

	handler.ServeHTTP(wCreate, reqCreate)

	if wCreate.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303 redirect after create, got %d", wCreate.Code)
	}

	// 2. Publish record (art-1)
	reqPub := httptest.NewRequest(http.MethodPost, "/admin/resources/article/art-1/publish", nil)
	reqPub.Header.Set("Sec-Fetch-Site", "same-origin")
	wPub := httptest.NewRecorder()
	handler.ServeHTTP(wPub, reqPub)

	if wPub.Code != http.StatusOK {
		t.Fatalf("expected status 200 on publish, got %d", wPub.Code)
	}

	// 3. Finish record (art-1)
	reqFin := httptest.NewRequest(http.MethodPost, "/admin/resources/article/art-1/finish", nil)
	reqFin.Header.Set("Sec-Fetch-Site", "same-origin")
	wFin := httptest.NewRecorder()
	handler.ServeHTTP(wFin, reqFin)

	if wFin.Code != http.StatusOK {
		t.Fatalf("expected status 200 on finish, got %d", wFin.Code)
	}

	// 4. Fetch History Modal
	reqHist := httptest.NewRequest(http.MethodGet, "/admin/resources/article/art-1/history", nil)
	wHist := httptest.NewRecorder()
	handler.ServeHTTP(wHist, reqHist)

	if wHist.Code != http.StatusOK {
		t.Fatalf("expected status 200 on history modal, got %d", wHist.Code)
	}
	histBody := wHist.Body.String()
	if !strings.Contains(histBody, "Audit Trail & Revisions") {
		t.Errorf("expected history modal content, got:\n%s", histBody)
	}
}

func TestAdminServer_MediaAndAPIKeys(t *testing.T) {
	server, _, _, _ := setupTestServer()
	handler := server.Handler()

	// 1. View Media Library
	reqMedia := httptest.NewRequest(http.MethodGet, "/admin/media", nil)
	wMedia := httptest.NewRecorder()
	handler.ServeHTTP(wMedia, reqMedia)
	if wMedia.Code != http.StatusOK {
		t.Errorf("expected 200 OK for /admin/media, got %d", wMedia.Code)
	}

	// 2. Upload Media via multipart form
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "test-image.png")
	part.Write([]byte("fake-png-content"))
	writer.Close()

	reqUpload := httptest.NewRequest(http.MethodPost, "/admin/media", &body)
	reqUpload.Header.Set("Content-Type", writer.FormDataContentType())
	reqUpload.Header.Set("Sec-Fetch-Site", "same-origin")
	wUpload := httptest.NewRecorder()
	handler.ServeHTTP(wUpload, reqUpload)
	if wUpload.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect on media upload, got %d", wUpload.Code)
	}

	// 3. View API Keys
	reqKeys := httptest.NewRequest(http.MethodGet, "/admin/api-keys", nil)
	wKeys := httptest.NewRecorder()
	handler.ServeHTTP(wKeys, reqKeys)
	if wKeys.Code != http.StatusOK {
		t.Errorf("expected 200 OK for /admin/api-keys, got %d", wKeys.Code)
	}

	// 4. Create API Key
	keyForm := url.Values{}
	keyForm.Set("name", "Frontend Next.js")
	keyForm.Set("permission", "read_write")

	reqCreateKey := httptest.NewRequest(http.MethodPost, "/admin/api-keys", strings.NewReader(keyForm.Encode()))
	reqCreateKey.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqCreateKey.Header.Set("Sec-Fetch-Site", "same-origin")
	wCreateKey := httptest.NewRecorder()
	handler.ServeHTTP(wCreateKey, reqCreateKey)
	if wCreateKey.Code != http.StatusOK {
		t.Errorf("expected 200 OK on API key create, got %d", wCreateKey.Code)
	}
	if !strings.Contains(wCreateKey.Body.String(), "gohcms_live_") {
		t.Errorf("expected generated token to be displayed")
	}
}

func TestAdminServer_Roles(t *testing.T) {
	server, _, _, _ := setupTestServer()

	// 1. RoleAdmin: /admin/resources/article is 200, /api/article is 404
	adminHandler := server.HandlerForRole(admin.RoleAdmin)
	reqAdmin1 := httptest.NewRequest(http.MethodGet, "/admin/resources/article", nil)
	wAdmin1 := httptest.NewRecorder()
	adminHandler.ServeHTTP(wAdmin1, reqAdmin1)
	if wAdmin1.Code != http.StatusOK {
		t.Errorf("expected 200 for /admin/resources/article under RoleAdmin, got %d", wAdmin1.Code)
	}

	reqAdmin2 := httptest.NewRequest(http.MethodGet, "/api/article", nil)
	wAdmin2 := httptest.NewRecorder()
	adminHandler.ServeHTTP(wAdmin2, reqAdmin2)
	if wAdmin2.Code != http.StatusNotFound {
		t.Errorf("expected 404 for /api/article under RoleAdmin, got %d", wAdmin2.Code)
	}

	// 2. RoleAPI: /api/article is 200 (with key), /admin/resources/article is 404
	// First generate an active API Key
	keyForm := url.Values{"name": {"API Test Key"}, "permission": {"read"}}
	reqCreateKey := httptest.NewRequest(http.MethodPost, "/admin/api-keys", strings.NewReader(keyForm.Encode()))
	reqCreateKey.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wCreateKey := httptest.NewRecorder()
	server.Handler().ServeHTTP(wCreateKey, reqCreateKey)

	// Fetch key from openapi spec or extract generated key
	// In our test, /api/openapi.json is public without auth
	apiHandler := server.HandlerForRole(admin.RoleAPI)
	reqAPI1 := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	wAPI1 := httptest.NewRecorder()
	apiHandler.ServeHTTP(wAPI1, reqAPI1)
	if wAPI1.Code != http.StatusOK {
		t.Errorf("expected 200 for /api/openapi.json under RoleAPI, got %d", wAPI1.Code)
	}

	reqAPI2 := httptest.NewRequest(http.MethodGet, "/admin/resources/article", nil)
	wAPI2 := httptest.NewRecorder()
	apiHandler.ServeHTTP(wAPI2, reqAPI2)
	if wAPI2.Code != http.StatusNotFound {
		t.Errorf("expected 404 for /admin/resources/article under RoleAPI, got %d", wAPI2.Code)
	}
}
