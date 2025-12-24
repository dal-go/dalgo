package recordset

type ColumnOption = func(o *ColumnOptions)

type ColumnOptions struct {
	dbType string
	format func(value any) string
}

func Format(format func(value any) string) func(o *ColumnOptions) {
	return func(o *ColumnOptions) {
		o.format = format
	}
}

func ColDbType(dbType string) func(o *ColumnOptions) {
	return func(o *ColumnOptions) {
		o.dbType = dbType
	}
}
