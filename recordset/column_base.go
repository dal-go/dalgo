package recordset

type columnBase[T any] struct {
	name       string
	defaultVal func() T
	ColumnOptions
	rowsCount int
}

func (c *columnBase[T]) Name() string {
	return c.name
}

func (c *columnBase[T]) DefaultValue() T {
	return c.defaultVal()
}
