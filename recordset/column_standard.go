package recordset

import (
	"fmt"
	"reflect"
)

var _ Column[string] = (*column[string])(nil)
var _ Column[int] = (*column[int])(nil)
var _ Column[*int] = (*column[*int])(nil)

type column[T any] struct {
	columnBase[T]
	values []T
}

// SetValue stores value at an existing row index.
func (c *column[T]) SetValue(row int, value T) (err error) {
	c.values[row] = value
	return nil
}

// GetValue returns the value at row.
// It returns an error when row is negative or not less than the number of stored values.
func (c *column[T]) GetValue(row int) (value T, err error) {
	if row < 0 || row >= len(c.values) {
		return value, fmt.Errorf("row %d out of range for %d rows", row, len(c.values))
	}
	return c.values[row], nil
}

// IsBitmap reports that this column stores values in a slice.
func (c *column[T]) IsBitmap() bool {
	return false // Bitmap cols are not implemented yet, and probably will be implemented by a dedicated type
}

// ValueType returns the reflect type of the zero value of T.
func (c *column[T]) ValueType() reflect.Type {
	var t T
	return reflect.TypeOf(t)
}

// Add appends value to the backing slice.
func (c *column[T]) Add(value T) error {
	c.values = append(c.values, value)
	return nil
}

// Values returns a copy of the backing slice.
func (c *column[T]) Values() []T {
	values := make([]T, len(c.values))
	copy(values, c.values)
	return values
}
