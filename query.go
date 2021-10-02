package dalgo

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	EqualOperator = "=="
	AndOperator   = "AND"
	OrOperator    = "OR"
)

// QueryField reference a field in a SELECT statement
type QueryField struct {
	Expression Expression
	Alias      string
}

// CollectionRef points to a collection (e.g. table) in a database
type CollectionRef struct {
	Name      string
	Parent    *Key
	NewRecord RecordConstructor
}

// Query holds definition of a query
type Query struct {
	Collection    CollectionRef
	Condition     Condition
	ExecuteReader func() (Reader, error)
}

// And creates a new query by adding a condition to a predefined query
func (q Query) groupWithConditions(operator string, conditions ...Condition) Query {
	query := Query{Collection: q.Collection, ExecuteReader: q.ExecuteReader}
	and := groupCondition{operator: operator, Conditions: make([]Condition, len(conditions)+1)}
	and.Conditions[0] = q.Condition
	for i, condition := range conditions {
		and.Conditions[i+1] = condition
	}
	query.Condition = and
	return query
}

// And creates an inherited query by adding AND conditions
func (q Query) And(conditions ...Condition) Query {
	return q.groupWithConditions(AndOperator, conditions...)
}

// Or creates an inherited query by adding OR conditions
func (q Query) Or(conditions ...Condition) Query {
	return q.groupWithConditions(OrOperator, conditions...)
}

type groupCondition struct {
	operator   string
	Conditions []Condition
}

func (v groupCondition) Operator() string {
	return v.operator
}

func (v groupCondition) String() string {
	s := make([]string, len(v.Conditions))
	for i, condition := range v.Conditions {
		s[i] = condition.String()
	}
	return fmt.Sprintf("(%v)", strings.Join(s, v.operator))
}

type field struct {
	Name string
}

func Field(name string) field {
	if name == "" {
		panic("can not reference field with empty name")
	}
	return field{Name: name}
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
