package introspection

// ColumnSchema represents metadata for a database column.
type ColumnSchema struct {
	Name         string
	DataType     string
	UDTName      string
	IsNullable   bool
	DefaultValue *string
}

// TableSchema represents metadata for a database table.
type TableSchema struct {
	Name    string
	Columns map[string]ColumnSchema
}

// DatabaseSchema holds metadata for multiple database tables.
type DatabaseSchema struct {
	Tables map[string]TableSchema
}

// NewDatabaseSchema creates an empty DatabaseSchema.
func NewDatabaseSchema() *DatabaseSchema {
	return &DatabaseSchema{
		Tables: make(map[string]TableSchema),
	}
}

// AddTable adds a TableSchema to the DatabaseSchema.
func (ds *DatabaseSchema) AddTable(table TableSchema) {
	ds.Tables[table.Name] = table
}

// GetTable retrieves a TableSchema by table name.
func (ds *DatabaseSchema) GetTable(tableName string) (TableSchema, bool) {
	t, ok := ds.Tables[tableName]
	return t, ok
}
