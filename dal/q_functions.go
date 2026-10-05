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
	FIRST   = "FIRST"
	LAST    = "LAST"
)

type function struct {
	Name     string       `json:"name"`
	Args     []Expression `json:"args"`
	Distinct bool         `json:"distinct,omitempty"`
	// order is the aggregate's own order; it is empty for every aggregate that
	// NewAggregate builds.
	order []OrderExpression
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

// DistinctAggregateFunc is the optional extension implemented by aggregate
// expressions that expose DISTINCT semantics. AggregateFunc intentionally
// remains unchanged so existing third-party expression implementations remain
// source compatible.
type DistinctAggregateFunc interface {
	AggregateFunc
	IsDistinct() bool
}

// OrderedAggregateFunc is the optional extension of an aggregate that reads the
// rows of its group in a stated order. An empty order means the aggregate has
// none. AggregateFunc intentionally remains unchanged so existing third-party
// expression implementations remain source compatible.
//
// The String of an implementation must write the order into its text, as the
// aggregate NewOrderedAggregate builds does: DALgo's in-memory engine keeps the
// state of an aggregate under its String, so two aggregates whose texts are equal
// share one state and one answer.
type OrderedAggregateFunc interface {
	AggregateFunc
	AggregateOrder() []OrderExpression
}

// StarExpression identifies the COUNT(*) row argument without requiring
// adapters to depend on DALgo's private concrete expression type.
type StarExpression interface {
	Expression
	IsStar() bool
}

var _ AggregateFunc = function{}

// FuncName returns the aggregate function name (e.g. SUM, COUNT).
func (v function) FuncName() string { return v.Name }

// FuncArgs returns the aggregate function arguments.
func (v function) FuncArgs() []Expression { return v.Args }

func (v function) IsDistinct() bool { return v.Distinct }

// AggregateOrder returns the order in which the aggregate reads the rows of its
// group; it is empty when the aggregate has none.
func (v function) AggregateOrder() []OrderExpression { return v.order }

// String returns a text representation of a function. The text of an
// aggregate with an order ends in ORDER BY and the order, for example
// LAST(Total ORDER BY InvoiceDate, InvoiceId DESC). It is also the key under
// which the in-memory engine keeps the aggregate's state.
func (v function) String() string {
	args := make([]string, len(v.Args))
	for i, arg := range v.Args {
		args[i] = arg.String()
	}
	prefix := ""
	if v.Distinct {
		prefix = "DISTINCT "
	}
	suffix := ""
	if len(v.order) > 0 {
		keys := make([]string, len(v.order))
		for i, key := range v.order {
			keys[i] = orderKeyText(key)
		}
		suffix = " ORDER BY " + strings.Join(keys, ", ")
	}
	return fmt.Sprintf("%v(%v%v%v)", v.Name, prefix, strings.Join(args, ", "), suffix)
}

// missingOrderKeyText stands in the text of an aggregate for an order key that is
// missing or has no expression. Such a key is refused by ValidateAggregation, and
// an aggregate is named by its text before that.
const missingOrderKeyText = "<missing key>"

// orderKeyText is the text of one key of an aggregate's order.
func orderKeyText(key OrderExpression) string {
	if key == nil || key.Expression() == nil {
		return missingOrderKeyText
	}
	return key.String()
}

// star is the `*` argument of COUNT(*); it is not a field reference, which is
// how the executor distinguishes COUNT(*) (count all rows) from COUNT(field).
type star struct{}

var _ Expression = star{}

// String renders the star argument as `*`.
func (star) String() string { return "*" }

func (star) IsStar() bool { return true }

// NewAggregate creates an aggregate expression for parsers and planners.
// Validation of function names, arity and DISTINCT support is performed by
// ValidateAggregation.
func NewAggregate(name string, distinct bool, args ...Expression) AggregateFunc {
	return function{Name: strings.ToUpper(name), Args: append([]Expression(nil), args...), Distinct: distinct}
}

// NewOrderedAggregate is NewAggregate for an aggregate with an order of its
// own: the rows of each group are read in that order. Only FIRST and LAST take
// one; ValidateAggregation refuses it on any other function.
func NewOrderedAggregate(name string, order []OrderExpression, args ...Expression) AggregateFunc {
	return function{
		Name:  strings.ToUpper(name),
		Args:  append([]Expression(nil), args...),
		order: append([]OrderExpression(nil), order...),
	}
}

// Star returns the special row expression used by COUNT(*).
func Star() Expression { return star{} }

// Count returns a COUNT(*) aggregate column counting all rows in a group
// regardless of nulls. Count() is the alias for COUNT(*); use the returned
// Column's Alias field to name the output. The existing CountAs(field, alias)
// keeps its field-count (skip-nulls) semantics.
func Count() Column {
	return Column{Expression: NewAggregate(COUNT, false, Star())}
}

func singleArgFunctionAs(name, alias string, expression Expression) Column {
	return Column{
		Expression: NewAggregate(name, false, expression),
		Alias:      alias,
	}
}

func distinctSingleArgFunctionAs(name, alias string, expression Expression) Column {
	return Column{Expression: NewAggregate(name, true, expression), Alias: alias}
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

// CountDistinctAs counts distinct non-null expression values.
func CountDistinctAs(expression Expression, alias string) Column {
	return distinctSingleArgFunctionAs(COUNT, alias, expression)
}

// SumDistinctAs sums distinct non-null numeric expression values.
func SumDistinctAs(expression Expression, alias string) Column {
	return distinctSingleArgFunctionAs(SUM, alias, expression)
}

// AverageDistinctAs averages distinct non-null numeric expression values.
func AverageDistinctAs(expression Expression, alias string) Column {
	return distinctSingleArgFunctionAs(AVERAGE, alias, expression)
}

// FirstAs returns the first value, including NULL, in a provider-declared
// stable input order. Execution rejects FIRST when that guarantee is absent.
func FirstAs(expression Expression, alias string) Column {
	return singleArgFunctionAs(FIRST, alias, expression)
}

// LastAs returns the last value, including NULL, in a provider-declared stable
// input order. Execution rejects LAST when that guarantee is absent.
func LastAs(expression Expression, alias string) Column {
	return singleArgFunctionAs(LAST, alias, expression)
}
