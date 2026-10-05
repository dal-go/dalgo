package access

import (
	"context"
	"fmt"
	"strings"

	"github.com/dal-go/dalgo/dal"
)

type requestedQueryCarrier interface {
	requestedStructuredQuery() dal.StructuredQuery
}

// securedStructuredQuery keeps the caller's original query alongside the
// effective query as it accumulates row filters and projections across nested
// secured sessions.
type securedStructuredQuery struct {
	dal.StructuredQuery
	requested dal.StructuredQuery
}

func (q securedStructuredQuery) requestedStructuredQuery() dal.StructuredQuery { return q.requested }
func (q securedStructuredQuery) String() string                                { return dal.QueryString(q) }
func (q securedStructuredQuery) GetRecordsReader(ctx context.Context, executor dal.QueryExecutor) (dal.RecordsReader, error) {
	return executor.ExecuteQueryToRecordsReader(ctx, q)
}
func (q securedStructuredQuery) GetRecordsetReader(ctx context.Context, executor dal.QueryExecutor) (dal.RecordsetReader, error) {
	return executor.ExecuteQueryToRecordsetReader(ctx, q)
}

func splitRequestedQuery(query dal.Query) (dal.Query, dal.StructuredQuery) {
	structured, ok := query.(dal.StructuredQuery)
	if !ok {
		return query, nil
	}
	if carried, ok := query.(requestedQueryCarrier); ok {
		return structured, carried.requestedStructuredQuery()
	}
	return structured, structured
}

func preserveRequestedQuery(query dal.Query, requested dal.StructuredQuery) dal.Query {
	structured, ok := query.(dal.StructuredQuery)
	if !ok || requested == nil {
		return query
	}
	return securedStructuredQuery{StructuredQuery: structured, requested: requested}
}

// fieldScope says how the fields of one clause are held to the allow-list.
type fieldScope struct {
	// usage names the clause in a denial.
	usage string
	// aggregates is true where an aggregate may stand: the select list, HAVING and
	// ORDER BY.
	aggregates bool
	// aliases is true where a bare name may be the alias of an allowed aggregate
	// of the same query: HAVING and ORDER BY.
	aliases bool
	// clauses, when set, attributes each field to a source: a field of a joined
	// source is not held to the list, which applies to the base source. Clauses
	// without it hold every field to the list whatever its qualifier.
	clauses *joinClauses
	// own is the source an unqualified field belongs to, when clauses is set.
	own fieldOwner
}

const (
	usageFilter   = "filter"
	usageGroup    = "group"
	usageOrder    = "order"
	usageSelected = "selected"
	usageJoin     = "join condition"
	usageScan     = "scan order"
)

// validateRequestedQueryFields holds the caller's query to the field allow-list.
// HAVING and ORDER BY may name a column only by a name the list allows.
func validateRequestedQueryFields(query dal.StructuredQuery, sets fieldSets) error {
	return validateQueryFields(query, sets, false)
}

// validateRequestedQueryFieldsWithAliases is validateRequestedQueryFields for a
// reader that sends an allowed aggregate under an alias of the access layer's own
// (see aliasRefusedOutputs) and names that alias in HAVING and ORDER BY: there,
// the alias of an allowed aggregate of the query is a name they may use.
func validateRequestedQueryFieldsWithAliases(query dal.StructuredQuery, sets fieldSets) error {
	return validateQueryFields(query, sets, true)
}

func validateQueryFields(query dal.StructuredQuery, sets fieldSets, aliasReferences bool) error {
	resource := resourcesForQuery(query)[0]
	unsupported := func(explanation string) error {
		return &DeniedError{Decision: Decision{Operation: Query, Resource: resource, Policy: "fields", Effect: effectDeny.String(), Code: CodeEnforcementUnsupported, Scope: DecisionScopeColumn, Slot: DecisionSlotFields, Explanation: explanation}}
	}
	deny := func(usage, field string) error {
		slot := DecisionSlotFields
		if usage == usageFilter || usage == usageJoin {
			slot = DecisionSlotWhere
		}
		return &DeniedError{Decision: Decision{Operation: Query, Resource: resource, Policy: "fields", Effect: effectDeny.String(), Code: CodeColumnDenied, Scope: DecisionScopeColumn, Slot: slot, Columns: [][]string{{field}}, Explanation: fmt.Sprintf("%s field %q is not allowed", usage, field)}}
	}
	var aliases map[string]bool
	if aliasReferences {
		aliases = allowedAggregateAliases(query.Columns(), sets)
	}
	// checkExpression holds an expression to the allow-list. An aggregate is
	// accepted only where aggregates belong (the select list, HAVING and ORDER
	// BY); anywhere else, and in any slot when it is not one DALgo defines, it
	// is refused as unsupported.
	checkExpression := func(expression dal.Expression, scope fieldScope) error {
		field, ok := expression.(dal.FieldRef)
		if !ok {
			if pointer, pointerOK := expression.(*dal.FieldRef); pointerOK && pointer != nil {
				field, ok = *pointer, true
			}
		}
		if !ok {
			// An aggregate reads only the fields of its operands, so it is held
			// to the allow-list operand by operand; the first refused field is
			// the one named.
			if aggregate, isAggregate := expression.(dal.AggregateFunc); isAggregate && scope.aggregates {
				if fields, checkable := aggregateFields(aggregate, 0); checkable {
					for _, name := range fields {
						if !sets.allowsWhole(name) {
							return deny(scope.usage, name)
						}
					}
					return nil
				}
			}
			return &DeniedError{Decision: Decision{Operation: Query, Resource: resource, Policy: "fields", Effect: effectDeny.String(), Code: CodeEnforcementUnsupported, Scope: DecisionScopeColumn, Slot: DecisionSlotFields, Explanation: fmt.Sprintf("%s expression cannot be safely checked against allowed fields", scope.usage)}}
		}
		name := field.Name()
		if scope.aliases && field.Source() == "" && aliases[name] {
			return nil
		}
		if scope.clauses != nil {
			switch scope.clauses.attribute(field, scope.own) {
			case ownerJoined:
				return nil
			case ownerUnknown:
				if !sets.allowsWhole(name) {
					return deny(scope.usage, name)
				}
				return unsupported(fmt.Sprintf("%s field %q cannot be attributed to a source", scope.usage, name))
			}
		}
		if !sets.allowsWhole(name) {
			return deny(scope.usage, name)
		}
		return nil
	}
	// checkCondition holds a condition to the allow-list. depth is the number of
	// groups the condition stands in, whether they are held by value or by pointer;
	// a condition nested deeper than maxQueryNesting, which includes one that holds
	// itself, is refused.
	var checkCondition func(dal.Condition, fieldScope, int) error
	checkCondition = func(condition dal.Condition, scope fieldScope, depth int) error {
		if depth > maxQueryNesting {
			return &DeniedError{Decision: Decision{Operation: Query, Resource: resource, Policy: "fields", Effect: effectDeny.String(), Code: CodeEnforcementUnsupported, Scope: DecisionScopeColumn, Slot: DecisionSlotWhere, Explanation: "filter condition is nested too deeply to be checked against allowed fields"}}
		}
		switch condition := condition.(type) {
		case nil:
			return nil
		case dal.Comparison:
			for _, expression := range []dal.Expression{condition.Left, condition.Right} {
				switch expression.(type) {
				case dal.Constant, *dal.Constant, dal.Array, *dal.Array, dal.Param, *dal.Param:
					continue
				}
				if err := checkExpression(expression, scope); err != nil {
					return err
				}
			}
			return nil
		case dal.IsNullCondition:
			// An IS [NOT] NULL test reveals whether a field is set, so its
			// operand is held to the field allow-list. Only a literal is
			// waved through: unlike a comparison operand, a param here is not
			// something a field is compared with, and DTQL does not accept one
			// as a null-test operand, so it fails closed.
			switch condition.Operand().(type) {
			case dal.Constant, *dal.Constant:
				return nil
			}
			return checkExpression(condition.Operand(), scope)
		case *dal.Comparison:
			if condition == nil {
				return nil
			}
			return checkCondition(*condition, scope, depth)
		case dal.GroupCondition:
			for _, child := range condition.Conditions() {
				if err := checkCondition(child, scope, depth+1); err != nil {
					return err
				}
			}
			return nil
		case *dal.GroupCondition:
			if condition == nil {
				return nil
			}
			return checkCondition(*condition, scope, depth)
		default:
			return &DeniedError{Decision: Decision{Operation: Query, Resource: resource, Policy: "fields", Effect: effectDeny.String(), Code: CodeEnforcementUnsupported, Scope: DecisionScopeColumn, Slot: DecisionSlotWhere, Explanation: "filter condition cannot be safely checked against allowed fields"}}
		}
	}
	if err := checkCondition(query.Where(), fieldScope{usage: usageFilter}, 0); err != nil {
		return err
	}
	for _, expression := range query.GroupBy() {
		if err := checkExpression(expression, fieldScope{usage: usageGroup}); err != nil {
			return err
		}
	}
	if err := checkCondition(query.Having(), fieldScope{usage: usageFilter, aggregates: true, aliases: true}, 0); err != nil {
		return err
	}
	for _, order := range query.OrderBy() {
		if order == nil {
			return deny(usageOrder, "")
		}
		if err := checkExpression(order.Expression(), fieldScope{usage: usageOrder, aggregates: true, aliases: true}); err != nil {
			return err
		}
	}
	// The list applies to the base source. A field in a join condition or a scan
	// order is held to it when it belongs to the base, let through when it
	// belongs to a joined source, and refused when it cannot be attributed to
	// either.
	if from := query.From(); from != nil {
		clauses, err := collectJoinClauses(from)
		if err != nil {
			return unsupported("join conditions and scan orders cannot be checked: " + err.Error())
		}
		for _, condition := range clauses.conditions {
			if err := checkCondition(condition, fieldScope{usage: usageJoin, clauses: clauses, own: ownerBase}, 0); err != nil {
				return err
			}
		}
		for _, scan := range clauses.scans {
			if isNilNode(scan.order) {
				return deny(usageScan, "")
			}
			// A scan orders its own source alone, whatever qualifier a field
			// carries, so every field of a scan order of the base source is held
			// to the list. In the scan order of a joined source a field of the base
			// is held to it too, and a field of a joined source is not.
			scope := fieldScope{usage: usageScan, clauses: clauses, own: scan.owner}
			if scan.owner == ownerBase {
				scope.clauses = nil
			}
			if err := checkExpression(scan.order.Expression(), scope); err != nil {
				return err
			}
		}
	}
	wildcards := 0
	for i, column := range query.Columns() {
		if column.Wildcard != nil {
			wildcards++
			if wildcards > 1 || i != 0 {
				return unsupported("wildcard projection must be the first and only wildcard item")
			}
			if query.From() != nil && len(query.From().Joins()) != 0 {
				return unsupported("wildcard projection with joins cannot be safely enforced")
			}
			if column.Expression != nil || column.Alias != "" {
				return unsupported("wildcard projection cannot contain an expression or alias")
			}
			if len(column.Wildcard.Exclude) == 0 {
				return unsupported("wildcard projection requires at least one exclusion")
			}
			for _, name := range column.Wildcard.Exclude {
				if name == "" {
					return unsupported("wildcard projection contains an empty exclusion")
				}
			}
			if source := column.Wildcard.Source; source != "" {
				from := query.From()
				if from == nil || from.Base() == nil || source != from.Base().Name() && source != from.Base().Alias() {
					return unsupported("wildcard projection source does not match the query source")
				}
			}
			// A wildcard exclusion names columns to omit, not columns the caller
			// demands access to. projectQuery intersects it with enumerable field
			// policy; non-enumerable policies fall back to result redaction.
			continue
		}
		if column.Expression == nil {
			return deny(usageSelected, "")
		}
		if err := checkExpression(column.Expression, fieldScope{usage: usageSelected, aggregates: true}); err != nil {
			return err
		}
	}
	return nil
}

// allowedAggregateAliases names the aliases of the select list that stand for an
// aggregate the field list allows. A name that two columns come back under
// stands for neither.
func allowedAggregateAliases(columns []dal.Column, sets fieldSets) map[string]bool {
	count := map[string]int{}
	for _, column := range columns {
		if name, named := outputName(column); named {
			count[name]++
		}
	}
	aliases := map[string]bool{}
	for _, column := range columns {
		if _, isAggregate := column.Expression.(dal.AggregateFunc); isAggregate && column.Alias != "" && count[column.Alias] == 1 && sets.readsOnlyAllowedFields(column.Expression) {
			aliases[column.Alias] = true
		}
	}
	return aliases
}

// aggregateFunctions are the aggregate functions DALgo defines. Only these are
// checked by their operands; an aggregate under any other name might read
// anything.
var aggregateFunctions = map[string]bool{
	dal.COUNT: true, dal.SUM: true, dal.AVERAGE: true, dal.MIN: true, dal.MAX: true, dal.FIRST: true, dal.LAST: true,
}

// aggregateFields lists, left to right, the fields an aggregate reads, then the
// fields of its order, when it has one: an order key is read to sort the rows, so
// it is held to the list as an operand is. checkable is false when the function is
// not one DALgo defines, or an operand is something a field list cannot be applied
// to: a param, an array, a subquery, an expression the check does not know, an
// arithmetic operator other than + - * /, a star anywhere but directly under
// COUNT, or nesting past maxQueryNesting; or when an order key is not a field. A
// constant reads no field, and COUNT(*) reads no field value.
func aggregateFields(aggregate dal.AggregateFunc, depth int) (fields []string, checkable bool) {
	if !aggregateFunctions[strings.ToUpper(aggregate.FuncName())] {
		return nil, false
	}
	for _, argument := range aggregate.FuncArgs() {
		if star, isStar := argument.(dal.StarExpression); isStar && star.IsStar() && strings.EqualFold(aggregate.FuncName(), dal.COUNT) {
			continue
		}
		operand, ok := operandFields(argument, depth+1)
		if !ok {
			return nil, false
		}
		fields = append(fields, operand...)
	}
	if ordered, isOrdered := aggregate.(dal.OrderedAggregateFunc); isOrdered {
		for _, key := range ordered.AggregateOrder() {
			name, isField := orderKeyField(key)
			if !isField {
				return nil, false
			}
			fields = append(fields, name)
		}
	}
	return fields, true
}

// orderKeyField returns the name of the field an order key sorts by; ok is false
// for a key that is missing or is not a field (given by value or by non-nil
// pointer), which a field list cannot be applied to.
func orderKeyField(key dal.OrderExpression) (name string, ok bool) {
	if isNilNode(key) {
		return "", false
	}
	switch expression := key.Expression().(type) {
	case dal.FieldRef:
		return expression.Name(), true
	case *dal.FieldRef:
		if expression != nil {
			return expression.Name(), true
		}
	}
	return "", false
}

// operandFields lists the fields an aggregate operand reads, in the order the
// operand names them; ok is false for an operand that cannot be checked.
func operandFields(expression dal.Expression, depth int) (fields []string, ok bool) {
	if depth > maxQueryNesting {
		return nil, false
	}
	switch expression := expression.(type) {
	case dal.FieldRef:
		return []string{expression.Name()}, true
	case *dal.FieldRef:
		if expression != nil {
			return []string{expression.Name()}, true
		}
	case dal.Constant, *dal.Constant:
		return nil, true
	case dal.BinaryExpression:
		return binaryFields(expression, depth)
	case *dal.BinaryExpression:
		if expression != nil {
			return binaryFields(*expression, depth)
		}
	case dal.AggregateFunc:
		return aggregateFields(expression, depth)
	}
	return nil, false
}

func binaryFields(expression dal.BinaryExpression, depth int) ([]string, bool) {
	switch expression.Operator {
	case dal.Add, dal.Subtract, dal.Multiply, dal.Divide:
	default:
		return nil, false
	}
	left, ok := operandFields(expression.Left, depth+1)
	if !ok {
		return nil, false
	}
	right, ok := operandFields(expression.Right, depth+1)
	return append(left, right...), ok
}
