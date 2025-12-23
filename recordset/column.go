package recordset

import (
	"reflect"
)

type Column[T any] interface {
	Name() string
	DefaultValue() T
	GetValue(row int) (value T, err error)
	SetValue(row int, value T) (err error)
	ValueType() reflect.Type
	IsBitmap() bool
	Add(value T) error
}

func NewColumn[T any](name string, defaultValue T, values ...T) Column[any] {
	return colAny[T]{
		column: &column[T]{
			columnBase: columnBase[T]{
				name: name,
				defaultVal: func() T {
					return defaultValue
				},
			},
			values: values,
		},
	}
}
