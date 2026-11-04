package recordset

import (
	"fmt"
	"reflect"
)

// ComputedColumn is a column whose value is derived from the stored columns of a
// row via an Evaluator, rather than from per-row stored data.
type ComputedColumn interface {
	Column[any]
	// Evaluator returns the Evaluator that derives this column's value.
	Evaluator() Evaluator
}

// computedColumn implements both Column[any] and ComputedColumn.
// It has no per-row stored backing: resolution is the Row's job.
type computedColumn struct {
	name      string
	evaluator Evaluator
	ColumnOptions
}

// NewComputedColumn returns a column that derives its value via the supplied
// Evaluator. The returned value implements both Column[any] and ComputedColumn.
func NewComputedColumn(name string, evaluator Evaluator, options ...ColumnOption) Column[any] {
	c := &computedColumn{
		name:      name,
		evaluator: evaluator,
	}
	for _, o := range options {
		o(&c.ColumnOptions)
	}
	return c
}

var _ ComputedColumn = (*computedColumn)(nil)

// Evaluator returns the evaluator supplied to NewComputedColumn.
func (c *computedColumn) Evaluator() Evaluator {
	return c.evaluator
}

// Name returns the column name.
func (c *computedColumn) Name() string {
	return c.name
}

// DbType returns the database type string from the column options.
func (c *computedColumn) DbType() string {
	return c.dbType
}

// DefaultValue returns nil.
func (c *computedColumn) DefaultValue() any {
	return nil
}

// GetValue fails loud: a computed column must be resolved via the Row, which
// supplies the stored sibling values to the Evaluator.
func (c *computedColumn) GetValue(int) (any, error) {
	return nil, fmt.Errorf("computed column %q must be resolved via Row", c.name)
}

// SetValue returns an error because a computed column has no stored value.
func (c *computedColumn) SetValue(int, any) error {
	return fmt.Errorf("cannot set value on computed column %q", c.name)
}

// ValueType returns the reflect type of any.
func (c *computedColumn) ValueType() reflect.Type {
	return reflect.TypeOf((*any)(nil)).Elem()
}

// IsBitmap reports that a computed column has no bitmap storage.
func (c *computedColumn) IsBitmap() bool {
	return false
}

// Add returns nil and does not store its argument.
func (c *computedColumn) Add(any) error {
	return nil
}

// Values returns nil.
func (c *computedColumn) Values() []any {
	return nil
}
