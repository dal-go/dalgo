package dal

import (
	"errors"
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
	// OrderBy reports that the provider runs an aggregate's own order (see
	// OrderedAggregateFunc) exactly as DALgo defines it: NULL lowest, numbers
	// as float64, text by code point, false before true, timestamps by instant,
	// and ties broken by the aggregate's argument ascending. It is read together
	// with First and Last: an ordered first is native only when both First and
	// OrderBy are set, an ordered last when both Last and OrderBy are set. It
	// does not describe the query's own ORDER BY, which QueryCapabilities.OrderBy
	// does.
	OrderBy bool
}

// QueryCapabilities is deliberately granular: providers can advertise only
// the operations they can preserve exactly. Missing capability information is
// treated conservatively and selects DALgo's local hash strategy.
type QueryCapabilities struct {
	GroupBy bool
	Having  bool
	OrderBy bool
	// GroupKeyOrder, together with OrderBy, reports that raw field ordering
	// keeps every DALgo-normalized typed grouping key in one contiguous run.
	GroupKeyOrder  bool
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
		return AggregationPlan{}, fmt.Errorf("FIRST/LAST require a provider-declared stable input order, or an order of their own (aggregate orderBy)")
	}
	if nativeAggregationSupported(q, capabilities) {
		return AggregationPlan{Strategy: AggregationNative, Reason: "provider supports every required aggregation stage"}, nil
	}
	if canStreamAggregation(q) && capabilities.OrderBy && capabilities.GroupKeyOrder {
		return AggregationPlan{Strategy: AggregationStreaming, Reason: "provider lacks full aggregation but guarantees DALgo-normalized grouping-key runs"}, nil
	}
	return AggregationPlan{Strategy: AggregationHash, Reason: "provider lacks full aggregation and compatible grouping-key order"}, nil
}

// canStreamAggregation remains conservative about expressions because the
// GroupKeyOrder guarantee applies only to field group keys. Generic OrderBy is
// insufficient: its comparator can treat values that DALgo groups separately
// as equal, allowing a group to be interleaved.
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
	if err := validateOrderedAggregatePlacement(q); err != nil {
		return err
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
	return validateOrderedAggregateSources(q)
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
	case IsNullCondition:
		if err := validateGroupedOperand(c.Operand(), groups, aliases); err != nil {
			return fmt.Errorf("%s operand: %w", label, err)
		}
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
		if err := validateAggregateOrder(name, aggregateOrder(e)); err != nil {
			return err
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

// validateAggregateOrder holds an aggregate's own order to its rules: only
// FIRST and LAST take one, and every key is a field. The refusals name the
// position of a key, never its text, so they echo nothing from the query.
func validateAggregateOrder(name string, order []OrderExpression) error {
	if len(order) == 0 {
		return nil
	}
	if name != FIRST && name != LAST {
		return fmt.Errorf("aggregate orderBy is supported only for FIRST and LAST")
	}
	for i, key := range order {
		if key == nil {
			return fmt.Errorf("an aggregate order key must be a field (key #%d)", i+1)
		}
		if _, ok := key.Expression().(FieldRef); !ok {
			return fmt.Errorf("an aggregate order key must be a field (key #%d)", i+1)
		}
	}
	return nil
}

// aggregateOrder returns the order an aggregate reads its group in; none when
// the aggregate has no order of its own.
func aggregateOrder(aggregate AggregateFunc) []OrderExpression {
	if ordered, ok := aggregate.(OrderedAggregateFunc); ok {
		return ordered.AggregateOrder()
	}
	return nil
}

// holdsOrderedAggregate reports whether q holds an aggregate with an order of
// its own, in any place an aggregate may stand.
func holdsOrderedAggregate(q StructuredQuery) bool {
	found := false
	walkAggregates(q, func(a AggregateFunc) {
		if len(aggregateOrder(a)) > 0 {
			found = true
		}
	})
	return found
}

// orderedAggregateScope lists the sources of a query by kind: the aliases of
// stored sources, and the aliases of derived ones in the order they appear.
type orderedAggregateScope struct {
	stored  map[string]bool
	derived map[string]bool
	order   []string
}

func newOrderedAggregateScope(from FromSource) orderedAggregateScope {
	scope := orderedAggregateScope{stored: map[string]bool{}, derived: map[string]bool{}}
	var visit func(FromSource)
	visit = func(node FromSource) {
		if node == nil || node.Base() == nil {
			return
		}
		alias := joinAlias(node.Base())
		if _, derived := asQuerySource(node.Base()); derived {
			scope.derived[alias] = true
			scope.order = append(scope.order, alias)
		} else {
			scope.stored[alias] = true
		}
		for _, join := range node.Joins() {
			visit(joinedFrom(join))
		}
	}
	visit(from)
	return scope
}

// check refuses a field that is not a field of a stored source of the query: a
// derived source hands on values and not their types, and a source the query
// does not hold is not one of its sources.
func (scope orderedAggregateScope) check(field FieldRef) error {
	const refusal = "an ordered aggregate reads fields of stored sources only"
	switch source := field.Source(); {
	case source == "" && len(scope.order) > 0:
		return fmt.Errorf("%s: field %q has no source and source %q is a query", refusal, field.Name(), scope.order[0])
	case source == "":
		return nil
	case scope.derived[source]:
		return fmt.Errorf("%s: source %q is a query", refusal, source)
	case !scope.stored[source]:
		return fmt.Errorf("%s: unknown source %q", refusal, source)
	}
	return nil
}

// validateOrderedAggregateSources holds every field an ordered aggregate reads,
// in its argument or in its order, to the stored sources of the query. Each
// aggregate has been through validateAggregateExpression by now, so its keys
// are fields and its argument is a field, a constant, a param or arithmetic.
func validateOrderedAggregateSources(q StructuredQuery) error {
	var scope *orderedAggregateScope
	var refusal error
	walkAggregates(q, func(a AggregateFunc) {
		order := aggregateOrder(a)
		if refusal != nil || len(order) == 0 {
			return
		}
		if scope == nil {
			built := newOrderedAggregateScope(q.From())
			scope = &built
		}
		var fields []FieldRef
		for _, arg := range a.FuncArgs() {
			fields = appendFieldRefs(fields, arg)
		}
		for _, key := range order {
			fields = appendFieldRefs(fields, key.Expression())
		}
		for _, field := range fields {
			if refusal = scope.check(field); refusal != nil {
				return
			}
		}
	})
	return refusal
}

// appendFieldRefs appends the fields an argument reads: a field itself, or the
// fields of the arithmetic over fields.
func appendFieldRefs(fields []FieldRef, expression Expression) []FieldRef {
	switch e := expression.(type) {
	case FieldRef:
		fields = append(fields, e)
	case BinaryExpression:
		fields = appendFieldRefs(fields, e.Left)
		fields = appendFieldRefs(fields, e.Right)
	}
	return fields
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

// needsStableOrder reports whether q holds a first or last that depends on the
// order rows arrive in, which is one with no order of its own.
func needsStableOrder(q StructuredQuery) bool {
	found := false
	walkAggregates(q, func(a AggregateFunc) {
		if (strings.EqualFold(a.FuncName(), FIRST) || strings.EqualFold(a.FuncName(), LAST)) && len(aggregateOrder(a)) == 0 {
			found = true
		}
	})
	return found
}

// orderedAggregateNative reports whether a provider runs an aggregate's own
// order: an ordered first needs First and OrderBy, an ordered last Last and
// OrderBy.
func orderedAggregateNative(aggregate AggregateFunc, c AggregateCapabilities) bool {
	switch strings.ToUpper(aggregate.FuncName()) {
	case FIRST:
		return c.First && c.OrderBy
	case LAST:
		return c.Last && c.OrderBy
	}
	return false
}

// providerRunsOrderedAggregates reports whether a provider runs every ordered
// aggregate of q natively. It is true for a query that holds none.
func providerRunsOrderedAggregates(q StructuredQuery, c QueryCapabilities) bool {
	runs := true
	walkAggregates(q, func(a AggregateFunc) {
		if len(aggregateOrder(a)) > 0 {
			runs = runs && orderedAggregateNative(a, c.Aggregate)
		}
	})
	return runs
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
			ok = ok && c.Aggregate.First && (len(aggregateOrder(a)) == 0 || c.Aggregate.OrderBy)
		case LAST:
			ok = ok && c.Aggregate.Last && (len(aggregateOrder(a)) == 0 || c.Aggregate.OrderBy)
		default:
			ok = false
		}
	})
	return ok
}

func walkAggregates(q StructuredQuery, visit func(AggregateFunc)) {
	for _, c := range q.Columns() {
		walkExpressionAggregates(c.Expression, visit)
	}
	for _, e := range q.GroupBy() {
		walkExpressionAggregates(e, visit)
	}
	for _, o := range q.OrderBy() {
		walkExpressionAggregates(o.Expression(), visit)
	}
	walkConditionAggregates(q.Having(), visit)
}

// walkExpressionAggregates visits every aggregate in an expression, outermost first.
func walkExpressionAggregates(expression Expression, visit func(AggregateFunc)) {
	switch e := expression.(type) {
	case AggregateFunc:
		visit(e)
		for _, arg := range e.FuncArgs() {
			walkExpressionAggregates(arg, visit)
		}
		for _, key := range aggregateOrder(e) {
			if key != nil {
				walkExpressionAggregates(key.Expression(), visit)
			}
		}
	case BinaryExpression:
		walkExpressionAggregates(e.Left, visit)
		walkExpressionAggregates(e.Right, visit)
	}
}

// walkConditionAggregates visits every aggregate in the operands of a condition.
func walkConditionAggregates(condition Condition, visit func(AggregateFunc)) {
	switch c := condition.(type) {
	case IsNullCondition:
		walkExpressionAggregates(c.Operand(), visit)
	case Comparison:
		walkExpressionAggregates(c.Left, visit)
		walkExpressionAggregates(c.Right, visit)
	case GroupCondition:
		for _, child := range c.Conditions() {
			walkConditionAggregates(child, visit)
		}
	}
}

// validateOrderedAggregatePlacement refuses an aggregate with an order of its own
// in WHERE. An ordered aggregate stands in the select list, in HAVING and in ORDER
// BY, as an aggregate does; WHERE reads rows before they are grouped. A query
// whose WHERE holds an aggregate with no order is not read here.
func validateOrderedAggregatePlacement(q StructuredQuery) error {
	found := false
	walkConditionAggregates(q.Where(), func(a AggregateFunc) {
		if len(aggregateOrder(a)) > 0 {
			found = true
		}
	})
	if found {
		return errors.New("an aggregate with an order cannot stand in where")
	}
	return nil
}

// validateOrderedAggregatesForProvider holds a query that is about to be handed
// to a provider unplanned to the rules of the ordered aggregate: where it stands,
// and, when it holds one, every rule ValidateAggregation applies.
func validateOrderedAggregatesForProvider(q StructuredQuery) error {
	if err := validateOrderedAggregatePlacement(q); err != nil {
		return err
	}
	if holdsOrderedAggregate(q) {
		return ValidateAggregation(q)
	}
	return nil
}
