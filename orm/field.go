package orm

import (
	"github.com/dal-go/dalgo/dal"
)

// Field defines field
type Field interface {
	Name() string
	Type() string
	IsRequired() bool
	CompareTo(operator dal.Operator, v dal.Expression) dal.Condition
}
