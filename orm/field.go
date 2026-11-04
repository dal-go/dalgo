package orm

import (
	"github.com/dal-go/dalgo/dal"
)

// Field defines field
type Field interface {
	// Name returns the field name.
	Name() string
	// Type returns the field type.
	Type() string
	// IsRequired reports whether the field is required.
	IsRequired() bool
	// CompareTo returns a condition comparing this field with v using operator.
	CompareTo(operator dal.Operator, v dal.Expression) dal.Condition
}
