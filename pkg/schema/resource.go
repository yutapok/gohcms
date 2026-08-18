package schema

import (
	"fmt"
)

// FieldType represents the supported field types in a Resource Definition.
type FieldType string

const (
	FieldTypeUUID      FieldType = "uuid"
	FieldTypeString    FieldType = "string"
	FieldTypeText      FieldType = "text"
	FieldTypeInteger   FieldType = "integer"
	FieldTypeFloat     FieldType = "float"
	FieldTypeBoolean   FieldType = "boolean"
	FieldTypeDateTime  FieldType = "datetime"
	FieldTypeEnum      FieldType = "enum"
	FieldTypeJSON      FieldType = "json"
	FieldTypeReference FieldType = "reference"
	FieldTypeMedia     FieldType = "media"
)

// LifecycleMode represents the lifecycle management mode.
type LifecycleMode string

const (
	LifecycleModeNone    LifecycleMode = "none"
	LifecycleModeManaged LifecycleMode = "managed"
)

// OrderedField represents a field entry with preserved definition order.
type OrderedField struct {
	Name       string
	Definition FieldDefinition
}

// ResourceDefinition is the top-level specification for a CMS resource.
type ResourceDefinition struct {
	Resource   string                     `yaml:"resource"`
	Storage    StorageConfig              `yaml:"storage"`
	Lifecycle  LifecycleConfig            `yaml:"lifecycle"`
	Fields     map[string]FieldDefinition `yaml:"fields"`
	FieldOrder []string                   `yaml:"-"` // Preserved order from YAML
}

// OrderedFields returns the list of fields in the order they were defined in YAML.
func (r *ResourceDefinition) OrderedFields() []OrderedField {
	if len(r.FieldOrder) == 0 {
		var list []OrderedField
		for name, def := range r.Fields {
			list = append(list, OrderedField{Name: name, Definition: def})
		}
		return list
	}

	var list []OrderedField
	for _, name := range r.FieldOrder {
		if def, ok := r.Fields[name]; ok {
			list = append(list, OrderedField{Name: name, Definition: def})
		}
	}
	return list
}

// StorageConfig defines the database table mapping.
type StorageConfig struct {
	Table string `yaml:"table"`
}

// LifecycleConfig defines the lifecycle columns and behavior.
type LifecycleConfig struct {
	Mode          LifecycleMode `yaml:"mode"`
	StatusColumn  string        `yaml:"status_column,omitempty"`
	VersionColumn string        `yaml:"version_column,omitempty"`
}

// FieldDefinition defines the properties of a single resource field.
type FieldDefinition struct {
	Type     FieldType `yaml:"type"`
	Column   string    `yaml:"column"`
	Required bool      `yaml:"required,omitempty"`
	Readonly bool      `yaml:"readonly,omitempty"`
	Resource string    `yaml:"resource,omitempty"` // For reference type
}

// Validate validates the structure of the ResourceDefinition itself.
func (r *ResourceDefinition) Validate() error {
	if r.Resource == "" {
		return fmt.Errorf("resource name is required")
	}
	if r.Storage.Table == "" {
		return fmt.Errorf("storage.table is required for resource '%s'", r.Resource)
	}
	if r.Lifecycle.Mode == LifecycleModeManaged {
		if r.Lifecycle.StatusColumn == "" {
			return fmt.Errorf("lifecycle.status_column is required when mode is managed for resource '%s'", r.Resource)
		}
		if r.Lifecycle.VersionColumn == "" {
			return fmt.Errorf("lifecycle.version_column is required when mode is managed for resource '%s'", r.Resource)
		}
	}
	if len(r.Fields) == 0 {
		return fmt.Errorf("at least one field is required for resource '%s'", r.Resource)
	}
	for name, field := range r.Fields {
		if field.Type == "" {
			return fmt.Errorf("field '%s' must have a type in resource '%s'", name, r.Resource)
		}
		if field.Column == "" {
			return fmt.Errorf("field '%s' must have a column in resource '%s'", name, r.Resource)
		}
	}
	return nil
}
