package dtql

import (
	"bytes"
	"fmt"

	"github.com/dal-go/dalgo/dal"
	"gopkg.in/yaml.v3"
)

// Deserialize reconstructs a dal.StructuredQuery from a DTQL-YAML document.
// Malformed or schema-invalid input — unknown keys, wrong value types, missing
// required fields, or an unknown operator — yields a descriptive error and no
// (partially-populated) query.
func Deserialize(data []byte) (dal.StructuredQuery, error) {
	var doc document
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true) // reject unknown keys
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("invalid DTQL-YAML: %w", err)
	}
	return documentToQuery(doc)
}

func documentToQuery(doc document) (dal.StructuredQuery, error) {
	if doc.From.Name == "" {
		return nil, fmt.Errorf("invalid DTQL: from.name is required")
	}
	source := dal.NewRootCollectionRef(doc.From.Name, doc.From.Alias)
	qb := dal.From(source).NewQuery()

	if doc.Where != nil {
		cond, err := condFromYAML(*doc.Where)
		if err != nil {
			return nil, err
		}
		qb.Where(cond)
	}

	orderBy, err := orderFromYAML(doc.OrderBy)
	if err != nil {
		return nil, err
	}
	if len(orderBy) > 0 {
		qb.OrderBy(orderBy...)
	}
	qb.Limit(doc.Limit)
	qb.Offset(doc.Offset)

	columns, err := columnsFromYAML(doc.Columns)
	if err != nil {
		return nil, err
	}

	base := qb.SelectIntoRecordset()
	return reconstructedQuery{StructuredQuery: base, columns: columns}, nil
}

func columnsFromYAML(cols []columnYAML) ([]dal.Column, error) {
	if len(cols) == 0 {
		return nil, nil
	}
	out := make([]dal.Column, 0, len(cols))
	for i, c := range cols {
		expr, err := exprFromYAML(c.exprYAML)
		if err != nil {
			return nil, fmt.Errorf("invalid DTQL: column #%d: %w", i, err)
		}
		out = append(out, dal.Column{Expression: expr, Alias: c.As})
	}
	return out, nil
}

func orderFromYAML(orders []orderYAML) ([]dal.OrderExpression, error) {
	if len(orders) == 0 {
		return nil, nil
	}
	out := make([]dal.OrderExpression, 0, len(orders))
	for i, o := range orders {
		expr, err := exprFromYAML(o.exprYAML)
		if err != nil {
			return nil, fmt.Errorf("invalid DTQL: orderBy #%d: %w", i, err)
		}
		if o.Desc {
			out = append(out, dal.Descending(expr))
		} else {
			out = append(out, dal.Ascending(expr))
		}
	}
	return out, nil
}

func exprFromYAML(e exprYAML) (dal.Expression, error) {
	set := 0
	if e.Field != "" {
		set++
	}
	if e.Value != nil {
		set++
	}
	if e.Values != nil {
		set++
	}
	if set == 0 {
		return nil, fmt.Errorf("expression must set exactly one of field, value or values")
	}
	if set > 1 {
		return nil, fmt.Errorf("expression must set exactly one of field, value or values, but several are set")
	}
	switch {
	case e.Field != "":
		return dal.NewFieldRef(e.Field), nil
	case e.Value != nil:
		return dal.Constant{Value: e.Value}, nil
	default: // e.Values != nil
		return dal.Array{Value: e.Values}, nil
	}
}

func condFromYAML(c condYAML) (dal.Condition, error) {
	isComparison := c.Op != "" || c.Left != nil || c.Right != nil
	hasAnd := c.And != nil
	hasOr := c.Or != nil

	forms := 0
	if isComparison {
		forms++
	}
	if hasAnd {
		forms++
	}
	if hasOr {
		forms++
	}
	switch {
	case forms == 0:
		return nil, fmt.Errorf("invalid DTQL: condition must be a comparison (op/left/right) or a group (and/or)")
	case forms > 1:
		return nil, fmt.Errorf("invalid DTQL: condition mixes comparison and group forms")
	}

	if isComparison {
		return comparisonFromYAML(c)
	}
	if hasAnd {
		return groupFromYAML(dal.And, c.And)
	}
	return groupFromYAML(dal.Or, c.Or)
}

func comparisonFromYAML(c condYAML) (dal.Condition, error) {
	op := dal.Operator(c.Op)
	if !inScopeComparisonOps[op] {
		return nil, fmt.Errorf("invalid DTQL: unknown comparison operator %q", c.Op)
	}
	if c.Left == nil || c.Right == nil {
		return nil, fmt.Errorf("invalid DTQL: comparison requires both left and right")
	}
	left, err := exprFromYAML(*c.Left)
	if err != nil {
		return nil, fmt.Errorf("invalid DTQL: comparison left: %w", err)
	}
	right, err := exprFromYAML(*c.Right)
	if err != nil {
		return nil, fmt.Errorf("invalid DTQL: comparison right: %w", err)
	}
	return dal.NewComparison(left, op, right), nil
}

func groupFromYAML(op dal.Operator, subs []condYAML) (dal.Condition, error) {
	if len(subs) == 0 {
		return nil, fmt.Errorf("invalid DTQL: %q group must have at least one condition", op)
	}
	conditions := make([]dal.Condition, 0, len(subs))
	for i, sub := range subs {
		cond, err := condFromYAML(sub)
		if err != nil {
			return nil, fmt.Errorf("group condition #%d: %w", i, err)
		}
		conditions = append(conditions, cond)
	}
	return dal.NewGroupCondition(op, conditions...), nil
}
