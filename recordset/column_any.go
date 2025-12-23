package recordset

import (
	"fmt"
	"reflect"
)

var _ Column[any] = &colAny[string]{}

type colAny[T any] struct {
	column Column[T]
}

func (c colAny[T]) Name() string {
	return c.column.Name()
}

func (c colAny[T]) Add(value any) error {
	if v, ok := value.(T); ok {
		return c.column.Add(v)
	}
	return fmt.Errorf("cannot add value of type %T to column %s", value, c.Name())
}

func (c colAny[T]) GetValue(row int) (value any, err error) {
	return c.column.GetValue(row)
}

func (c colAny[T]) DefaultValue() (value any) {
	return c.column.DefaultValue()
}

func (c colAny[T]) SetValue(row int, value any) (err error) {
	return c.column.SetValue(row, value.(T))
}

func (c colAny[T]) ValueType() reflect.Type {
	return c.column.ValueType()
}

func (c colAny[T]) IsBitmap() bool {
	return c.column.IsBitmap()
}
