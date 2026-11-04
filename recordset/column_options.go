package recordset

// ColumnOption applies a setting to ColumnOptions.
type ColumnOption = func(o *ColumnOptions)

// ColumnOptions holds the database type and format function selected for a column.
type ColumnOptions struct {
	dbType string
	format func(value any) string
}

// Format returns a ColumnOption that stores format on ColumnOptions.
func Format(format func(value any) string) func(o *ColumnOptions) {
	return func(o *ColumnOptions) {
		o.format = format
	}
}

// ColDbType returns a ColumnOption that sets the database type string read by DbType.
func ColDbType(dbType string) func(o *ColumnOptions) {
	return func(o *ColumnOptions) {
		o.dbType = dbType
	}
}
