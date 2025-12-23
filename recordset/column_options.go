package recordset

type ColumnOption = func(o *ColumnOptions)

type ColumnOptions struct {
	format func(value any) string
}

func Format(format func(value any) string) func(o *ColumnOptions) {
	return func(o *ColumnOptions) {
		o.format = format
	}
}
