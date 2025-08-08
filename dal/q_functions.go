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

// String returns a text representation of a function
func (v function) String() string {
	args := make([]string, len(v.Args))
	for i, arg := range v.Args {
		args[i] = arg.String()
	}
	return fmt.Sprintf("%v(%v)", v.Name, strings.Join(args, ", "))
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
