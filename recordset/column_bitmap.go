package recordset

import (
	"reflect"

	"github.com/RoaringBitmap/roaring/v2"
)

type bitmapValue[T comparable] struct {
	v    T
	rows *roaring.Bitmap
}

// NewBitmapColumn returns a Column[T] that records each distinct value with the row indexes that hold it.
// initialCapacity is the initial length of that distinct-value slice.
// getDefaultVal supplies DefaultValue, and options configure the column.
func NewBitmapColumn[T comparable](name string, initialCapacity int, getDefaultVal func() T, options ...ColumnOption) Column[T] {
	c := columnBitmap[T]{
		columnBase: columnBase[T]{
			name:       name,
			defaultVal: getDefaultVal,
		},

		values: make([]bitmapValue[T], initialCapacity),
	}
	for _, o := range options {
		o(&c.ColumnOptions)
	}
	return &c
}

type columnBitmap[T comparable] struct {
	rowsCount int
	columnBase[T]
	values []bitmapValue[T]
}

// GetValue returns the first stored value whose bitmap contains row.
// It returns the zero T and a nil error when no bitmap contains row.
func (c *columnBitmap[T]) GetValue(row int) (value T, err error) {
	i := uint32(row)
	for _, val := range c.values {
		if val.rows.Contains(i) {
			return val.v, nil
		}
	}
	return
}

// SetValue assigns value to row and removes row from every other value's bitmap.
// A value that is not already stored is appended with a new bitmap containing row.
func (c *columnBitmap[T]) SetValue(row int, value T) (err error) {
	i := uint32(row)
	for _, val := range c.values {
		if val.v == value {
			val.rows.Add(i)
		} else {
			val.rows.Remove(i)
		}
	}
	for _, val := range c.values {
		if val.v == value {
			return
		}
	}
	rows := roaring.New()
	rows.Add(i)
	c.values = append(c.values, bitmapValue[T]{v: value, rows: rows})
	return
}

// ValueType returns the reflect type of the zero value of T.
func (c *columnBitmap[T]) ValueType() reflect.Type {
	var t T
	return reflect.TypeOf(t)
}

// IsBitmap reports that this column stores values with bitmaps.
func (c *columnBitmap[T]) IsBitmap() bool {
	return true
}

// Add stores value at the next row and increases the row count by one.
func (c *columnBitmap[T]) Add(value T) error {
	c.rowsCount++
	return c.SetValue(c.rowsCount-1, value)
}

// Values returns the value stored for each row below the row count.
// A row that matches no bitmap contributes the zero T.
func (c *columnBitmap[T]) Values() []T {
	result := make([]T, c.rowsCount)
	for i := 0; i < c.rowsCount; i++ {
		for _, val := range c.values {
			if val.rows.Contains(uint32(i)) {
				result[i] = val.v
				break
			}
		}
	}
	return result
}
