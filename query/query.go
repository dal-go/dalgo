package query

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Operator defines a Comparison operator
type Operator string

const (
	// Equal is a Comparison operator
	Equal Operator = "=="

	// And is a Comparison operator
	And = "AND"

	// Or is a Comparison operator
	Or = "OR"

	// In is a Comparison operator
	In = "In"
)

// Column reference a column in a SELECT statement
type Column struct {
	Alias      string     `json:"alias"`
	Expression Expression `json:"expression"`
}

// String stringifies column value
func (v Column) String() string {
	if v.Alias == "" {
		return v.Expression.String()
	}
	expr := v.Expression.String()
	if expr == v.Alias {
		return expr
	}
	return fmt.Sprintf("%v AS %v", expr, v.Alias)
}

type field struct {
	Name string `json:"name"`
	IsID bool   `json:"isID"`
}

// Field creates an expression that represents a field value
func Field(name string) Expression {
	return field{Name: name}
}

// ID creates an expression that compares an ID with a constant
func ID(name string, value any) Expression {
	return Comparison{Operator: Equal, Expressions: []Expression{
		field{Name: name, IsID: true},
		constant{Value: value},
	}}
}

// Columns shortcut for creating slice of columns by names
func Columns(names ...string) []Column {
	cols := make([]Column, len(names))
	for i, name := range names {
		cols[i] = Column{Expression: field{Name: name}, Alias: name}
	}
	return cols
}

// String returns string representation of a field
func (f field) String() string {
	return fmt.Sprintf("[%v]", f.Name)
}

// EqualTo creates equality condition for a field
func (f field) EqualTo(v any) Condition {
	var val Expression
	switch v := v.(type) {
	case string, int:
		val = constant{Value: v}
	case constant:
		val = v
	case field:
		val = v
	}
	return equal{Comparison: Comparison{Operator: Equal, Expressions: []Expression{f, val}}}
}

type constant struct {
	Value any `json:"value"`
}

// String returns string representation of a constant
func (v constant) String() string {
	switch v.Value.(type) {
	case int:
		return strconv.Itoa(v.Value.(int))
	case string:
		return fmt.Sprintf("'%v'", v.Value)
	default:
		s, _ := json.Marshal(v.Value)
		return string(s)
	}
}

// Expression represent either a field, constant or a formula
type Expression interface {
	fmt.Stringer
}

// Condition holds condition definition
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

// String creates a new constant expression
func String(v string) Expression {
	return constant{Value: v}
}

type equal struct {
	Comparison
}
