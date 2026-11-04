package recordset

type columnBase[T any] struct {
	name       string
	defaultVal func() T
	ColumnOptions
	rowsCount int
}

// Name returns the column name.
func (c *columnBase[T]) Name() string {
	return c.name
}

// DbType returns the database type string from the column options.
func (c *columnBase[T]) DbType() string {
	return c.dbType
}

// DefaultValue returns the result of the column's default function.
func (c *columnBase[T]) DefaultValue() T {
	return c.defaultVal()
}
