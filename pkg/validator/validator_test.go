package validator_test

import (
	"strings"
	"testing"

	"github.com/yutapok/gohcms/pkg/introspection"
	"github.com/yutapok/gohcms/pkg/schema"
	"github.com/yutapok/gohcms/pkg/validator"
)

func createSampleArticleDefinition() *schema.ResourceDefinition {
	return &schema.ResourceDefinition{
		Resource: "article",
		Storage: schema.StorageConfig{
			Table: "articles",
		},
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
			"category_id": {
				Type:   schema.FieldTypeUUID,
				Column: "category_id",
			},
		},
	}
}

func createMatchingDatabaseSchema() *introspection.DatabaseSchema {
	dbSchema := introspection.NewDatabaseSchema()
	dbSchema.AddTable(introspection.TableSchema{
		Name: "articles",
		Columns: map[string]introspection.ColumnSchema{
			"id": {
				Name:       "id",
				DataType:   "uuid",
				UDTName:    "uuid",
				IsNullable: false,
			},
			"title": {
				Name:       "title",
				DataType:   "character varying",
				UDTName:    "varchar",
				IsNullable: false,
			},
			"body": {
				Name:       "body",
				DataType:   "text",
				UDTName:    "text",
				IsNullable: true,
			},
			"category_id": {
				Name:       "category_id",
				DataType:   "uuid",
				UDTName:    "uuid",
				IsNullable: true,
			},
			"cms_status": {
				Name:       "cms_status",
				DataType:   "text",
				UDTName:    "text",
				IsNullable: false,
			},
			"cms_version": {
				Name:       "cms_version",
				DataType:   "bigint",
				UDTName:    "int8",
				IsNullable: false,
			},
			"created_at": {
				Name:       "created_at",
				DataType:   "timestamp with time zone",
				UDTName:    "timestamptz",
				IsNullable: false,
			},
			"updated_at": {
				Name:       "updated_at",
				DataType:   "timestamp with time zone",
				UDTName:    "timestamptz",
				IsNullable: false,
			},
		},
	})
	return dbSchema
}

// 1. 正常系 (Success)
func TestValidator_Success(t *testing.T) {
	v := validator.New()
	def := createSampleArticleDefinition()
	dbSchema := createMatchingDatabaseSchema()

	result := v.Validate(def, dbSchema)

	if !result.IsValid() {
		t.Fatalf("expected validation to pass, got errors: %v", result.Errors)
	}

	report := result.FormatReport()
	if !strings.Contains(report, "✓ article.title -> articles.title") {
		t.Errorf("expected report to contain passed title field, got:\n%s", report)
	}
	if !strings.Contains(report, "✓ article.id -> articles.id") {
		t.Errorf("expected report to contain passed id field, got:\n%s", report)
	}
}

// 2. 型不整合 (Type Mismatch)
func TestValidator_TypeMismatch(t *testing.T) {
	v := validator.New()
	def := createSampleArticleDefinition()
	dbSchema := createMatchingDatabaseSchema()

	// Modify body column to integer in database
	table := dbSchema.Tables["articles"]
	table.Columns["body"] = introspection.ColumnSchema{
		Name:     "body",
		DataType: "integer",
		UDTName:  "int4",
	}
	dbSchema.AddTable(table)

	result := v.Validate(def, dbSchema)

	if result.IsValid() {
		t.Fatal("expected validation to fail for type mismatch, but it passed")
	}

	found := false
	for _, err := range result.Errors {
		if err.Field == "body" && strings.Contains(err.Message, "type mismatch") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected type mismatch error for field 'body', got errors: %v", result.Errors)
	}

	report := result.FormatReport()
	if !strings.Contains(report, "ERROR:") {
		t.Errorf("expected report to contain 'ERROR:', got:\n%s", report)
	}
}

// 3. カラム欠落 (Missing Column)
func TestValidator_MissingColumn(t *testing.T) {
	v := validator.New()
	def := createSampleArticleDefinition()
	dbSchema := createMatchingDatabaseSchema()

	// Delete category_id column from database
	table := dbSchema.Tables["articles"]
	delete(table.Columns, "category_id")
	dbSchema.AddTable(table)

	result := v.Validate(def, dbSchema)

	if result.IsValid() {
		t.Fatal("expected validation to fail for missing column, but it passed")
	}

	found := false
	for _, err := range result.Errors {
		if err.Field == "category_id" && strings.Contains(err.Message, "does not exist in table") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected missing column error for field 'category_id', got errors: %v", result.Errors)
	}
}

// 4. テーブル欠落 (Missing Table)
func TestValidator_MissingTable(t *testing.T) {
	v := validator.New()
	def := createSampleArticleDefinition()
	dbSchema := introspection.NewDatabaseSchema() // Empty schema without 'articles' table

	result := v.Validate(def, dbSchema)

	if result.IsValid() {
		t.Fatal("expected validation to fail for missing table, but it passed")
	}

	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "does not exist in database") {
		t.Errorf("expected missing table error, got: %v", result.Errors)
	}
}

// 5. Lifecycle カラム不整合 (Missing / Invalid Lifecycle Column)
func TestValidator_LifecycleErrors(t *testing.T) {
	v := validator.New()

	t.Run("missing status column", func(t *testing.T) {
		def := createSampleArticleDefinition()
		dbSchema := createMatchingDatabaseSchema()
		table := dbSchema.Tables["articles"]
		delete(table.Columns, "cms_status")
		dbSchema.AddTable(table)

		res := v.Validate(def, dbSchema)
		if res.IsValid() {
			t.Fatal("expected error for missing cms_status")
		}
	})

	t.Run("invalid version column type", func(t *testing.T) {
		def := createSampleArticleDefinition()
		dbSchema := createMatchingDatabaseSchema()
		table := dbSchema.Tables["articles"]
		table.Columns["cms_version"] = introspection.ColumnSchema{
			Name:     "cms_version",
			DataType: "text",
			UDTName:  "text",
		}
		dbSchema.AddTable(table)

		res := v.Validate(def, dbSchema)
		if res.IsValid() {
			t.Fatal("expected error for invalid cms_version type")
		}
	})
}
