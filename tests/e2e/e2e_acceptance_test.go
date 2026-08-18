package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/yutapok/gohcms/internal/admin"
	"github.com/yutapok/gohcms/pkg/api"
	"github.com/yutapok/gohcms/pkg/auth"
	"github.com/yutapok/gohcms/pkg/content"
	"github.com/yutapok/gohcms/pkg/introspection"
	"github.com/yutapok/gohcms/pkg/job"
	"github.com/yutapok/gohcms/pkg/mcp"
	"github.com/yutapok/gohcms/pkg/media"
	"github.com/yutapok/gohcms/pkg/openapi"
	"github.com/yutapok/gohcms/pkg/schema"
	"github.com/yutapok/gohcms/pkg/validator"
)

// Define two distinct resources: Category and Article with Media
const categoryYAML = `resource: category
storage:
  table: categories
lifecycle:
  mode: none
fields:
  id:
    type: uuid
    column: id
    readonly: true
  name:
    type: string
    column: name
    required: true
  slug:
    type: string
    column: slug
    required: true
`

const articleYAML = `resource: article
storage:
  table: articles
lifecycle:
  mode: managed
  status_column: cms_status
  version_column: cms_version
fields:
  id:
    type: uuid
    column: id
    readonly: true
  title:
    type: string
    column: title
    required: true
  body:
    type: text
    column: body
  cover_image:
    type: media
    column: cover_image_id
  category_id:
    type: reference
    column: category_id
    resource: category
  depends_on:
    type: reference
    column: depends_on_id
    resource: article
  published_at:
    type: datetime
    column: published_at
`

func setupMultiResourceE2E(t *testing.T) (
	*content.ContentService,
	*content.MemoryContentRepository,
	*content.MemoryAuditRepository,
	*content.MemoryRevisionRepository,
	*media.Service,
	*auth.Service,
	[]*schema.ResourceDefinition,
	*introspection.DatabaseSchema,
) {
	catDef, err := schema.Parse([]byte(categoryYAML))
	if err != nil {
		t.Fatalf("failed to parse category YAML: %v", err)
	}

	artDef, err := schema.Parse([]byte(articleYAML))
	if err != nil {
		t.Fatalf("failed to parse article YAML: %v", err)
	}

	definitions := []*schema.ResourceDefinition{catDef, artDef}

	memRepo := content.NewMemoryContentRepository()
	memAudit := content.NewMemoryAuditRepository()
	memRev := content.NewMemoryRevisionRepository()
	uow := content.NewMemoryUnitOfWork(memRepo, memAudit, memRev)
	svc := content.NewService(uow, memRepo, memAudit, memRev)

	// Media & Auth Services
	memStorage := media.NewMemoryStorage()
	memMediaRepo := media.NewMemoryMediaRepository()
	mediaSvc := media.NewService(memStorage, memMediaRepo, memAudit)

	memKeyRepo := auth.NewMemoryAPIKeyRepository()
	authSvc := auth.NewService(memKeyRepo, memAudit)

	// Valid matching DB schema
	dbSchema := introspection.NewDatabaseSchema()
	dbSchema.AddTable(introspection.TableSchema{
		Name: "categories",
		Columns: map[string]introspection.ColumnSchema{
			"id":   {Name: "id", DataType: "uuid", UDTName: "uuid", IsNullable: false},
			"name": {Name: "name", DataType: "character varying", UDTName: "varchar", IsNullable: false},
			"slug": {Name: "slug", DataType: "character varying", UDTName: "varchar", IsNullable: false},
		},
	})
	dbSchema.AddTable(introspection.TableSchema{
		Name: "articles",
		Columns: map[string]introspection.ColumnSchema{
			"id":             {Name: "id", DataType: "uuid", UDTName: "uuid", IsNullable: false},
			"title":          {Name: "title", DataType: "character varying", UDTName: "varchar", IsNullable: false},
			"body":           {Name: "body", DataType: "text", UDTName: "text", IsNullable: true},
			"cover_image_id": {Name: "cover_image_id", DataType: "uuid", UDTName: "uuid", IsNullable: true},
			"category_id":    {Name: "category_id", DataType: "uuid", UDTName: "uuid", IsNullable: true},
			"depends_on_id":  {Name: "depends_on_id", DataType: "uuid", UDTName: "uuid", IsNullable: true},
			"published_at":   {Name: "published_at", DataType: "timestamp with time zone", UDTName: "timestamptz", IsNullable: true},
			"cms_status":     {Name: "cms_status", DataType: "text", UDTName: "text", IsNullable: false},
			"cms_version":    {Name: "cms_version", DataType: "bigint", UDTName: "int8", IsNullable: false},
		},
	})

	return svc, memRepo, memAudit, memRev, mediaSvc, authSvc, definitions, dbSchema
}

// 1. Schema Drift & Validation Acceptance
func TestE2E_SchemaDriftDetection(t *testing.T) {
	_, _, _, _, _, _, definitions, dbSchema := setupMultiResourceE2E(t)
	v := validator.New()

	// 1.1 Valid Schema Check
	result := v.ValidateAll(definitions, dbSchema)
	if !result.IsValid() {
		t.Fatalf("expected initial validation to pass, but got errors: %v", result.Errors)
	}

	// 1.2 Introduce Drift: Drop 'categories' table
	driftSchema := introspection.NewDatabaseSchema()
	driftSchema.AddTable(dbSchema.Tables["articles"])

	driftResult := v.ValidateAll(definitions, driftSchema)
	if driftResult.IsValid() {
		t.Fatal("expected validation to fail when categories table is missing, but it passed")
	}

	var foundMissingTable bool
	for _, err := range driftResult.Errors {
		if err.Resource == "category" && strings.Contains(err.Message, "does not exist in database") {
			foundMissingTable = true
			break
		}
	}
	if !foundMissingTable {
		t.Errorf("expected missing table error for category, got: %v", driftResult.Errors)
	}

	// 1.3 Introduce Drift: Column Type Mismatch in Article
	artTable := dbSchema.Tables["articles"]
	artTable.Columns["title"] = introspection.ColumnSchema{
		Name:     "title",
		DataType: "integer",
		UDTName:  "int4",
	}
	driftSchema.AddTable(artTable)

	typeDriftResult := v.ValidateAll(definitions, driftSchema)
	if typeDriftResult.IsValid() {
		t.Fatal("expected validation to fail for column type drift, but it passed")
	}
}

// 2. Admin UI & Lifecycle Acceptance (Full End-to-End Flow)
func TestE2E_AdminUI_FullLifecycleAndAudit(t *testing.T) {
	svc, _, auditRepo, revRepo, mediaSvc, authSvc, definitions, dbSchema := setupMultiResourceE2E(t)
	v := validator.New()
	valResult := v.ValidateAll(definitions, dbSchema)

	server, err := admin.NewServerWithFull(svc, auditRepo, revRepo, mediaSvc, authSvc, definitions, dbSchema, valResult, auth.Config{})
	if err != nil {
		t.Fatalf("failed to create admin server: %v", err)
	}
	handler := server.Handler()

	// 2.1 Create Category via Admin UI POST
	catForm := url.Values{}
	catForm.Set("name", "Engineering")
	catForm.Set("slug", "engineering")

	reqCat := httptest.NewRequest(http.MethodPost, "/admin/resources/category", strings.NewReader(catForm.Encode()))
	reqCat.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqCat.Header.Set("Sec-Fetch-Site", "same-origin")
	wCat := httptest.NewRecorder()
	handler.ServeHTTP(wCat, reqCat)

	if wCat.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on category create, got %d", wCat.Code)
	}

	// 2.2 Create Article 1 (Root)
	art1Form := url.Values{}
	art1Form.Set("title", "Go Architecture Guide")
	art1Form.Set("body", "Deep dive into clean Go architecture...")
	art1Form.Set("category_id", "cat-1")

	reqArt1 := httptest.NewRequest(http.MethodPost, "/admin/resources/article", strings.NewReader(art1Form.Encode()))
	reqArt1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqArt1.Header.Set("Sec-Fetch-Site", "same-origin")
	wArt1 := httptest.NewRecorder()
	handler.ServeHTTP(wArt1, reqArt1)

	if wArt1.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on article 1 create, got %d", wArt1.Code)
	}

	// 2.3 Publish Article 1
	records, _, _ := svc.List(context.Background(), definitions[1], content.ContentFilter{}, content.Pagination{Limit: 1})
	if len(records) == 0 {
		t.Fatal("expected article 1 in repository")
	}
	art1ID := records[0].ID

	reqPub1 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/admin/resources/article/%s/publish", art1ID), nil)
	reqPub1.Header.Set("Sec-Fetch-Site", "same-origin")
	wPub1 := httptest.NewRecorder()
	handler.ServeHTTP(wPub1, reqPub1)

	if wPub1.Code != http.StatusOK {
		t.Fatalf("expected 200 on publish article 1, got %d. Body: %s", wPub1.Code, wPub1.Body.String())
	}

	// 2.4 Finish Article 1
	reqFin1 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/admin/resources/article/%s/finish", art1ID), nil)
	reqFin1.Header.Set("Sec-Fetch-Site", "same-origin")
	wFin1 := httptest.NewRecorder()
	handler.ServeHTTP(wFin1, reqFin1)

	if wFin1.Code != http.StatusOK {
		t.Fatalf("expected 200 on finish article 1, got %d", wFin1.Code)
	}

	// 2.5 Verify Audit Trail & Revision Consistency
	logs, err := auditRepo.List(context.Background(), "article", art1ID)
	if err != nil || len(logs) < 3 {
		t.Fatalf("expected at least 3 audit logs (create, publish, finish), got %d (err: %v)", len(logs), err)
	}

	revs, err := revRepo.List(context.Background(), "article", art1ID)
	if err != nil || len(revs) < 3 {
		t.Fatalf("expected at least 3 revisions, got %d (err: %v)", len(revs), err)
	}

	// 2.6 View Kanban, Table, and Timeline views
	for _, view := range []string{"table", "kanban", "timeline"} {
		reqView := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/admin/resources/article?view=%s", view), nil)
		wView := httptest.NewRecorder()
		handler.ServeHTTP(wView, reqView)
		if wView.Code != http.StatusOK {
			t.Errorf("expected 200 for view %s, got %d", view, wView.Code)
		}
	}
}

// 3. Headless REST API & OpenAPI Acceptance
func TestE2E_REST_And_OpenAPIAcceptance(t *testing.T) {
	svc, _, _, _, _, _, definitions, _ := setupMultiResourceE2E(t)
	apiHandler := api.NewHandler(svc, definitions)

	// 3.1 OpenAPI Specification Generation & Verification
	gen := openapi.NewGenerator("Multi-Resource CMS API", "1.0.0", "E2E Acceptance Test")
	doc := gen.Generate(definitions)

	if _, exists := doc.Components.Schemas["Category"]; !exists {
		t.Error("expected Category schema in OpenAPI doc")
	}
	if _, exists := doc.Components.Schemas["Article"]; !exists {
		t.Error("expected Article schema in OpenAPI doc")
	}
	if _, exists := doc.Components.Schemas["Media"]; !exists {
		t.Error("expected Media schema in OpenAPI doc")
	}
	if _, exists := doc.Paths["/api/category"]; !exists {
		t.Error("expected /api/category in OpenAPI paths")
	}
	if _, exists := doc.Paths["/api/article"]; !exists {
		t.Error("expected /api/article in OpenAPI paths")
	}

	// 3.2 REST API: Create Category (POST /api/category)
	catBody := []byte(`{"id": "cat-e2e", "name": "Technology", "slug": "tech"}`)
	reqCat := httptest.NewRequest(http.MethodPost, "/api/category", bytes.NewReader(catBody))
	wCat := httptest.NewRecorder()
	apiHandler.ServeHTTP(wCat, reqCat)

	if wCat.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on category API, got %d. Body: %s", wCat.Code, wCat.Body.String())
	}

	// 3.3 REST API: Create Article (POST /api/article)
	artBody := []byte(`{"id": "art-e2e", "title": "REST API Article", "body": "Tested via E2E", "category_id": "cat-e2e"}`)
	reqArt := httptest.NewRequest(http.MethodPost, "/api/article", bytes.NewReader(artBody))
	wArt := httptest.NewRecorder()
	apiHandler.ServeHTTP(wArt, reqArt)

	if wArt.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on article API, got %d. Body: %s", wArt.Code, wArt.Body.String())
	}

	// 3.4 REST API: List Categories (GET /api/category)
	reqListCat := httptest.NewRequest(http.MethodGet, "/api/category", nil)
	wListCat := httptest.NewRecorder()
	apiHandler.ServeHTTP(wListCat, reqListCat)

	if wListCat.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on list categories, got %d", wListCat.Code)
	}

	var catListRes struct {
		Data []map[string]interface{} `json:"data"`
	}
	json.Unmarshal(wListCat.Body.Bytes(), &catListRes)
	if len(catListRes.Data) != 1 {
		t.Errorf("expected 1 category, got %d", len(catListRes.Data))
	}

	// 3.5 REST API: Publish Article (POST /api/article/art-e2e/publish)
	reqPub := httptest.NewRequest(http.MethodPost, "/api/article/art-e2e/publish", nil)
	wPub := httptest.NewRecorder()
	apiHandler.ServeHTTP(wPub, reqPub)

	if wPub.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on publish article API, got %d", wPub.Code)
	}

	// 3.6 REST API: List Published Articles (GET /api/article)
	reqListArt := httptest.NewRequest(http.MethodGet, "/api/article", nil)
	wListArt := httptest.NewRecorder()
	apiHandler.ServeHTTP(wListArt, reqListArt)

	var artListRes struct {
		Data []map[string]interface{} `json:"data"`
	}
	json.Unmarshal(wListArt.Body.Bytes(), &artListRes)
	if len(artListRes.Data) != 1 {
		t.Errorf("expected 1 published article, got %d", len(artListRes.Data))
	}
}

// 4. Milestone v0.2: Media Asset Management and API Key Security Acceptance
func TestE2E_MediaAndAPIKeySecurity(t *testing.T) {
	svc, _, _, _, mediaSvc, authSvc, definitions, dbSchema := setupMultiResourceE2E(t)
	v := validator.New()
	valResult := v.ValidateAll(definitions, dbSchema)

	ctx := context.Background()
	mctx := content.MutationContext{Actor: "admin-user", ActorType: content.ActorTypeUser, RequestID: "req-e2e-auth"}

	// 4.1 Issue API Keys
	readToken, _, err := authSvc.CreateKey(ctx, "Next.js Frontend Read", auth.PermissionRead, mctx)
	if err != nil {
		t.Fatalf("failed to create read key: %v", err)
	}
	rwToken, _, err := authSvc.CreateKey(ctx, "Backend Service RW", auth.PermissionReadWrite, mctx)
	if err != nil {
		t.Fatalf("failed to create rw key: %v", err)
	}

	// 4.2 Start Server with API Key Auth Enabled
	authCfg := auth.Config{
		Service: authSvc,
	}
	server, err := admin.NewServerWithFull(svc, content.NewMemoryAuditRepository(), content.NewMemoryRevisionRepository(), mediaSvc, authSvc, definitions, dbSchema, valResult, authCfg)
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	handler := server.Handler()

	// 4.3 Upload Image via Admin UI (/admin/media)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "header-banner.png")
	part.Write([]byte("banner-png-data-stream"))
	writer.Close()

	reqUpload := httptest.NewRequest(http.MethodPost, "/admin/media", &body)
	reqUpload.Header.Set("Content-Type", writer.FormDataContentType())
	reqUpload.Header.Set("Sec-Fetch-Site", "same-origin")
	wUpload := httptest.NewRecorder()
	handler.ServeHTTP(wUpload, reqUpload)

	if wUpload.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on admin media upload, got %d", wUpload.Code)
	}

	// Verify uploaded media
	mediaList, _ := mediaSvc.List(ctx)
	if len(mediaList) == 0 {
		t.Fatal("expected at least 1 media item")
	}
	bannerID := mediaList[0].ID

	// 4.4 Verify Binary Delivery (/media/{id})
	reqDelivery := httptest.NewRequest(http.MethodGet, "/media/"+bannerID, nil)
	wDelivery := httptest.NewRecorder()
	handler.ServeHTTP(wDelivery, reqDelivery)

	if wDelivery.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /media delivery, got %d", wDelivery.Code)
	}
	if !bytes.Equal(wDelivery.Body.Bytes(), []byte("banner-png-data-stream")) {
		t.Error("delivered media binary does not match uploaded data")
	}

	// 4.5 Create Article with Media Reference via Admin UI
	artForm := url.Values{}
	artForm.Set("title", "Media Rich Article")
	artForm.Set("body", "Article with banner image")
	artForm.Set("cover_image", bannerID)

	reqCreateArt := httptest.NewRequest(http.MethodPost, "/admin/resources/article", strings.NewReader(artForm.Encode()))
	reqCreateArt.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqCreateArt.Header.Set("Sec-Fetch-Site", "same-origin")
	wCreateArt := httptest.NewRecorder()
	handler.ServeHTTP(wCreateArt, reqCreateArt)

	if wCreateArt.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on article create, got %d", wCreateArt.Code)
	}

	// Publish Article
	articles, _, _ := svc.List(ctx, definitions[1], content.ContentFilter{}, content.Pagination{Limit: 1})
	artID := articles[0].ID
	svc.Publish(ctx, definitions[1], artID, mctx)

	// 4.6 REST API Access Control Verification:
	// A. Unauthenticated Request -> 401 Unauthorized
	reqNoAuth := httptest.NewRequest(http.MethodGet, "/api/article", nil)
	wNoAuth := httptest.NewRecorder()
	handler.ServeHTTP(wNoAuth, reqNoAuth)
	if wNoAuth.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for unauthenticated REST request, got %d", wNoAuth.Code)
	}

	// B. Read-Only Key -> GET 200 OK
	reqRead := httptest.NewRequest(http.MethodGet, "/api/article", nil)
	reqRead.Header.Set("Authorization", "Bearer "+readToken)
	wRead := httptest.NewRecorder()
	handler.ServeHTTP(wRead, reqRead)
	if wRead.Code != http.StatusOK {
		t.Errorf("expected 200 OK for GET with read token, got %d", wRead.Code)
	}

	// C. Read-Only Key -> POST 403 Forbidden
	reqForbidden := httptest.NewRequest(http.MethodPost, "/api/article", strings.NewReader(`{"title":"Hacked"}`))
	reqForbidden.Header.Set("Authorization", "Bearer "+readToken)
	wForbidden := httptest.NewRecorder()
	handler.ServeHTTP(wForbidden, reqForbidden)
	if wForbidden.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for POST with read token, got %d", wForbidden.Code)
	}

	// D. Read-Write Key -> POST 201 Created
	reqRW := httptest.NewRequest(http.MethodPost, "/api/article", strings.NewReader(`{"title":"New API Article"}`))
	reqRW.Header.Set("X-API-Key", rwToken)
	wRW := httptest.NewRecorder()
	handler.ServeHTTP(wRW, reqRW)
	if wRW.Code != http.StatusCreated {
		t.Errorf("expected 201 Created for POST with rw token, got %d", wRW.Code)
	}
}

// 5. Milestone v0.3: Stateless MCP & Runtime Role Separation & One-shot Job Acceptance
func TestE2E_MCPAndRolesAndJobs(t *testing.T) {
	svc, _, auditRepo, _, _, authSvc, definitions, dbSchema := setupMultiResourceE2E(t)

	// 5.1 MCP Server: stdio JSON-RPC 2.0 flow
	mcpServer := mcp.NewServer(svc, auditRepo, definitions, dbSchema)

	// 5.1.1 Initialize
	inInit := bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"claude","version":"1.0"}}}` + "\n")
	var outInit bytes.Buffer
	if err := mcpServer.Serve(inInit, &outInit); err != nil {
		t.Fatalf("mcp initialize failed: %v", err)
	}
	if !strings.Contains(outInit.String(), `"protocolVersion":"2024-11-05"`) {
		t.Errorf("expected protocolVersion in mcp init response, got: %s", outInit.String())
	}

	// 5.1.2 Create content via MCP tool cms_content_mutate
	inCreate := bytes.NewBufferString(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"cms_content_mutate","arguments":{"resource":"article","title":"Agent Created Article","body":"Authored by Claude"}}}` + "\n")
	var outCreate bytes.Buffer
	if err := mcpServer.Serve(inCreate, &outCreate); err != nil {
		t.Fatalf("mcp create failed: %v", err)
	}
	if !strings.Contains(outCreate.String(), "Agent Created Article") {
		t.Errorf("expected created article title in mcp response, got: %s", outCreate.String())
	}

	// 5.1.3 Search audit log via MCP tool cms_audit_search for actor='agent'
	inAudit := bytes.NewBufferString(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"cms_audit_search","arguments":{"resource":"article","actor":"agent"}}}` + "\n")
	var outAudit bytes.Buffer
	if err := mcpServer.Serve(inAudit, &outAudit); err != nil {
		t.Fatalf("mcp audit search failed: %v", err)
	}
	if !strings.Contains(outAudit.String(), "agent") {
		t.Errorf("expected actor agent in audit search, got: %s", outAudit.String())
	}

	// 5.2 Runtime Role Separation: verify routing isolation
	server, err := admin.NewServerWithFull(svc, auditRepo, content.NewMemoryRevisionRepository(), nil, authSvc, definitions, dbSchema, &validator.ValidationResult{}, auth.Config{})
	if err != nil {
		t.Fatalf("failed to create admin server: %v", err)
	}

	// RoleAdmin: /admin/resources/article is 200, /api/article is 404
	adminMux := server.HandlerForRole(admin.RoleAdmin)
	wAdmin1 := httptest.NewRecorder()
	adminMux.ServeHTTP(wAdmin1, httptest.NewRequest(http.MethodGet, "/admin/resources/article", nil))
	if wAdmin1.Code != http.StatusOK {
		t.Errorf("expected 200 for /admin/resources/article under RoleAdmin, got %d", wAdmin1.Code)
	}

	wAdmin2 := httptest.NewRecorder()
	adminMux.ServeHTTP(wAdmin2, httptest.NewRequest(http.MethodGet, "/api/article", nil))
	if wAdmin2.Code != http.StatusNotFound {
		t.Errorf("expected 404 for /api/article under RoleAdmin, got %d", wAdmin2.Code)
	}

	// RoleAPI: /api/openapi.json is 200, /admin/resources/article is 404
	apiMux := server.HandlerForRole(admin.RoleAPI)
	wAPI1 := httptest.NewRecorder()
	apiMux.ServeHTTP(wAPI1, httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil))
	if wAPI1.Code != http.StatusOK {
		t.Errorf("expected 200 for /api/openapi.json under RoleAPI, got %d", wAPI1.Code)
	}

	wAPI2 := httptest.NewRecorder()
	apiMux.ServeHTTP(wAPI2, httptest.NewRequest(http.MethodGet, "/admin/resources/article", nil))
	if wAPI2.Code != http.StatusNotFound {
		t.Errorf("expected 404 for /admin/resources/article under RoleAPI, got %d", wAPI2.Code)
	}

	// 5.3 One-shot Job Runner: Validate Job
	vJob := job.NewValidateJob(definitions, dbSchema)
	code, err := vJob.Run(context.Background(), nil)
	if err != nil || code != 0 {
		t.Errorf("expected job validate to return 0, got code=%d, err=%v", code, err)
	}
}
