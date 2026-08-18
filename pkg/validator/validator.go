package validator

import (
	"fmt"

	"github.com/yutapok/gohcms/pkg/introspection"
	"github.com/yutapok/gohcms/pkg/schema"
)

// Validator validates ResourceDefinitions against a DatabaseSchema.
type Validator struct{}

// New creates a new Validator.
func New() *Validator {
	return &Validator{}
}

// Validate validates a single ResourceDefinition against a DatabaseSchema.
func (v *Validator) Validate(def *schema.ResourceDefinition, dbSchema *introspection.DatabaseSchema) *ValidationResult {
	result := &ValidationResult{}

	table, exists := dbSchema.GetTable(def.Storage.Table)
	if !exists {
		result.AddError(ValidationError{
			Resource: def.Resource,
			Table:    def.Storage.Table,
			Message:  fmt.Sprintf("table '%s' does not exist in database", def.Storage.Table),
		})
		return result
	}

	result.AddPassed(ValidationPassed{
		Resource: def.Resource,
		Table:    def.Storage.Table,
		Details:  "table exists",
	})

	// 1. Validate fields
	for fieldName, field := range def.Fields {
		col, colExists := table.Columns[field.Column]
		if !colExists {
			result.AddError(ValidationError{
				Resource: def.Resource,
				Table:    def.Storage.Table,
				Field:    fieldName,
				Column:   field.Column,
				Message:  fmt.Sprintf("column '%s' does not exist in table '%s'", field.Column, def.Storage.Table),
			})
			continue
		}

		if !IsTypeCompatible(field.Type, col.DataType, col.UDTName) {
			result.AddError(ValidationError{
				Resource: def.Resource,
				Table:    def.Storage.Table,
				Field:    fieldName,
				Column:   field.Column,
				Message:  fmt.Sprintf("type mismatch: field expects '%s', actual database type is '%s' (udt: %s)", field.Type, col.DataType, col.UDTName),
			})
			continue
		}

		typeDetails := col.DataType
		if !col.IsNullable {
			typeDetails += " NOT NULL"
		}
		result.AddPassed(ValidationPassed{
			Resource: def.Resource,
			Table:    def.Storage.Table,
			Field:    fieldName,
			Column:   field.Column,
			Details:  typeDetails,
		})
	}

	// 2. Validate lifecycle columns if mode is managed
	if def.Lifecycle.Mode == schema.LifecycleModeManaged {
		statusCol, statusExists := table.Columns[def.Lifecycle.StatusColumn]
		if !statusExists {
			result.AddError(ValidationError{
				Resource: def.Resource,
				Table:    def.Storage.Table,
				Column:   def.Lifecycle.StatusColumn,
				Message:  fmt.Sprintf("lifecycle status column '%s' does not exist in table '%s'", def.Lifecycle.StatusColumn, def.Storage.Table),
			})
		} else if !IsStatusColumnCompatible(statusCol.DataType, statusCol.UDTName) {
			result.AddError(ValidationError{
				Resource: def.Resource,
				Table:    def.Storage.Table,
				Column:   def.Lifecycle.StatusColumn,
				Message:  fmt.Sprintf("lifecycle status column '%s' must be a string type (text/varchar), actual database type is '%s'", def.Lifecycle.StatusColumn, statusCol.DataType),
			})
		} else {
			result.AddPassed(ValidationPassed{
				Resource: def.Resource,
				Table:    def.Storage.Table,
				Column:   def.Lifecycle.StatusColumn,
				Details:  fmt.Sprintf("lifecycle status column (%s)", statusCol.DataType),
			})
		}

		versionCol, versionExists := table.Columns[def.Lifecycle.VersionColumn]
		if !versionExists {
			result.AddError(ValidationError{
				Resource: def.Resource,
				Table:    def.Storage.Table,
				Column:   def.Lifecycle.VersionColumn,
				Message:  fmt.Sprintf("lifecycle version column '%s' does not exist in table '%s'", def.Lifecycle.VersionColumn, def.Storage.Table),
			})
		} else if !IsVersionColumnCompatible(versionCol.DataType, versionCol.UDTName) {
			result.AddError(ValidationError{
				Resource: def.Resource,
				Table:    def.Storage.Table,
				Column:   def.Lifecycle.VersionColumn,
				Message:  fmt.Sprintf("lifecycle version column '%s' must be an integer type (bigint/integer), actual database type is '%s'", def.Lifecycle.VersionColumn, versionCol.DataType),
			})
		} else {
			result.AddPassed(ValidationPassed{
				Resource: def.Resource,
				Table:    def.Storage.Table,
				Column:   def.Lifecycle.VersionColumn,
				Details:  fmt.Sprintf("lifecycle version column (%s)", versionCol.DataType),
			})
		}
	}

	return result
}

// ValidateAll validates multiple ResourceDefinitions against a DatabaseSchema.
func (v *Validator) ValidateAll(definitions []*schema.ResourceDefinition, dbSchema *introspection.DatabaseSchema) *ValidationResult {
	combined := &ValidationResult{}
	for _, def := range definitions {
		res := v.Validate(def, dbSchema)
		combined.Merge(res)
	}
	return combined
}
