package recordset

type columnBase[T any] struct {
	name       string
	defaultVal func() T
}

func (c *columnBase[T]) Name() string {
	return c.name
}

func (c *columnBase[T]) DefaultValue() T {
	return c.defaultVal()
}
