package query

import (
	"encoding/json"
	"fmt"
)

const (
	EqualOperator = "=="
	AndOperator   = "AND"
	OrOperator    = "OR"
)

// Column reference a column in a SELECT statement
type Column struct {
	Expression Expression
	Alias      string
}

type field struct {
	Name string
}

// Columns shortcut for creating slice of columns by names
func Columns(names ...string) []Column {
	cols := make([]Column, len(names))
	for i, name := range names {
		cols[i] = Column{Expression: field{Name: name}}
	}
	return cols
}

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
	return equal{comparison: comparison{operator: EqualOperator, expression1: f, expression2: val}}
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
	Operator() string
}

type comparison struct {
	operator    string
	expression1 Expression
	expression2 Expression
}

func (v comparison) Operator() string {
	return v.operator
}

func (v comparison) String() string {
	return fmt.Sprintf("%v %v %v",
		v.expression1, v.operator, v.expression2)
}

type equal struct {
	comparison
}
