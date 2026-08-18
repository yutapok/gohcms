package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yutapok/gohcms/pkg/api"
	"github.com/yutapok/gohcms/pkg/content"
	"github.com/yutapok/gohcms/pkg/schema"
)

func setupTestAPI() (http.Handler, *schema.ResourceDefinition) {
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
			"title": {Type: schema.FieldTypeString, Column: "title", Required: true},
			"body":  {Type: schema.FieldTypeText, Column: "body"},
		},
	}

	handler := api.NewHandler(svc, []*schema.ResourceDefinition{def})
	return handler, def
}

func TestAPI_OpenAPISpec(t *testing.T) {
	handler, _ := setupTestAPI()
	req := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var doc map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}
	if doc["openapi"] != "3.1.0" {
		t.Errorf("expected openapi 3.1.0, got %v", doc["openapi"])
	}
}

func TestAPI_FullCRUDAndLifecycleFlow(t *testing.T) {
	handler, _ := setupTestAPI()

	// 1. Create Article (POST /api/article)
	createBody := []byte(`{"id": "art-1", "title": "API Test Article", "body": "Testing Headless API"}`)
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/article", bytes.NewReader(createBody))
	wCreate := httptest.NewRecorder()
	handler.ServeHTTP(wCreate, reqCreate)

	if wCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d. Body: %s", wCreate.Code, wCreate.Body.String())
	}

	// 2. Default List (GET /api/article) -> should be empty because it defaults to published only
	reqListPub := httptest.NewRequest(http.MethodGet, "/api/article", nil)
	wListPub := httptest.NewRecorder()
	handler.ServeHTTP(wListPub, reqListPub)

	var listPubRes struct {
		Data []map[string]interface{} `json:"data"`
	}
	json.Unmarshal(wListPub.Body.Bytes(), &listPubRes)
	if len(listPubRes.Data) != 0 {
		t.Errorf("expected 0 published articles, got %d", len(listPubRes.Data))
	}

	// 3. List All (GET /api/article?status=all) -> should contain our draft article
	reqListAll := httptest.NewRequest(http.MethodGet, "/api/article?status=all", nil)
	wListAll := httptest.NewRecorder()
	handler.ServeHTTP(wListAll, reqListAll)

	var listAllRes struct {
		Data []map[string]interface{} `json:"data"`
	}
	json.Unmarshal(wListAll.Body.Bytes(), &listAllRes)
	if len(listAllRes.Data) != 1 {
		t.Fatalf("expected 1 article in status=all, got %d", len(listAllRes.Data))
	}

	// 4. Publish (POST /api/article/art-1/publish)
	reqPub := httptest.NewRequest(http.MethodPost, "/api/article/art-1/publish", nil)
	wPub := httptest.NewRecorder()
	handler.ServeHTTP(wPub, reqPub)

	if wPub.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on publish, got %d", wPub.Code)
	}

	// 5. Default List (GET /api/article) -> should now return the published article
	reqListPub2 := httptest.NewRequest(http.MethodGet, "/api/article", nil)
	wListPub2 := httptest.NewRecorder()
	handler.ServeHTTP(wListPub2, reqListPub2)

	json.Unmarshal(wListPub2.Body.Bytes(), &listPubRes)
	if len(listPubRes.Data) != 1 {
		t.Fatalf("expected 1 published article, got %d", len(listPubRes.Data))
	}

	// 6. Update Article (PATCH /api/article/art-1)
	updateBody := []byte(`{"title": "Updated Title"}`)
	reqPatch := httptest.NewRequest(http.MethodPatch, "/api/article/art-1", bytes.NewReader(updateBody))
	wPatch := httptest.NewRecorder()
	handler.ServeHTTP(wPatch, reqPatch)

	if wPatch.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on patch, got %d", wPatch.Code)
	}

	// 7. Finish Article (POST /api/article/art-1/finish)
	reqFin := httptest.NewRequest(http.MethodPost, "/api/article/art-1/finish", nil)
	wFin := httptest.NewRecorder()
	handler.ServeHTTP(wFin, reqFin)

	if wFin.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on finish, got %d", wFin.Code)
	}

	// 8. Delete Article (DELETE /api/article/art-1)
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/article/art-1", nil)
	wDel := httptest.NewRecorder()
	handler.ServeHTTP(wDel, reqDel)

	if wDel.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content on delete, got %d", wDel.Code)
	}
}
