package recordset

import (
	"reflect"

	"github.com/RoaringBitmap/roaring"
)

type bitmapValue[T comparable] struct {
	v    T
	rows *roaring.Bitmap
}

func NewBitmapColumn[T comparable](name string, initialCapacity int) Column[T] {
	return &columnBitmap[T]{
		columnBase: columnBase[T]{name: name},
		values:     make([]bitmapValue[T], initialCapacity),
	}
}

type columnBitmap[T comparable] struct {
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
	// Find max row index
	maxRow := -1
	for _, val := range c.values {
		if !val.rows.IsEmpty() {
			last := int(val.rows.Maximum())
			if last > maxRow {
				maxRow = last
			}
		}
	}
	return c.SetValue(maxRow+1, value)
}
