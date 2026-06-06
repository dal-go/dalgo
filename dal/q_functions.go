package dal

import (
	"fmt"
	"strings"
)

const (
	SUM     = "SUM"
	COUNT   = "COUNT"
	MIN     = "MIN"
	MAX     = "MAX"
	AVERAGE = "AVG"
)

type function struct {
	Name string       `json:"name"`
	Args []Expression `json:"args"`
}

var _ Expression = (*function)(nil)

// AggregateFunc is implemented by aggregate function expressions (SUM, COUNT,
// MIN, MAX, AVG) so adapters can introspect the function name and its arguments
// without depending on the unexported concrete type.
type AggregateFunc interface {
	Expression
	FuncName() string
	FuncArgs() []Expression
}

var _ AggregateFunc = function{}

// FuncName returns the aggregate function name (e.g. SUM, COUNT).
func (v function) FuncName() string { return v.Name }

// FuncArgs returns the aggregate function arguments.
func (v function) FuncArgs() []Expression { return v.Args }

// String returns a text representation of a function
func (v function) String() string {
	args := make([]string, len(v.Args))
	for i, arg := range v.Args {
		args[i] = arg.String()
	}
	return fmt.Sprintf("%v(%v)", v.Name, strings.Join(args, ", "))
}

// star is the `*` argument of COUNT(*); it is not a field reference, which is
// how the executor distinguishes COUNT(*) (count all rows) from COUNT(field).
type star struct{}

var _ Expression = star{}

// String renders the star argument as `*`.
func (star) String() string { return "*" }

// Count returns a COUNT(*) aggregate column counting all rows in a group
// regardless of nulls. Count() is the alias for COUNT(*); use the returned
// Column's Alias field to name the output. The existing CountAs(field, alias)
// keeps its field-count (skip-nulls) semantics.
func Count() Column {
	return Column{Expression: function{Name: COUNT, Args: []Expression{star{}}}}
}

func singleArgFunctionAs(name, alias string, expression Expression) Column {
	return Column{
		Expression: function{
			Name: name,
			Args: []Expression{expression},
		},
		Alias: alias,
	}
}

// SumAs aggregate function (see SQL SUM())
func SumAs(expression Expression, alias string) Column {
	return singleArgFunctionAs(SUM, alias, expression)
}

// CountAs aggregate function (see SQL COUNT())
func CountAs(expression Expression, alias string) Column {
	return singleArgFunctionAs(COUNT, alias, expression)
}

// MinAs returns minimum value for a given expression
func MinAs(expression Expression, alias string) Column {
	return singleArgFunctionAs(MIN, alias, expression)
}

// MaxAs returns maximum value for a given expression
func MaxAs(expression Expression, alias string) Column {
	return singleArgFunctionAs(MAX, alias, expression)
}

// AverageAs returns average value for a given expression
func AverageAs(expression Expression, alias string) Column {
	return singleArgFunctionAs(AVERAGE, alias, expression)
}
