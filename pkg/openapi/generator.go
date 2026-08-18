package openapi

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yutapok/gohcms/pkg/schema"
	"gopkg.in/yaml.v3"
)

// Generator builds OpenAPI 3.1 specifications from Resource Definitions.
type Generator struct {
	title       string
	version     string
	description string
}

// NewGenerator creates a new OpenAPI Generator.
func NewGenerator(title, version, description string) *Generator {
	if title == "" {
		title = "gohcms Headless API"
	}
	if version == "" {
		version = "1.0.0"
	}
	return &Generator{
		title:       title,
		version:     version,
		description: description,
	}
}

// Generate generates an OpenAPI 3.1 object from definitions.
func (g *Generator) Generate(definitions []*schema.ResourceDefinition) *OpenAPI {
	doc := &OpenAPI{
		OpenAPI: "3.1.0",
		Info: Info{
			Title:       g.title,
			Version:     g.version,
			Description: g.description,
		},
		Security: []map[string][]string{
			{"BearerAuth": {}},
			{"ApiKeyAuth": {}},
		},
		Paths: make(map[string]PathItem),
		Components: Components{
			Schemas: make(map[string]*Schema),
			SecuritySchemes: map[string]SecurityScheme{
				"BearerAuth": {
					Type:         "http",
					Scheme:       "bearer",
					BearerFormat: "gohcms_live_*",
					Description:  "Use Bearer token for API key authentication",
				},
				"ApiKeyAuth": {
					Type:        "apiKey",
					In:          "header",
					Name:        "X-API-Key",
					Description: "Use X-API-Key header for API key authentication",
				},
			},
		},
	}

	// Add common Error Schema
	doc.Components.Schemas["ErrorResponse"] = &Schema{
		Type: "object",
		Properties: map[string]*Schema{
			"error": {
				Type: "object",
				Properties: map[string]*Schema{
					"code":    {Type: "string"},
					"message": {Type: "string"},
				},
				Required: []string{"code", "message"},
			},
		},
		Required: []string{"error"},
	}

	// Add Media Schema
	doc.Components.Schemas["Media"] = &Schema{
		Type:        "object",
		Description: "Uploaded media asset metadata",
		Properties: map[string]*Schema{
			"id":         {Type: "string", Format: "uuid", ReadOnly: true},
			"filename":   {Type: "string"},
			"filepath":   {Type: "string", ReadOnly: true},
			"mime_type":  {Type: "string"},
			"size_bytes": {Type: "integer"},
			"url":        {Type: "string", ReadOnly: true},
			"created_at": {Type: "string", Format: "date-time", ReadOnly: true},
		},
		Required: []string{"id", "filename", "mime_type", "size_bytes", "url", "created_at"},
	}

	// Add Media Paths (/api/media)
	doc.Paths["/api/media"] = PathItem{
		Summary: "Media Assets Management",
		Get: &Operation{
			Tags:        []string{"Media"},
			Summary:     "List all media assets",
			OperationID: "listMedia",
			Responses: map[string]Response{
				"200": {
					Description: "List of media assets",
					Content: map[string]MediaType{
						"application/json": {
							Schema: &Schema{
								Type: "object",
								Properties: map[string]*Schema{
									"data": {Type: "array", Items: &Schema{Ref: "#/components/schemas/Media"}},
								},
							},
						},
					},
				},
			},
		},
		Post: &Operation{
			Tags:        []string{"Media"},
			Summary:     "Upload a new media asset",
			OperationID: "uploadMedia",
			RequestBody: &RequestBody{
				Required: true,
				Content: map[string]MediaType{
					"multipart/form-data": {
						Schema: &Schema{
							Type: "object",
							Properties: map[string]*Schema{
								"file": {Type: "string", Format: "binary", Description: "Binary file to upload"},
							},
							Required: []string{"file"},
						},
					},
				},
			},
			Responses: map[string]Response{
				"201": {
					Description: "Uploaded media asset",
					Content: map[string]MediaType{
						"application/json": {
							Schema: &Schema{
								Type: "object",
								Properties: map[string]*Schema{
									"data": {Ref: "#/components/schemas/Media"},
								},
							},
						},
					},
				},
				"400": {Description: "Upload error", Content: map[string]MediaType{"application/json": {Schema: &Schema{Ref: "#/components/schemas/ErrorResponse"}}}},
			},
		},
	}

	doc.Paths["/api/media/{id}"] = PathItem{
		Summary: "Individual Media Asset Operations",
		Get: &Operation{
			Tags:        []string{"Media"},
			Summary:     "Get media metadata by ID",
			OperationID: "getMediaById",
			Parameters: []Parameter{
				{Name: "id", In: "path", Required: true, Schema: &Schema{Type: "string", Format: "uuid"}},
			},
			Responses: map[string]Response{
				"200": {
					Description: "Found media metadata",
					Content: map[string]MediaType{
						"application/json": {
							Schema: &Schema{
								Type: "object",
								Properties: map[string]*Schema{
									"data": {Ref: "#/components/schemas/Media"},
								},
							},
						},
					},
				},
				"404": {Description: "Media not found", Content: map[string]MediaType{"application/json": {Schema: &Schema{Ref: "#/components/schemas/ErrorResponse"}}}},
			},
		},
		Delete: &Operation{
			Tags:        []string{"Media"},
			Summary:     "Delete media asset by ID",
			OperationID: "deleteMediaById",
			Parameters: []Parameter{
				{Name: "id", In: "path", Required: true, Schema: &Schema{Type: "string", Format: "uuid"}},
			},
			Responses: map[string]Response{
				"204": {Description: "Deleted successfully"},
				"404": {Description: "Media not found", Content: map[string]MediaType{"application/json": {Schema: &Schema{Ref: "#/components/schemas/ErrorResponse"}}}},
			},
		},
	}

	for _, def := range definitions {
		resourceName := def.Resource
		typeName := strings.ToUpper(resourceName[:1]) + resourceName[1:]

		// 1. Build Entity Schema
		entitySchema := &Schema{
			Type:        "object",
			Description: fmt.Sprintf("%s content entity", typeName),
			Properties:  make(map[string]*Schema),
		}

		createSchema := &Schema{
			Type:        "object",
			Description: fmt.Sprintf("Input payload for creating a %s", typeName),
			Properties:  make(map[string]*Schema),
		}

		var requiredFields []string

		// Add defined fields
		for fieldName, field := range def.Fields {
			fSchema := mapFieldToOpenAPISchema(field)
			entitySchema.Properties[fieldName] = fSchema

			if field.Readonly || fieldName == "id" {
				fSchema.ReadOnly = true
			} else {
				createSchema.Properties[fieldName] = fSchema
				if field.Required {
					requiredFields = append(requiredFields, fieldName)
				}
			}
		}

		// Add system fields
		entitySchema.Properties["id"] = &Schema{Type: "string", Format: "uuid", ReadOnly: true}
		if def.Lifecycle.Mode == schema.LifecycleModeManaged {
			entitySchema.Properties["status"] = &Schema{
				Type: "string",
				Enum: []interface{}{"draft", "published", "finished"},
			}
			entitySchema.Properties["version"] = &Schema{Type: "integer"}
		}
		entitySchema.Properties["created_at"] = &Schema{Type: "string", Format: "date-time", ReadOnly: true}
		entitySchema.Properties["updated_at"] = &Schema{Type: "string", Format: "date-time", ReadOnly: true}

		createSchema.Required = requiredFields

		doc.Components.Schemas[typeName] = entitySchema
		doc.Components.Schemas[typeName+"CreateInput"] = createSchema

		// 2. Build Paths
		listPath := fmt.Sprintf("/api/%s", resourceName)
		detailPath := fmt.Sprintf("/api/%s/{id}", resourceName)

		// List & Create Path Item
		doc.Paths[listPath] = PathItem{
			Summary: fmt.Sprintf("Manage %s collection", resourceName),
			Get: &Operation{
				Tags:        []string{typeName},
				Summary:     fmt.Sprintf("List %s records", resourceName),
				OperationID: fmt.Sprintf("list%s", typeName),
				Parameters: []Parameter{
					{Name: "limit", In: "query", Schema: &Schema{Type: "integer"}, Description: "Number of records to return (default: 20)"},
					{Name: "offset", In: "query", Schema: &Schema{Type: "integer"}, Description: "Offset for pagination"},
					{Name: "status", In: "query", Schema: &Schema{Type: "string", Enum: []interface{}{"published", "draft", "finished", "all"}}, Description: "Filter by lifecycle status (default: published)"},
				},
				Responses: map[string]Response{
					"200": {
						Description: "List of records",
						Content: map[string]MediaType{
							"application/json": {
								Schema: &Schema{
									Type: "object",
									Properties: map[string]*Schema{
										"data": {Type: "array", Items: &Schema{Ref: fmt.Sprintf("#/components/schemas/%s", typeName)}},
										"meta": {
											Type: "object",
											Properties: map[string]*Schema{
												"total":  {Type: "integer"},
												"limit":  {Type: "integer"},
												"offset": {Type: "integer"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			Post: &Operation{
				Tags:        []string{typeName},
				Summary:     fmt.Sprintf("Create a new %s", resourceName),
				OperationID: fmt.Sprintf("create%s", typeName),
				RequestBody: &RequestBody{
					Required: true,
					Content: map[string]MediaType{
						"application/json": {
							Schema: &Schema{Ref: fmt.Sprintf("#/components/schemas/%sCreateInput", typeName)},
						},
					},
				},
				Responses: map[string]Response{
					"201": {
						Description: "Created record",
						Content: map[string]MediaType{
							"application/json": {
								Schema: &Schema{
									Type: "object",
									Properties: map[string]*Schema{
										"data": {Ref: fmt.Sprintf("#/components/schemas/%s", typeName)},
									},
								},
							},
						},
					},
					"400": {Description: "Validation error", Content: map[string]MediaType{"application/json": {Schema: &Schema{Ref: "#/components/schemas/ErrorResponse"}}}},
				},
			},
		}

		// Detail Path Item (Get, Patch, Delete)
		doc.Paths[detailPath] = PathItem{
			Summary: fmt.Sprintf("Individual %s operations", resourceName),
			Get: &Operation{
				Tags:        []string{typeName},
				Summary:     fmt.Sprintf("Get %s by ID", resourceName),
				OperationID: fmt.Sprintf("get%sById", typeName),
				Parameters: []Parameter{
					{Name: "id", In: "path", Required: true, Schema: &Schema{Type: "string", Format: "uuid"}},
				},
				Responses: map[string]Response{
					"200": {
						Description: "Found record",
						Content: map[string]MediaType{
							"application/json": {
								Schema: &Schema{
									Type: "object",
									Properties: map[string]*Schema{
										"data": {Ref: fmt.Sprintf("#/components/schemas/%s", typeName)},
									},
								},
							},
						},
					},
					"404": {Description: "Record not found", Content: map[string]MediaType{"application/json": {Schema: &Schema{Ref: "#/components/schemas/ErrorResponse"}}}},
				},
			},
			Patch: &Operation{
				Tags:        []string{typeName},
				Summary:     fmt.Sprintf("Update %s by ID", resourceName),
				OperationID: fmt.Sprintf("update%sById", typeName),
				Parameters: []Parameter{
					{Name: "id", In: "path", Required: true, Schema: &Schema{Type: "string", Format: "uuid"}},
				},
				RequestBody: &RequestBody{
					Required: true,
					Content: map[string]MediaType{
						"application/json": {
							Schema: &Schema{Ref: fmt.Sprintf("#/components/schemas/%sCreateInput", typeName)},
						},
					},
				},
				Responses: map[string]Response{
					"200": {
						Description: "Updated record",
						Content: map[string]MediaType{
							"application/json": {
								Schema: &Schema{
									Type: "object",
									Properties: map[string]*Schema{
										"data": {Ref: fmt.Sprintf("#/components/schemas/%s", typeName)},
									},
								},
							},
						},
					},
					"404": {Description: "Record not found", Content: map[string]MediaType{"application/json": {Schema: &Schema{Ref: "#/components/schemas/ErrorResponse"}}}},
				},
			},
			Delete: &Operation{
				Tags:        []string{typeName},
				Summary:     fmt.Sprintf("Delete %s by ID", resourceName),
				OperationID: fmt.Sprintf("delete%sById", typeName),
				Parameters: []Parameter{
					{Name: "id", In: "path", Required: true, Schema: &Schema{Type: "string", Format: "uuid"}},
				},
				Responses: map[string]Response{
					"204": {Description: "Deleted successfully"},
					"404": {Description: "Record not found", Content: map[string]MediaType{"application/json": {Schema: &Schema{Ref: "#/components/schemas/ErrorResponse"}}}},
				},
			},
		}

		// Lifecycle Action Endpoints
		if def.Lifecycle.Mode == schema.LifecycleModeManaged {
			for _, action := range []string{"publish", "unpublish", "finish"} {
				actionPath := fmt.Sprintf("/api/%s/{id}/%s", resourceName, action)
				doc.Paths[actionPath] = PathItem{
					Summary: fmt.Sprintf("%s %s record", strings.Title(action), resourceName),
					Post: &Operation{
						Tags:        []string{typeName},
						Summary:     fmt.Sprintf("%s %s by ID", strings.Title(action), resourceName),
						OperationID: fmt.Sprintf("%s%s", action, typeName),
						Parameters: []Parameter{
							{Name: "id", In: "path", Required: true, Schema: &Schema{Type: "string", Format: "uuid"}},
						},
						Responses: map[string]Response{
							"200": {
								Description: "Status transitioned successfully",
								Content: map[string]MediaType{
									"application/json": {
										Schema: &Schema{
											Type: "object",
											Properties: map[string]*Schema{
												"data": {Ref: fmt.Sprintf("#/components/schemas/%s", typeName)},
											},
										},
									},
								},
							},
							"400": {Description: "Transition error (e.g. prerequisite not published)", Content: map[string]MediaType{"application/json": {Schema: &Schema{Ref: "#/components/schemas/ErrorResponse"}}}},
						},
					},
				}
			}
		}
	}

	return doc
}

// ToJSON serializes the OpenAPI document to formatted JSON.
func (g *Generator) ToJSON(definitions []*schema.ResourceDefinition) ([]byte, error) {
	doc := g.Generate(definitions)
	return json.MarshalIndent(doc, "", "  ")
}

// ToYAML serializes the OpenAPI document to YAML.
func (g *Generator) ToYAML(definitions []*schema.ResourceDefinition) ([]byte, error) {
	doc := g.Generate(definitions)
	return yaml.Marshal(doc)
}

func mapFieldToOpenAPISchema(f schema.FieldDefinition) *Schema {
	switch f.Type {
	case schema.FieldTypeUUID:
		return &Schema{Type: "string", Format: "uuid"}
	case schema.FieldTypeString, schema.FieldTypeText:
		return &Schema{Type: "string"}
	case schema.FieldTypeInteger:
		return &Schema{Type: "integer"}
	case schema.FieldTypeFloat:
		return &Schema{Type: "number"}
	case schema.FieldTypeBoolean:
		return &Schema{Type: "boolean"}
	case schema.FieldTypeDateTime:
		return &Schema{Type: "string", Format: "date-time"}
	case schema.FieldTypeJSON:
		return &Schema{Type: "object"}
	case schema.FieldTypeReference:
		return &Schema{Type: "string", Description: fmt.Sprintf("Reference ID to %s", f.Resource)}
	case schema.FieldTypeMedia:
		return &Schema{Type: "string", Format: "uuid", Description: "Media asset ID (accessible at /media/{id})"}
	default:
		return &Schema{Type: "string"}
	}
}
