package recordset

import (
	"reflect"
)

// Column is one typed field of a recordset, with a value for each row.
type Column[T any] interface {
	// Name returns the column name.
	Name() string
	// DefaultValue returns the column's default value.
	DefaultValue() T
	// GetValue returns the value at row.
	GetValue(row int) (value T, err error)
	// SetValue stores value at row.
	SetValue(row int, value T) (err error)
	// DbType returns the database type string configured for the column.
	DbType() string
	// ValueType returns the reflect type of the column's values.
	ValueType() reflect.Type
	// IsBitmap reports whether the column stores values in a bitmap.
	IsBitmap() bool
	// Add adds value to the column.
	Add(value T) error
	// Values returns the column's values in row order.
	Values() []T
}

// NewTypedColumn returns a slice-backed Column[T] named name.
// defaultValue is returned by DefaultValue, and options configure the column.
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

// NewColumn returns a Column[any] wrapping a slice-backed column of T named name.
// defaultValue is the column default, and options configure the column.
func NewColumn[T any](name string, defaultValue T, options ...ColumnOption) Column[any] {
	return UntypedCol(NewTypedColumn(name, defaultValue, options...))
}
