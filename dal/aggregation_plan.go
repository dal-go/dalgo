package dal

import (
	"fmt"
	"strings"
)

// AggregateCapabilities describes which aggregate forms a provider can execute
// without DALgo's local fallback.
type AggregateCapabilities struct {
	Count, CountDistinct bool
	Sum, SumDistinct     bool
	Avg, AvgDistinct     bool
	Min, Max             bool
	First, Last          bool
	OrderBy              bool
}

// QueryCapabilities is deliberately granular: providers can advertise only
// the operations they can preserve exactly. Missing capability information is
// treated conservatively and selects DALgo's local hash strategy.
type QueryCapabilities struct {
	GroupBy        bool
	Having         bool
	OrderBy        bool
	StableRowOrder bool
	Aggregate      AggregateCapabilities
}

// QueryCapabilitiesProvider is an optional adapter capability.
type QueryCapabilitiesProvider interface {
	QueryCapabilities() QueryCapabilities
}

// AggregationStrategy identifies the physical aggregation implementation.
type AggregationStrategy string

const (
	AggregationNative    AggregationStrategy = "native"
	AggregationStreaming AggregationStrategy = "dalgo-streaming"
	AggregationHash      AggregationStrategy = "dalgo-hash"
)

// AggregationPlan is inspectable so DataTug can expose the decision later.
type AggregationPlan struct {
	Strategy AggregationStrategy
	Reason   string
}

// PlanAggregation validates q and chooses the safest available strategy.
func PlanAggregation(q StructuredQuery, capabilities QueryCapabilities) (AggregationPlan, error) {
	if err := ValidateAggregation(q); err != nil {
		return AggregationPlan{}, err
	}
	if !HasAggregation(q) {
		return AggregationPlan{Strategy: AggregationNative, Reason: "query has no aggregation"}, nil
	}
	if needsStableOrder(q) && !capabilities.StableRowOrder {
		return AggregationPlan{}, fmt.Errorf("FIRST/LAST require a provider-declared stable input order; aggregate ORDER BY is not yet supported")
	}
	if nativeAggregationSupported(q, capabilities) {
		return AggregationPlan{Strategy: AggregationNative, Reason: "provider supports every required aggregation stage"}, nil
	}
	if canStreamAggregation(q) && capabilities.OrderBy {
		return AggregationPlan{Strategy: AggregationStreaming, Reason: "provider lacks full aggregation but can order raw rows by grouping keys"}, nil
	}
	return AggregationPlan{Strategy: AggregationHash, Reason: "provider lacks full aggregation and useful grouping order"}, nil
}

// canStreamAggregation is deliberately conservative. The generic ORDER BY
// capability guarantees field ordering; providers may not evaluate arbitrary
// scalar expressions in their raw-row ordering path. Expression group keys
// therefore use hash aggregation unless the whole query is native.
func canStreamAggregation(q StructuredQuery) bool {
	if len(q.GroupBy()) == 0 {
		return false
	}
	for _, expression := range q.GroupBy() {
		if _, ok := expression.(FieldRef); !ok {
			return false
		}
	}
	return true
}

// HasAggregation reports whether a query enters aggregate semantics, including
// an implicit single group with no GROUP BY.
func HasAggregation(q StructuredQuery) bool {
	if q == nil {
		return false
	}
	if len(q.GroupBy()) > 0 || q.Having() != nil {
		return true
	}
	for _, c := range q.Columns() {
		if expressionContainsAggregate(c.Expression) {
			return true
		}
	}
	for _, o := range q.OrderBy() {
		if expressionContainsAggregate(o.Expression()) {
			return true
		}
	}
	return false
}

// ValidateAggregation applies SQL-compatible grouping rules without imposing
// SQL syntax or alias restrictions on DTQL.
func ValidateAggregation(q StructuredQuery) error {
	if q == nil || !HasAggregation(q) {
		return nil
	}
	groupKeys := map[string]bool{}
	for i, expression := range q.GroupBy() {
		if expression == nil {
			return fmt.Errorf("GROUP BY expression #%d is nil", i)
		}
		if expressionContainsAggregate(expression) {
			return fmt.Errorf("GROUP BY expression #%d contains an aggregate", i)
		}
		if err := validateScalarExpression(expression); err != nil {
			return fmt.Errorf("GROUP BY expression #%d: %w", i, err)
		}
		groupKeys[expression.String()] = true
	}
	aliases := map[string]Expression{}
	for i, column := range effectiveAggregationColumns(q) {
		if column.Wildcard != nil {
			return fmt.Errorf("aggregate SELECT column #%d cannot be a wildcard", i)
		}
		if column.Expression == nil {
			return fmt.Errorf("aggregate SELECT column #%d has no expression", i)
		}
		if expressionContainsAggregate(column.Expression) {
			if err := validateGroupedExpression(column.Expression, groupKeys, nil); err != nil {
				return fmt.Errorf("aggregate SELECT column #%d: %w", i, err)
			}
		} else if !groupKeys[column.Expression.String()] {
			return fmt.Errorf("selected expression %q is neither aggregated nor present in GROUP BY", column.Expression.String())
		}
		if column.Alias != "" {
			if _, exists := aliases[column.Alias]; exists {
				return fmt.Errorf("duplicate SELECT alias %q", column.Alias)
			}
			aliases[column.Alias] = column.Expression
		}
	}
	if err := validateAggregateCondition(q.Having(), groupKeys, aliases, "HAVING"); err != nil {
		return err
	}
	for i, order := range q.OrderBy() {
		if err := validateGroupedOperand(order.Expression(), groupKeys, aliases); err != nil {
			return fmt.Errorf("ORDER BY expression #%d: %w", i, err)
		}
	}
	return nil
}

// EffectiveAggregationColumns supplies GROUP BY expressions as the implicit
// projection when a grouped DTQL query omits columns.
func EffectiveAggregationColumns(q StructuredQuery) []Column {
	return effectiveAggregationColumns(q)
}

func effectiveAggregationColumns(q StructuredQuery) []Column {
	if columns := q.Columns(); len(columns) > 0 {
		return columns
	}
	if groupBy := q.GroupBy(); len(groupBy) > 0 {
		columns := make([]Column, len(groupBy))
		for i, expression := range groupBy {
			columns[i] = Column{Expression: expression}
		}
		return columns
	}
	return nil
}

func validateAggregateCondition(condition Condition, groups map[string]bool, aliases map[string]Expression, label string) error {
	if condition == nil {
		return nil
	}
	switch c := condition.(type) {
	case Comparison:
		if err := validateGroupedOperand(c.Left, groups, aliases); err != nil {
			return fmt.Errorf("%s left operand: %w", label, err)
		}
		if err := validateGroupedOperand(c.Right, groups, aliases); err != nil {
			return fmt.Errorf("%s right operand: %w", label, err)
		}
	case GroupCondition:
		for i, child := range c.Conditions() {
			if err := validateAggregateCondition(child, groups, aliases, label); err != nil {
				return fmt.Errorf("%s condition #%d: %w", label, i, err)
			}
		}
	default:
		return fmt.Errorf("%s uses unsupported condition %T", label, condition)
	}
	return nil
}

func validateGroupedOperand(expression Expression, groups map[string]bool, aliases map[string]Expression) error {
	return validateGroupedExpression(expression, groups, aliases)
}

func validateGroupedExpression(expression Expression, groups map[string]bool, aliases map[string]Expression) error {
	if expression == nil {
		return fmt.Errorf("expression is nil")
	}
	if field, ok := expression.(FieldRef); ok && field.Source() == "" {
		if _, alias := aliases[field.Name()]; alias {
			return nil
		}
	}
	switch e := expression.(type) {
	case AggregateFunc:
		return validateAggregateExpression(e)
	case BinaryExpression:
		if err := validateGroupedExpression(e.Left, groups, aliases); err != nil {
			return err
		}
		return validateGroupedExpression(e.Right, groups, aliases)
	case Constant:
		return nil
	default:
		if groups[expression.String()] {
			return nil
		}
	}
	return fmt.Errorf("%q is neither an aggregate, SELECT alias, nor GROUP BY expression", expression.String())
}

func validateAggregateExpression(expression Expression) error {
	switch e := expression.(type) {
	case AggregateFunc:
		name := strings.ToUpper(e.FuncName())
		args := e.FuncArgs()
		if len(args) != 1 {
			return fmt.Errorf("%s requires exactly one argument", name)
		}
		_, star := args[0].(StarExpression)
		distinct := aggregateDistinct(e)
		switch name {
		case COUNT:
			if star && distinct {
				return fmt.Errorf("COUNT(DISTINCT *) is not supported")
			}
		case SUM, AVERAGE:
			if star {
				return fmt.Errorf("%s(*) is not supported", name)
			}
		case MIN, MAX, FIRST, LAST:
			if distinct {
				return fmt.Errorf("DISTINCT is not supported for %s", name)
			}
			if star {
				return fmt.Errorf("%s(*) is not supported", name)
			}
		default:
			return fmt.Errorf("unsupported aggregate %q", e.FuncName())
		}
		if !star {
			if expressionContainsAggregate(args[0]) {
				return fmt.Errorf("nested aggregates are not supported")
			}
			return validateScalarExpression(args[0])
		}
		return nil
	case BinaryExpression:
		if err := validateAggregateExpression(e.Left); err != nil {
			return err
		}
		return validateAggregateExpression(e.Right)
	default:
		return validateScalarExpression(expression)
	}
}

func validateScalarExpression(expression Expression) error {
	switch e := expression.(type) {
	case FieldRef, Constant, Param:
		return nil
	case BinaryExpression:
		switch e.Operator {
		case Add, Subtract, Multiply, Divide:
		default:
			return fmt.Errorf("unsupported arithmetic operator %q", e.Operator)
		}
		if err := validateScalarExpression(e.Left); err != nil {
			return err
		}
		return validateScalarExpression(e.Right)
	default:
		return fmt.Errorf("unsupported scalar expression %T", expression)
	}
}

func expressionContainsAggregate(expression Expression) bool {
	switch e := expression.(type) {
	case AggregateFunc:
		return true
	case BinaryExpression:
		return expressionContainsAggregate(e.Left) || expressionContainsAggregate(e.Right)
	default:
		return false
	}
}

func aggregateDistinct(aggregate AggregateFunc) bool {
	distinct, ok := aggregate.(DistinctAggregateFunc)
	return ok && distinct.IsDistinct()
}

func needsStableOrder(q StructuredQuery) bool {
	found := false
	walkAggregates(q, func(a AggregateFunc) {
		if strings.EqualFold(a.FuncName(), FIRST) || strings.EqualFold(a.FuncName(), LAST) {
			found = true
		}
	})
	return found
}

func nativeAggregationSupported(q StructuredQuery, c QueryCapabilities) bool {
	if len(q.GroupBy()) > 0 && !c.GroupBy || q.Having() != nil && !c.Having || len(q.OrderBy()) > 0 && !c.OrderBy {
		return false
	}
	ok := true
	walkAggregates(q, func(a AggregateFunc) {
		distinct := aggregateDistinct(a)
		switch strings.ToUpper(a.FuncName()) {
		case COUNT:
			ok = ok && ((!distinct && c.Aggregate.Count) || (distinct && c.Aggregate.CountDistinct))
		case SUM:
			ok = ok && ((!distinct && c.Aggregate.Sum) || (distinct && c.Aggregate.SumDistinct))
		case AVERAGE:
			ok = ok && ((!distinct && c.Aggregate.Avg) || (distinct && c.Aggregate.AvgDistinct))
		case MIN:
			ok = ok && c.Aggregate.Min
		case MAX:
			ok = ok && c.Aggregate.Max
		case FIRST:
			ok = ok && c.Aggregate.First
		case LAST:
			ok = ok && c.Aggregate.Last
		default:
			ok = false
		}
	})
	return ok
}

func walkAggregates(q StructuredQuery, visit func(AggregateFunc)) {
	var walkExpr func(Expression)
	walkExpr = func(expression Expression) {
		switch e := expression.(type) {
		case AggregateFunc:
			visit(e)
			for _, arg := range e.FuncArgs() {
				walkExpr(arg)
			}
		case BinaryExpression:
			walkExpr(e.Left)
			walkExpr(e.Right)
		}
	}
	var walkCondition func(Condition)
	walkCondition = func(condition Condition) {
		switch c := condition.(type) {
		case Comparison:
			walkExpr(c.Left)
			walkExpr(c.Right)
		case GroupCondition:
			for _, child := range c.Conditions() {
				walkCondition(child)
			}
		}
	}
	for _, c := range q.Columns() {
		walkExpr(c.Expression)
	}
	for _, e := range q.GroupBy() {
		walkExpr(e)
	}
	for _, o := range q.OrderBy() {
		walkExpr(o.Expression())
	}
	walkCondition(q.Having())
}
