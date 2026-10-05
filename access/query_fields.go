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

func validateRequestedQueryFields(query dal.StructuredQuery, sets fieldSets) error {
	resource := resourcesForQuery(query)[0]
	unsupported := func(explanation string) error {
		return &DeniedError{Decision: Decision{Operation: Query, Resource: resource, Policy: "fields", Effect: effectDeny.String(), Code: CodeEnforcementUnsupported, Scope: DecisionScopeColumn, Slot: DecisionSlotFields, Explanation: explanation}}
	}
	deny := func(usage, field string) error {
		slot := DecisionSlotFields
		if usage == "filter" {
			slot = DecisionSlotWhere
		}
		return &DeniedError{Decision: Decision{Operation: Query, Resource: resource, Policy: "fields", Effect: effectDeny.String(), Code: CodeColumnDenied, Scope: DecisionScopeColumn, Slot: slot, Columns: [][]string{{field}}, Explanation: fmt.Sprintf("%s field %q is not allowed", usage, field)}}
	}
	// checkExpression holds an expression to the allow-list. An aggregate is
	// accepted only where aggregates belong (the select list, HAVING and ORDER
	// BY); anywhere else, and in any slot when it is not one DALgo defines, it
	// is refused as unsupported.
	checkExpression := func(expression dal.Expression, usage string, aggregatesAllowed bool) error {
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
			if aggregate, isAggregate := expression.(dal.AggregateFunc); isAggregate && aggregatesAllowed {
				if fields, checkable := aggregateFields(aggregate, 0); checkable {
					for _, name := range fields {
						if !sets.allowsWhole(name) {
							return deny(usage, name)
						}
					}
					return nil
				}
			}
			return &DeniedError{Decision: Decision{Operation: Query, Resource: resource, Policy: "fields", Effect: effectDeny.String(), Code: CodeEnforcementUnsupported, Scope: DecisionScopeColumn, Slot: DecisionSlotFields, Explanation: fmt.Sprintf("%s expression cannot be safely checked against allowed fields", usage)}}
		}
		if !sets.allowsWhole(field.Name()) {
			return deny(usage, field.Name())
		}
		return nil
	}
	// checkCondition holds a condition to the allow-list; aggregatesAllowed is
	// true for HAVING and false for WHERE.
	var checkCondition func(dal.Condition, bool) error
	checkCondition = func(condition dal.Condition, aggregatesAllowed bool) error {
		switch condition := condition.(type) {
		case nil:
			return nil
		case dal.Comparison:
			for _, expression := range []dal.Expression{condition.Left, condition.Right} {
				switch expression.(type) {
				case dal.Constant, *dal.Constant, dal.Array, *dal.Array, dal.Param, *dal.Param:
					continue
				}
				if err := checkExpression(expression, "filter", aggregatesAllowed); err != nil {
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
			return checkExpression(condition.Operand(), "filter", aggregatesAllowed)
		case *dal.Comparison:
			if condition == nil {
				return nil
			}
			return checkCondition(*condition, aggregatesAllowed)
		case dal.GroupCondition:
			for _, child := range condition.Conditions() {
				if err := checkCondition(child, aggregatesAllowed); err != nil {
					return err
				}
			}
			return nil
		case *dal.GroupCondition:
			if condition == nil {
				return nil
			}
			return checkCondition(*condition, aggregatesAllowed)
		default:
			return &DeniedError{Decision: Decision{Operation: Query, Resource: resource, Policy: "fields", Effect: effectDeny.String(), Code: CodeEnforcementUnsupported, Scope: DecisionScopeColumn, Slot: DecisionSlotWhere, Explanation: "filter condition cannot be safely checked against allowed fields"}}
		}
	}
	if err := checkCondition(query.Where(), false); err != nil {
		return err
	}
	for _, expression := range query.GroupBy() {
		if err := checkExpression(expression, "group", false); err != nil {
			return err
		}
	}
	if err := checkCondition(query.Having(), true); err != nil {
		return err
	}
	for _, order := range query.OrderBy() {
		if order == nil {
			return deny("order", "")
		}
		if err := checkExpression(order.Expression(), "order", true); err != nil {
			return err
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
			return deny("selected", "")
		}
		if err := checkExpression(column.Expression, "selected", true); err != nil {
			return err
		}
	}
	return nil
}

// aggregateFunctions are the aggregate functions DALgo defines. Only these are
// checked by their operands; an aggregate under any other name might read
// anything.
var aggregateFunctions = map[string]bool{
	dal.COUNT: true, dal.SUM: true, dal.AVERAGE: true, dal.MIN: true, dal.MAX: true, dal.FIRST: true, dal.LAST: true,
}

// aggregateFields lists, left to right, the fields an aggregate reads. checkable
// is false when the function is not one DALgo defines, or an operand is
// something a field list cannot be applied to: a param, an array, a subquery,
// an expression the check does not know, an arithmetic operator other than
// + - * /, a star anywhere but directly under COUNT, or nesting past
// maxQueryNesting. A constant reads no field, and COUNT(*) reads no field value.
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
	return fields, true
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
