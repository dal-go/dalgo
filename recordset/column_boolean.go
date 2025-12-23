package recordset

import (
	"reflect"

	"github.com/RoaringBitmap/roaring"
)

type columnBool struct {
	columnBase[bool]
	values *roaring.Bitmap
}

func (c *columnBool) GetValue(row int) (value bool, err error) {
	return c.values.Contains(uint32(row)), nil
}

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

func (c *columnBool) ValueType() reflect.Type {
	return reflect.TypeOf(true)
}

func (c *columnBool) IsBitmap() bool {
	return true
}

func (c *columnBool) Add(value bool) error {
	c.values.Add(uint32(c.values.GetCardinality()))
	return c.SetValue(int(c.values.GetCardinality()-1), value)
}

func NewBoolColumn(name string) Column[bool] {
	return &columnBool{
		columnBase: columnBase[bool]{
			name: name,
			defaultVal: func() bool {
				return false
			},
		},
		values: roaring.New(),
	}
}

func (c *columnBool) Values() []bool {
	result := make([]bool, c.rowsCount)
	for i := 0; i < c.rowsCount; i++ {
		result[i] = c.values.Contains(uint32(i))
	}
	return result
}
