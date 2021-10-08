package query

import (
	"encoding/json"
	"fmt"
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
	Expression Expression
	Alias      string
}

func (v Column) String() string {
	if v.Alias == "" {
		return v.Expression.String()
	}
	return fmt.Sprintf("%v AS %v", v.Expression, v.Alias)
}

type field struct {
	Name string
}

// Field creates new field
func Field(name string) Expression {
	return field{Name: name}
}

// Columns shortcut for creating slice of columns by names
func Columns(names ...string) []Column {
	cols := make([]Column, len(names))
	for i, name := range names {
		cols[i] = Column{Expression: field{Name: name}}
	}
	return cols
}

// String returns string representation of a field
func (f field) String() string {
	return fmt.Sprintf("[%v]", f.Name)
}

func (f field) EqualTo(v interface{}) Condition {
	var val Expression
	switch v.(type) {
	case string, int:
		val = constant{value: v}
	case constant:
		val = v.(constant)
	case field:
		val = v.(field)
	}
	return equal{Comparison: Comparison{operator: Equal, expressions: []Expression{f, val}}}
}

type constant struct {
	value interface{}
}

// String returns string representation of a constant
func (v constant) String() string {
	s, _ := json.Marshal(v.value)
	return string(s)
}

// Expression represent either a field, constant or a formula
type Expression interface {
	fmt.Stringer
}

// Condition holds condition definition
type Condition interface {
	fmt.Stringer
	Operator() Operator
}

// Comparison defines a contact for a comparison
type Comparison struct {
	operator    Operator
	expressions []Expression
}

// Operator returns comparison operator
func (v Comparison) Operator() Operator {
	return v.operator
}

// IsGroupOperator says if an operator is a group operator
func IsGroupOperator(o Operator) bool {
	return o == In
}

// String returns string representation of a comparison
func (v Comparison) String() string {
	if IsGroupOperator(v.operator) {
		s := make([]string, len(v.expressions))
		for i, e := range v.expressions {
			s[i] = e.String()
		}
		fmt.Sprintf("%v (%v)", v.operator, strings.Join(s, ", "))
	}
	return fmt.Sprintf("%v %v %v", v.expressions[0], v.operator, v.expressions[1])
}

// NewComparison creates new Comparison
func NewComparison(o Operator, expressions ...Expression) Comparison {
	return Comparison{operator: o, expressions: expressions}
}

// String creates a new constant expression
func String(v string) Expression {
	return constant{value: v}
}

type equal struct {
	Comparison
}

type function struct {
	name string
	args []Expression
}

func (v function) String() string {
	args := make([]string, len(v.args))
	for i, arg := range v.args {
		args[i] = arg.String()
	}
	return fmt.Sprintf("%v(%v)", v.name, strings.Join(args, ", "))
}

func sum(fieldName string) function {
	return function{name: "SUM", args: []Expression{field{Name: fieldName}}}
}

// SumAs aggregate function (see SQL SUM())
func SumAs(fieldName, alias string) Column {
	return Column{Expression: sum(fieldName), Alias: alias}
}

func count(fieldName string) function {
	return function{name: "COUNT", args: []Expression{field{Name: fieldName}}}
}

// CountAs aggregate function (see SQL COUNT())
func CountAs(fieldName, alias string) Column {
	return Column{Expression: count(fieldName), Alias: alias}
}
