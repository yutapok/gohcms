package validator

import (
	"strings"

	"github.com/yutapok/gohcms/pkg/schema"
)

// IsTypeCompatible checks whether a PostgreSQL data type or UDT is compatible with a Resource FieldType.
func IsTypeCompatible(fieldType schema.FieldType, pgDataType string, pgUDTName string) bool {
	dataType := strings.ToLower(pgDataType)
	udtName := strings.ToLower(pgUDTName)

	switch fieldType {
	case schema.FieldTypeUUID:
		return dataType == "uuid" || udtName == "uuid"

	case schema.FieldTypeString:
		return dataType == "character varying" || dataType == "varchar" || dataType == "text" || dataType == "character" || dataType == "char" || udtName == "varchar" || udtName == "text"

	case schema.FieldTypeText:
		return dataType == "text" || udtName == "text" || dataType == "character varying" || dataType == "varchar"

	case schema.FieldTypeInteger:
		return dataType == "integer" || dataType == "bigint" || dataType == "smallint" || udtName == "int4" || udtName == "int8" || udtName == "int2"

	case schema.FieldTypeFloat:
		return dataType == "double precision" || dataType == "real" || dataType == "numeric" || dataType == "decimal" || udtName == "float8" || udtName == "float4" || udtName == "numeric"

	case schema.FieldTypeBoolean:
		return dataType == "boolean" || udtName == "bool"

	case schema.FieldTypeDateTime:
		return strings.HasPrefix(dataType, "timestamp") || udtName == "timestamptz" || udtName == "timestamp"

	case schema.FieldTypeJSON:
		return dataType == "json" || dataType == "jsonb" || udtName == "json" || udtName == "jsonb"

	case schema.FieldTypeEnum:
		return dataType == "user-defined" || dataType == "text" || dataType == "character varying" || dataType == "varchar"

	case schema.FieldTypeReference:
		// References typically map to UUID, BIGINT, or INTEGER foreign keys.
		return dataType == "uuid" || dataType == "bigint" || dataType == "integer" || udtName == "uuid" || udtName == "int8" || udtName == "int4"

	case schema.FieldTypeMedia:
		// Media fields map to UUID or string foreign keys (references cms_media.id)
		return dataType == "uuid" || dataType == "character varying" || dataType == "varchar" || dataType == "text" || udtName == "uuid" || udtName == "varchar" || udtName == "text"

	default:
		return false
	}
}

// IsStatusColumnCompatible checks if a column is suitable for lifecycle status.
func IsStatusColumnCompatible(pgDataType string, pgUDTName string) bool {
	return IsTypeCompatible(schema.FieldTypeString, pgDataType, pgUDTName)
}

// IsVersionColumnCompatible checks if a column is suitable for lifecycle version.
func IsVersionColumnCompatible(pgDataType string, pgUDTName string) bool {
	return IsTypeCompatible(schema.FieldTypeInteger, pgDataType, pgUDTName)
}
