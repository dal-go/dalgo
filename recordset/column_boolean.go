package recordset

import (
	"reflect"

	"github.com/RoaringBitmap/roaring/v2"
)

type columnBool struct {
	columnBase[bool]
	values *roaring.Bitmap
}

// GetValue reports whether the bitmap contains row. The error result is nil.
func (c *columnBool) GetValue(row int) (value bool, err error) {
	return c.values.Contains(uint32(row)), nil
}

// SetValue adds row to the bitmap when value is true and removes it when value is false.
// When row is greater than or equal to the row count, SetValue sets the row count to row+1.
func (c *columnBool) SetValue(row int, value bool) (err error) {
	if row >= c.rowsCount {
		c.rowsCount = row + 1
	}
	if value {
		c.values.Add(uint32(row))
	} else {
		c.values.Remove(uint32(row))
	}
	return nil
}

// ValueType returns the reflect type of bool.
func (c *columnBool) ValueType() reflect.Type {
	return reflect.TypeOf(true)
}

// IsBitmap reports that this column stores booleans in a bitmap.
func (c *columnBool) IsBitmap() bool {
	return true
}

// Add sets the bit at the current cardinality, then stores value at one less than the cardinality after that update.
func (c *columnBool) Add(value bool) error {
	c.values.Add(uint32(c.values.GetCardinality()))
	return c.SetValue(int(c.values.GetCardinality()-1), value)
}

// NewBoolColumn returns a bitmap-backed column of bools named name.
// The default value is false, and options configure the column.
func NewBoolColumn(name string, options ...ColumnOption) Column[bool] {
	c := columnBool{
		columnBase: columnBase[bool]{
			name: name,
			defaultVal: func() bool {
				return false
			},
		},
		values: roaring.New(),
	}
	for _, o := range options {
		o(&c.ColumnOptions)
	}
	return &c
}

// Values returns a bool for each row index below the row count, true when the bitmap contains that index.
func (c *columnBool) Values() []bool {
	result := make([]bool, c.rowsCount)
	for i := 0; i < c.rowsCount; i++ {
		result[i] = c.values.Contains(uint32(i))
	}
	return result
}
