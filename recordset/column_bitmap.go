package recordset

import (
	"reflect"

	"github.com/RoaringBitmap/roaring/v2"
)

type bitmapValue[T comparable] struct {
	v    T
	rows *roaring.Bitmap
}

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

func (c *columnBitmap[T]) GetValue(row int) (value T, err error) {
	i := uint32(row)
	for _, val := range c.values {
		if val.rows.Contains(i) {
			return val.v, nil
		}
	}
	return
}

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

func (c *columnBitmap[T]) ValueType() reflect.Type {
	var t T
	return reflect.TypeOf(t)
}

func (c *columnBitmap[T]) IsBitmap() bool {
	return true
}

func (c *columnBitmap[T]) Add(value T) error {
	c.rowsCount++
	return c.SetValue(c.rowsCount-1, value)
}

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
