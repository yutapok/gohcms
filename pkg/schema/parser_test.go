package schema_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yutapok/gohcms/pkg/schema"
)

const sampleArticleYAML = `
resource: article
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
  category_id:
    type: uuid
    column: category_id
`

func TestParse_Success(t *testing.T) {
	def, err := schema.Parse([]byte(sampleArticleYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if def.Resource != "article" {
		t.Errorf("expected resource 'article', got '%s'", def.Resource)
	}
	if def.Storage.Table != "articles" {
		t.Errorf("expected table 'articles', got '%s'", def.Storage.Table)
	}
	if def.Lifecycle.Mode != schema.LifecycleModeManaged {
		t.Errorf("expected lifecycle mode 'managed', got '%s'", def.Lifecycle.Mode)
	}
	if def.Lifecycle.StatusColumn != "cms_status" {
		t.Errorf("expected status_column 'cms_status', got '%s'", def.Lifecycle.StatusColumn)
	}
	if def.Lifecycle.VersionColumn != "cms_version" {
		t.Errorf("expected version_column 'cms_version', got '%s'", def.Lifecycle.VersionColumn)
	}

	if len(def.Fields) != 4 {
		t.Fatalf("expected 4 fields, got %d", len(def.Fields))
	}

	titleField, exists := def.Fields["title"]
	if !exists {
		t.Fatal("expected field 'title' to exist")
	}
	if titleField.Type != schema.FieldTypeString {
		t.Errorf("expected field type 'string', got '%s'", titleField.Type)
	}
	if titleField.Column != "title" {
		t.Errorf("expected column 'title', got '%s'", titleField.Column)
	}
	if !titleField.Required {
		t.Errorf("expected required to be true")
	}

	idField := def.Fields["id"]
	if !idField.Readonly {
		t.Errorf("expected id field readonly to be true")
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	invalidYAML := `
resource: article
  invalid indentation
`
	_, err := schema.Parse([]byte(invalidYAML))
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestParse_ValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "missing resource name",
			yaml: `
storage:
  table: articles
fields:
  title:
    type: string
    column: title
`,
		},
		{
			name: "missing table name",
			yaml: `
resource: article
storage:
  table: ""
fields:
  title:
    type: string
    column: title
`,
		},
		{
			name: "managed lifecycle missing status column",
			yaml: `
resource: article
storage:
  table: articles
lifecycle:
  mode: managed
  version_column: cms_version
fields:
  title:
    type: string
    column: title
`,
		},
		{
			name: "no fields defined",
			yaml: `
resource: article
storage:
  table: articles
`,
		},
		{
			name: "field missing type",
			yaml: `
resource: article
storage:
  table: articles
fields:
  title:
    column: title
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := schema.Parse([]byte(tt.yaml))
			if err == nil {
				t.Fatalf("expected validation error for case '%s', got nil", tt.name)
			}
		})
	}
}

func TestLoadDirectory(t *testing.T) {
	tempDir := t.TempDir()

	file1 := filepath.Join(tempDir, "article.yaml")
	if err := os.WriteFile(file1, []byte(sampleArticleYAML), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	defs, err := schema.LoadDirectory(tempDir)
	if err != nil {
		t.Fatalf("unexpected error loading directory: %v", err)
	}

	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
}
