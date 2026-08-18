package openapi_test

import (
	"strings"
	"testing"

	"github.com/yutapok/gohcms/pkg/openapi"
	"github.com/yutapok/gohcms/pkg/schema"
)

func TestGenerator_Generate(t *testing.T) {
	def := &schema.ResourceDefinition{
		Resource: "article",
		Storage:  schema.StorageConfig{Table: "articles"},
		Lifecycle: schema.LifecycleConfig{
			Mode:          schema.LifecycleModeManaged,
			StatusColumn:  "cms_status",
			VersionColumn: "cms_version",
		},
		Fields: map[string]schema.FieldDefinition{
			"id": {
				Type:     schema.FieldTypeUUID,
				Column:   "id",
				Readonly: true,
			},
			"title": {
				Type:     schema.FieldTypeString,
				Column:   "title",
				Required: true,
			},
			"body": {
				Type:   schema.FieldTypeText,
				Column: "body",
			},
			"cover_image": {
				Type:   schema.FieldTypeMedia,
				Column: "cover_image_id",
			},
			"depends_on": {
				Type:     schema.FieldTypeReference,
				Column:   "depends_on_id",
				Resource: "article",
			},
		},
	}

	gen := openapi.NewGenerator("Test CMS API", "1.0.0", "Test Description")
	doc := gen.Generate([]*schema.ResourceDefinition{def})

	if doc.OpenAPI != "3.1.0" {
		t.Errorf("expected openapi 3.1.0, got %s", doc.OpenAPI)
	}

	// Verify SecuritySchemes
	if _, exists := doc.Components.SecuritySchemes["BearerAuth"]; !exists {
		t.Error("expected BearerAuth in security schemes")
	}
	if _, exists := doc.Components.SecuritySchemes["ApiKeyAuth"]; !exists {
		t.Error("expected ApiKeyAuth in security schemes")
	}

	// Verify components
	articleSchema, exists := doc.Components.Schemas["Article"]
	if !exists {
		t.Fatal("expected Article schema in components")
	}
	if articleSchema.Properties["title"].Type != "string" {
		t.Errorf("expected title to be string type")
	}
	if articleSchema.Properties["cover_image"].Format != "uuid" {
		t.Errorf("expected cover_image to have uuid format")
	}

	if _, exists := doc.Components.Schemas["Media"]; !exists {
		t.Error("expected Media schema in components")
	}

	// Verify paths
	if _, exists := doc.Paths["/api/article"]; !exists {
		t.Error("expected /api/article path")
	}
	if _, exists := doc.Paths["/api/media"]; !exists {
		t.Error("expected /api/media path")
	}

	// Verify JSON export
	jsonData, err := gen.ToJSON([]*schema.ResourceDefinition{def})
	if err != nil {
		t.Fatalf("failed to export JSON: %v", err)
	}
	if !strings.Contains(string(jsonData), "3.1.0") {
		t.Errorf("expected json to contain 3.1.0")
	}

	// Verify YAML export
	yamlData, err := gen.ToYAML([]*schema.ResourceDefinition{def})
	if err != nil {
		t.Fatalf("failed to export YAML: %v", err)
	}
	if !strings.Contains(string(yamlData), "openapi: 3.1.0") {
		t.Errorf("expected yaml to contain openapi: 3.1.0")
	}
}
