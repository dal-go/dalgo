package dal

import (
	"github.com/dal-go/dalgo/constant"
)

// ID creates an expression that compares an ID with a constant
func ID(name string, value any) Expression {
	return Comparison{
		Operator: Equal,
		Left:     FieldRef{Name: name, IsID: true},
		Right:    constant.Value(value),
	}
}
