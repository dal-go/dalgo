package recordset

import (
	"fmt"
	"reflect"
)

var _ Column[any] = &UntypedColWrapper[string]{}

// UntypedCol wraps c as a Column[any].
func UntypedCol[T any](c Column[T]) Column[any] {
	return UntypedColWrapper[T]{column: c}
}

// UntypedColWrapper adapts a Column[T] so it can be used as a Column[any].
type UntypedColWrapper[T any] struct {
	column Column[T]
}

// TypedColumn returns the wrapped Column[T].
func (c UntypedColWrapper[T]) TypedColumn() Column[T] {
	return c.column
}

// Name returns the wrapped column's name.
func (c UntypedColWrapper[T]) Name() string {
	return c.column.Name()
}

// DbType returns the wrapped column's database type string.
func (c UntypedColWrapper[T]) DbType() string {
	return c.column.DbType()
}

// Add appends value when its dynamic type is T.
// Otherwise Add returns an error and leaves the wrapped column unchanged.
func (c UntypedColWrapper[T]) Add(value any) error {
	if v, ok := value.(T); ok {
		return c.column.Add(v)
	}
	return fmt.Errorf("cannot add value of type %T to column %s", value, c.Name())
}

// GetValue returns the wrapped column's value at row.
func (c UntypedColWrapper[T]) GetValue(row int) (value any, err error) {
	return c.column.GetValue(row)
}

// DefaultValue returns the wrapped column's default value.
func (c UntypedColWrapper[T]) DefaultValue() (value any) {
	return c.column.DefaultValue()
}

// SetValue stores value at row. It panics when value's dynamic type is not T.
func (c UntypedColWrapper[T]) SetValue(row int, value any) (err error) {
	return c.column.SetValue(row, value.(T))
}

// ValueType returns the wrapped column's value type.
func (c UntypedColWrapper[T]) ValueType() reflect.Type {
	return c.column.ValueType()
}

// IsBitmap reports whether the wrapped column stores values in a bitmap.
func (c UntypedColWrapper[T]) IsBitmap() bool {
	return c.column.IsBitmap()
}

// Values returns a new slice of the wrapped column's values.
func (c UntypedColWrapper[T]) Values() []any {
	values := c.column.Values()
	result := make([]any, len(values))
	for i, v := range values {
		result[i] = v
	}
	return result
}
