package dal

import (
	"fmt"
	"reflect"
)

type Condition interface {
	fmt.Stringer
}

// Comparison defines a contact for a comparison
type Comparison struct {
	Operator Operator
	Left     Expression
	Right    Expression
}

func (v Comparison) Equal(b Comparison) bool {
	return v.Operator == b.Operator && reflect.DeepEqual(v.Left, b.Left) && reflect.DeepEqual(v.Right, b.Right)
}

// IsGroupOperator says if an operator is a group operator
func IsGroupOperator(o Operator) bool {
	return o == In
}

// String returns string representation of a comparison
func (v Comparison) String() string {
	o := v.Operator
	if o == Equal {
		o = "="
	}
	return fmt.Sprintf("%v %v %v", v.Left, o, v.Right)
}

// NewComparison creates new Comparison
func NewComparison(left Expression, o Operator, right Expression) Comparison {
	return Comparison{Operator: o, Left: left, Right: right}
}

// String creates a new constantExpression expression
func String(v string) Expression {
	return constantExpression{Value: v}
}
