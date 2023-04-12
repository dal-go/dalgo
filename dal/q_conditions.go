package dal

import (
	"fmt"
	"strings"
)

type Condition interface {
	fmt.Stringer
}

// Comparison defines a contact for a comparison
type Comparison struct {
	Operator    Operator     `json:"operator"`
	Expressions []Expression `json:"expressions"`
}

// IsGroupOperator says if an operator is a group operator
func IsGroupOperator(o Operator) bool {
	return o == In
}

// String returns string representation of a comparison
func (v Comparison) String() string {
	if IsGroupOperator(v.Operator) {
		s := make([]string, len(v.Expressions))
		for i, e := range v.Expressions {
			s[i] = e.String()
		}
		return fmt.Sprintf("%v (%v)", v.Operator, strings.Join(s, ", "))
	}
	o := v.Operator
	if o == Equal {
		o = "="
	}
	return fmt.Sprintf("%v %v %v", v.Expressions[0], o, v.Expressions[1])
}

// NewComparison creates new Comparison
func NewComparison(o Operator, expressions ...Expression) Comparison {
	return Comparison{Operator: o, Expressions: expressions}
}

// String creates a new constantCondition expression
func String(v string) Expression {
	return constantCondition{Value: v}
}

type equal struct {
	Comparison
}
