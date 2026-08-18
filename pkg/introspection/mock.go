package introspection

import "github.com/yutapok/gohcms/pkg/schema"

// BuildMockDatabaseSchema generates a synthetic DatabaseSchema matching given ResourceDefinitions.
// Useful for in-memory demo mode and local unit/integration tests without PostgreSQL.
func BuildMockDatabaseSchema(definitions []*schema.ResourceDefinition) *DatabaseSchema {
	dbSchema := NewDatabaseSchema()
	for _, def := range definitions {
		table := TableSchema{
			Name:    def.Storage.Table,
			Columns: make(map[string]ColumnSchema),
		}
		for _, f := range def.Fields {
			dt, udt := mapFieldTypeToMockPG(f.Type)
			table.Columns[f.Column] = ColumnSchema{
				Name:       f.Column,
				DataType:   dt,
				UDTName:    udt,
				IsNullable: !f.Required,
			}
		}
		if def.Lifecycle.Mode == schema.LifecycleModeManaged {
			table.Columns[def.Lifecycle.StatusColumn] = ColumnSchema{
				Name:     def.Lifecycle.StatusColumn,
				DataType: "text",
				UDTName:  "text",
			}
			table.Columns[def.Lifecycle.VersionColumn] = ColumnSchema{
				Name:     def.Lifecycle.VersionColumn,
				DataType: "bigint",
				UDTName:  "int8",
			}
		}
		dbSchema.AddTable(table)
	}
	return dbSchema
}

func mapFieldTypeToMockPG(ft schema.FieldType) (string, string) {
	switch ft {
	case schema.FieldTypeUUID, schema.FieldTypeReference, schema.FieldTypeMedia:
		return "uuid", "uuid"
	case schema.FieldTypeString:
		return "character varying", "varchar"
	case schema.FieldTypeText:
		return "text", "text"
	case schema.FieldTypeInteger:
		return "bigint", "int8"
	case schema.FieldTypeFloat:
		return "double precision", "float8"
	case schema.FieldTypeBoolean:
		return "boolean", "bool"
	case schema.FieldTypeDateTime:
		return "timestamp with time zone", "timestamptz"
	case schema.FieldTypeJSON:
		return "jsonb", "jsonb"
	default:
		return "text", "text"
	}
}
