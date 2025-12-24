package recordset

import (
	"fmt"
	"reflect"
)

var _ Column[any] = &UntypedColWrapper[string]{}

func UntypedCol[T any](c Column[T]) Column[any] {
	return UntypedColWrapper[T]{column: c}
}

type UntypedColWrapper[T any] struct {
	column Column[T]
}

func (c UntypedColWrapper[T]) TypedColumn() Column[T] {
	return c.column
}

func (c UntypedColWrapper[T]) Name() string {
	return c.column.Name()
}
func (c UntypedColWrapper[T]) DbType() string {
	return c.column.DbType()
}

func (c UntypedColWrapper[T]) Add(value any) error {
	if v, ok := value.(T); ok {
		return c.column.Add(v)
	}
	return fmt.Errorf("cannot add value of type %T to column %s", value, c.Name())
}

func (c UntypedColWrapper[T]) GetValue(row int) (value any, err error) {
	return c.column.GetValue(row)
}

func (c UntypedColWrapper[T]) DefaultValue() (value any) {
	return c.column.DefaultValue()
}

func (c UntypedColWrapper[T]) SetValue(row int, value any) (err error) {
	return c.column.SetValue(row, value.(T))
}

func (c UntypedColWrapper[T]) ValueType() reflect.Type {
	return c.column.ValueType()
}

func (c UntypedColWrapper[T]) IsBitmap() bool {
	return c.column.IsBitmap()
}

func (c UntypedColWrapper[T]) Values() []any {
	values := c.column.Values()
	result := make([]any, len(values))
	for i, v := range values {
		result[i] = v
	}
	return result
}
