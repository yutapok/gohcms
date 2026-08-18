package mcp_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yutapok/gohcms/pkg/content"
	"github.com/yutapok/gohcms/pkg/introspection"
	"github.com/yutapok/gohcms/pkg/mcp"
	"github.com/yutapok/gohcms/pkg/schema"
)

func setupTestMCPServer() (*mcp.Server, *content.ContentService, *content.MemoryAuditRepository) {
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
			"id":    {Type: schema.FieldTypeUUID, Column: "id", Readonly: true},
			"title": {Type: schema.FieldTypeString, Column: "title", Required: true},
			"body":  {Type: schema.FieldTypeText, Column: "body"},
		},
	}

	dbSchema := introspection.NewDatabaseSchema()
	dbSchema.AddTable(introspection.TableSchema{
		Name: "articles",
		Columns: map[string]introspection.ColumnSchema{
			"id":          {Name: "id", DataType: "uuid", UDTName: "uuid"},
			"title":       {Name: "title", DataType: "text", UDTName: "text"},
			"body":        {Name: "body", DataType: "text", UDTName: "text"},
			"cms_status":  {Name: "cms_status", DataType: "text", UDTName: "text"},
			"cms_version": {Name: "cms_version", DataType: "bigint", UDTName: "int8"},
		},
	})

	server := mcp.NewServer(svc, memAudit, []*schema.ResourceDefinition{def}, dbSchema)
	return server, svc, memAudit
}

func sendRPC(t *testing.T, server *mcp.Server, req string) mcp.Response {
	in := bytes.NewBufferString(req + "\n")
	var out bytes.Buffer

	err := server.Serve(in, &out)
	if err != nil {
		t.Fatalf("Serve error: %v", err)
	}

	var resp mcp.Response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse RPC response: %v, raw: %s", err, out.String())
	}
	return resp
}

func TestMCP_Initialize(t *testing.T) {
	server, _, _ := setupTestMCPServer()
	req := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}`

	resp := sendRPC(t, server, req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	resMap, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected result type: %T", resp.Result)
	}
	if resMap["protocolVersion"] != "2024-11-05" {
		t.Errorf("expected protocolVersion 2024-11-05, got %v", resMap["protocolVersion"])
	}
}

func TestMCP_ToolsList(t *testing.T) {
	server, _, _ := setupTestMCPServer()
	req := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`

	resp := sendRPC(t, server, req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	resMap := resp.Result.(map[string]interface{})
	tools := resMap["tools"].([]interface{})
	if len(tools) != 6 {
		t.Fatalf("expected 6 tools, got %d", len(tools))
	}
}

func TestMCP_SchemaDriftAndMutations(t *testing.T) {
	server, _, _ := setupTestMCPServer()

	// 1. Check Schema Drift
	reqDrift := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"cms_schema_drift","arguments":{}}}`
	respDrift := sendRPC(t, server, reqDrift)
	if respDrift.Error != nil {
		t.Fatalf("expected no error, got: %v", respDrift.Error)
	}

	// 2. Create Article via MCP (actor='agent')
	reqCreate := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"cms_content_mutate","arguments":{"resource":"article","title":"Agent Generated Post","body":"Created via MCP"}}}`
	respCreate := sendRPC(t, server, reqCreate)
	if respCreate.Error != nil {
		t.Fatalf("expected no error, got: %v", respCreate.Error)
	}
	createResult := respCreate.Result.(map[string]interface{})
	contentBlocks := createResult["content"].([]interface{})
	firstBlock := contentBlocks[0].(map[string]interface{})
	text := firstBlock["text"].(string)

	var createdRec content.Record
	json.Unmarshal([]byte(text), &createdRec)
	if createdRec.ID == "" || createdRec.Data["title"] != "Agent Generated Post" {
		t.Errorf("unexpected record: %+v", createdRec)
	}

	// 3. Publish Article via MCP
	reqPub := `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"cms_content_publish","arguments":{"resource":"article","id":"` + createdRec.ID + `","action":"publish"}}}`
	respPub := sendRPC(t, server, reqPub)
	if respPub.Error != nil {
		t.Fatalf("expected no error, got: %v", respPub.Error)
	}

	// 4. Verify Audit Search for actor='agent'
	reqAudit := `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"cms_audit_search","arguments":{"resource":"article","actor":"agent"}}}`
	respAudit := sendRPC(t, server, reqAudit)
	if respAudit.Error != nil {
		t.Fatalf("expected no error, got: %v", respAudit.Error)
	}
	auditResult := respAudit.Result.(map[string]interface{})
	auditBlocks := auditResult["content"].([]interface{})
	auditText := auditBlocks[0].(map[string]interface{})["text"].(string)

	if !strings.Contains(auditText, `"actor": "agent"`) {
		t.Errorf("expected audit text to contain actor agent, got:\n%s", auditText)
	}
}
