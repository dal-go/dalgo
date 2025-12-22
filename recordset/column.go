package recordset

import (
	"fmt"
	"reflect"
)

type Column[T any] interface {
	Name() string
	DefaultValue() T
	Add(value T) error
	GetValue(row int) (value T, err error)
	SetValue(row int, value T) (err error)
	ValueType() reflect.Type
	IsBitmap() bool
}

func NewColumn[T any](name string, defaultValue T, values ...T) Column[any] {
	return colAny[T]{
		column: &column[T]{
			name: name,
			defaultVal: func() T {
				return defaultValue
			},
			values: values,
		},
	}
}

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

type column[T any] struct {
	name       string
	defaultVal func() T
	values     []T
}

func (c *column[T]) DefaultValue() T {
	return c.defaultVal()
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
	//TODO implement me using reflection
	panic("implement me")
}

func (c *column[T]) Name() string {
	return c.name
}

func (c *column[T]) Add(value T) error {
	c.values = append(c.values, value)
	return nil
}
