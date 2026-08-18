package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/yutapok/gohcms/pkg/content"
	"github.com/yutapok/gohcms/pkg/introspection"
	"github.com/yutapok/gohcms/pkg/schema"
	"github.com/yutapok/gohcms/pkg/validator"
)

// Server implements a lightweight, pure Go stateless MCP server over stdio.
type Server struct {
	svc         *content.ContentService
	auditor     content.AuditRepository
	definitions []*schema.ResourceDefinition
	dbSchema    *introspection.DatabaseSchema
	name        string
	version     string
}

// NewServer creates a new Stateless MCP Server instance.
func NewServer(
	svc *content.ContentService,
	auditor content.AuditRepository,
	definitions []*schema.ResourceDefinition,
	dbSchema *introspection.DatabaseSchema,
) *Server {
	return &Server{
		svc:         svc,
		auditor:     auditor,
		definitions: definitions,
		dbSchema:    dbSchema,
		name:        "gohcms-mcp-server",
		version:     "0.3.0",
	}
}

// Serve reads JSON-RPC 2.0 messages line-by-line from in and writes JSON responses to out.
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	// Allow large payloads (up to 10MB)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	encoder := json.NewEncoder(out)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			resp := Response{
				JSONRPC: "2.0",
				Error: &RPCError{
					Code:    -32700,
					Message: "Parse error: invalid JSON",
				},
			}
			if encErr := encoder.Encode(resp); encErr != nil {
				return encErr
			}
			continue
		}

		resp := s.handleRequest(context.Background(), &req)
		if req.ID != nil || resp.Error != nil {
			if err := encoder.Encode(resp); err != nil {
				return err
			}
		}
	}

	return scanner.Err()
}

func (s *Server) handleRequest(ctx context.Context, req *Request) Response {
	resp := Response{
		JSONRPC: "2.0",
		ID:      req.ID,
	}

	switch req.Method {
	case "initialize":
		resp.Result = InitializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities: ServerCaps{
				Tools: &ToolsCap{},
			},
			ServerInfo: ServerInfo{
				Name:    s.name,
				Version: s.version,
			},
			Instructions: "gohcms Agent-Native Headless CMS MCP Server. Use available tools to inspect schemas, search audit logs, query content, and perform validated mutations.",
		}

	case "notifications/initialized":
		// No response required for notification
		return resp

	case "ping":
		resp.Result = map[string]interface{}{}

	case "tools/list":
		resp.Result = ListToolsResult{
			Tools: s.getToolDefinitions(),
		}

	case "tools/call":
		var params CallToolParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &RPCError{Code: -32602, Message: "Invalid params: " + err.Error()}
			return resp
		}

		result, err := s.callTool(ctx, params.Name, params.Arguments)
		if err != nil {
			resp.Result = CallToolResult{
				Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Error: %v", err)}},
				IsError: true,
			}
		} else {
			resp.Result = result
		}

	default:
		resp.Error = &RPCError{
			Code:    -32601,
			Message: fmt.Sprintf("Method not found: %s", req.Method),
		}
	}

	return resp
}

func (s *Server) getToolDefinitions() []Tool {
	return []Tool{
		{
			Name:        "cms_schema_drift",
			Description: "Validate and check for schema drift between YAML Resource Definitions and database tables.",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]PropertyDef{},
			},
		},
		{
			Name:        "cms_audit_search",
			Description: "Search immutable audit logs by resource, record ID, or actor.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"resource": {
						Type:        "string",
						Description: "Target resource name (e.g. article, category)",
					},
					"resource_id": {
						Type:        "string",
						Description: "Optional target record ID to filter by",
					},
					"actor": {
						Type:        "string",
						Description: "Optional actor identifier (e.g. agent, admin-user)",
					},
				},
				Required: []string{"resource"},
			},
		},
		{
			Name:        "cms_content_list",
			Description: "List content records for a specified resource with optional status filter.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"resource": {
						Type:        "string",
						Description: "Target resource name (e.g. article)",
					},
					"status": {
						Type:        "string",
						Description: "Filter by lifecycle status: 'published', 'draft', 'finished', or 'all'",
						Enum:        []string{"published", "draft", "finished", "all"},
					},
				},
				Required: []string{"resource"},
			},
		},
		{
			Name:        "cms_content_get",
			Description: "Get detailed content record by ID.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"resource": {
						Type:        "string",
						Description: "Target resource name (e.g. article)",
					},
					"id": {
						Type:        "string",
						Description: "The UUID or primary key of the record",
					},
				},
				Required: []string{"resource", "id"},
			},
		},
		{
			Name:        "cms_content_mutate",
			Description: "Create or update a content record with schema validation and audit logging (actor='agent').",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"resource": {
						Type:        "string",
						Description: "Target resource name (e.g. article)",
					},
					"id": {
						Type:        "string",
						Description: "Optional record ID. If provided, updates existing record; if omitted, creates new record.",
					},
				},
				Required: []string{"resource"},
			},
		},
		{
			Name:        "cms_content_publish",
			Description: "Transition the lifecycle state of a content record (publish, unpublish, or finish).",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"resource": {
						Type:        "string",
						Description: "Target resource name (e.g. article)",
					},
					"id": {
						Type:        "string",
						Description: "Record ID to transition",
					},
					"action": {
						Type:        "string",
						Description: "Lifecycle transition action: 'publish', 'unpublish', or 'finish'",
						Enum:        []string{"publish", "unpublish", "finish"},
					},
				},
				Required: []string{"resource", "id", "action"},
			},
		},
	}
}

func (s *Server) getDefinition(name string) *schema.ResourceDefinition {
	for _, d := range s.definitions {
		if d.Resource == name {
			return d
		}
	}
	return nil
}

func (s *Server) callTool(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error) {
	mctx := content.MutationContext{
		Actor:     "agent",
		ActorType: content.ActorTypeAgent,
		RequestID: fmt.Sprintf("mcp-%d", time.Now().UnixNano()),
	}

	switch name {
	case "cms_schema_drift":
		v := validator.New()
		result := v.ValidateAll(s.definitions, s.dbSchema)
		report := result.FormatReport()
		return &CallToolResult{
			Content: []ContentBlock{{Type: "text", Text: report}},
			IsError: !result.IsValid(),
		}, nil

	case "cms_audit_search":
		resource, _ := args["resource"].(string)
		if resource == "" {
			return nil, fmt.Errorf("missing required argument 'resource'")
		}
		recordID, _ := args["resource_id"].(string)
		actorFilter, _ := args["actor"].(string)

		logs, err := s.auditor.List(ctx, resource, recordID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch audit logs: %w", err)
		}

		var filtered []*content.AuditLog
		for _, log := range logs {
			if actorFilter != "" && !strings.EqualFold(log.Actor, actorFilter) {
				continue
			}
			filtered = append(filtered, log)
		}

		data, _ := json.MarshalIndent(filtered, "", "  ")
		return &CallToolResult{
			Content: []ContentBlock{{Type: "text", Text: string(data)}},
		}, nil

	case "cms_content_list":
		resource, _ := args["resource"].(string)
		def := s.getDefinition(resource)
		if def == nil {
			return nil, fmt.Errorf("resource definition '%s' not found", resource)
		}

		var filter content.ContentFilter
		if statusStr, ok := args["status"].(string); ok && statusStr != "" && statusStr != "all" {
			status := content.ContentStatus(statusStr)
			filter.Status = &status
		}

		records, total, err := s.svc.List(ctx, def, filter, content.Pagination{Limit: 100})
		if err != nil {
			return nil, fmt.Errorf("failed to list records: %w", err)
		}

		out := map[string]interface{}{
			"resource": resource,
			"total":    total,
			"records":  records,
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		return &CallToolResult{
			Content: []ContentBlock{{Type: "text", Text: string(data)}},
		}, nil

	case "cms_content_get":
		resource, _ := args["resource"].(string)
		id, _ := args["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("missing required argument 'id'")
		}
		def := s.getDefinition(resource)
		if def == nil {
			return nil, fmt.Errorf("resource definition '%s' not found", resource)
		}

		record, err := s.svc.Get(ctx, def, id)
		if err != nil {
			return nil, fmt.Errorf("record not found: %w", err)
		}

		data, _ := json.MarshalIndent(record, "", "  ")
		return &CallToolResult{
			Content: []ContentBlock{{Type: "text", Text: string(data)}},
		}, nil

	case "cms_content_mutate":
		resource, _ := args["resource"].(string)
		def := s.getDefinition(resource)
		if def == nil {
			return nil, fmt.Errorf("resource definition '%s' not found", resource)
		}

		// Extract form data from arguments excluding 'resource' and 'id'
		formData := make(map[string]interface{})
		for k, v := range args {
			if k != "resource" && k != "id" {
				formData[k] = v
			}
		}

		id, _ := args["id"].(string)
		var record *content.Record
		var err error

		if id != "" {
			// Update
			record, err = s.svc.Update(ctx, def, id, formData, mctx)
		} else {
			// Create
			record, err = s.svc.Create(ctx, def, formData, mctx)
		}

		if err != nil {
			return nil, fmt.Errorf("mutation error: %w", err)
		}

		data, _ := json.MarshalIndent(record, "", "  ")
		return &CallToolResult{
			Content: []ContentBlock{{Type: "text", Text: string(data)}},
		}, nil

	case "cms_content_publish":
		resource, _ := args["resource"].(string)
		id, _ := args["id"].(string)
		action, _ := args["action"].(string)

		if id == "" || action == "" {
			return nil, fmt.Errorf("missing required arguments 'id' and 'action'")
		}
		def := s.getDefinition(resource)
		if def == nil {
			return nil, fmt.Errorf("resource definition '%s' not found", resource)
		}

		var record *content.Record
		var err error

		switch action {
		case "publish":
			record, err = s.svc.Publish(ctx, def, id, mctx)
		case "unpublish":
			record, err = s.svc.Unpublish(ctx, def, id, mctx)
		case "finish":
			record, err = s.svc.Finish(ctx, def, id, mctx)
		default:
			return nil, fmt.Errorf("invalid action '%s' (expected 'publish', 'unpublish', or 'finish')", action)
		}

		if err != nil {
			return nil, fmt.Errorf("lifecycle transition failed: %w", err)
		}

		data, _ := json.MarshalIndent(record, "", "  ")
		return &CallToolResult{
			Content: []ContentBlock{{Type: "text", Text: string(data)}},
		}, nil

	default:
		return nil, fmt.Errorf("unknown tool '%s'", name)
	}
}
