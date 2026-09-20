package access

import (
	"context"
	"fmt"

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
	checkExpression := func(expression dal.Expression, usage string) error {
		field, ok := expression.(dal.FieldRef)
		if !ok {
			if pointer, pointerOK := expression.(*dal.FieldRef); pointerOK && pointer != nil {
				field, ok = *pointer, true
			}
		}
		if !ok {
			return &DeniedError{Decision: Decision{Operation: Query, Resource: resource, Policy: "fields", Effect: effectDeny.String(), Code: CodeEnforcementUnsupported, Scope: DecisionScopeColumn, Slot: DecisionSlotFields, Explanation: fmt.Sprintf("%s expression cannot be safely checked against allowed fields", usage)}}
		}
		if !sets.allowsWhole(field.Name()) {
			return deny(usage, field.Name())
		}
		return nil
	}
	var checkCondition func(dal.Condition) error
	checkCondition = func(condition dal.Condition) error {
		switch condition := condition.(type) {
		case nil:
			return nil
		case dal.Comparison:
			for _, expression := range []dal.Expression{condition.Left, condition.Right} {
				switch expression.(type) {
				case dal.Constant, *dal.Constant, dal.Array, *dal.Array, dal.Param, *dal.Param:
					continue
				}
				if err := checkExpression(expression, "filter"); err != nil {
					return err
				}
			}
			return nil
		case *dal.Comparison:
			if condition == nil {
				return nil
			}
			return checkCondition(*condition)
		case dal.GroupCondition:
			for _, child := range condition.Conditions() {
				if err := checkCondition(child); err != nil {
					return err
				}
			}
			return nil
		case *dal.GroupCondition:
			if condition == nil {
				return nil
			}
			return checkCondition(*condition)
		default:
			return &DeniedError{Decision: Decision{Operation: Query, Resource: resource, Policy: "fields", Effect: effectDeny.String(), Code: CodeEnforcementUnsupported, Scope: DecisionScopeColumn, Slot: DecisionSlotWhere, Explanation: "filter condition cannot be safely checked against allowed fields"}}
		}
	}
	if err := checkCondition(query.Where()); err != nil {
		return err
	}
	for _, expression := range query.GroupBy() {
		if err := checkExpression(expression, "group"); err != nil {
			return err
		}
	}
	if err := checkCondition(query.Having()); err != nil {
		return err
	}
	for _, order := range query.OrderBy() {
		if order == nil {
			return deny("order", "")
		}
		if err := checkExpression(order.Expression(), "order"); err != nil {
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
		if err := checkExpression(column.Expression, "selected"); err != nil {
			return err
		}
	}
	return nil
}
