package recordset

import "reflect"

var _ Column[string] = (*column[string])(nil)
var _ Column[int] = (*column[int])(nil)
var _ Column[*int] = (*column[*int])(nil)

type column[T any] struct {
	columnBase[T]
	values []T
}

func (c *column[T]) SetValue(row int, value T) (err error) {
	c.values[row] = value
	return nil
}

func (c *column[T]) GetValue(row int) (value T, err error) {
	return c.values[row], nil
}

func (c *column[T]) IsBitmap() bool {
	return false // Bitmap cols are not implemented yet, and probably will be implemented by a dedicated type
}

func (c *column[T]) ValueType() reflect.Type {
	var t T
	return reflect.TypeOf(t)
}

func (c *column[T]) Add(value T) error {
	c.values = append(c.values, value)
	return nil
}
