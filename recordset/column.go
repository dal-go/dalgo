package recordset

import (
	"reflect"
)

type Column[T any] interface {
	Name() string
	DefaultValue() T
	GetValue(row int) (value T, err error)
	SetValue(row int, value T) (err error)
	DbType() string
	ValueType() reflect.Type
	IsBitmap() bool
	Add(value T) error
	Values() []T
}

func NewTypedColumn[T any](name string, defaultValue T, options ...ColumnOption) Column[T] {
	c := column[T]{
		columnBase: columnBase[T]{
			name: name,
			defaultVal: func() T {
				return defaultValue
			},
		},
	}
	for _, o := range options {
		o(&c.ColumnOptions)
	}
	return &c
}

func NewColumn[T any](name string, defaultValue T, options ...ColumnOption) Column[any] {
	return UntypedCol(NewTypedColumn(name, defaultValue, options...))
}
