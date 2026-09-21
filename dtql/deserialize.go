package dtql

import (
	"bytes"
	"fmt"
	"io"

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
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("invalid DTQL-YAML: multiple documents are not allowed")
		}
		return nil, fmt.Errorf("invalid DTQL-YAML: %w", err)
	}
	return documentToQuery(doc)
}

func documentToQuery(doc document) (dal.StructuredQuery, error) {
	if doc.From.Name == "" {
		return nil, fmt.Errorf("invalid DTQL: from.name is required")
	}
	if doc.Limit < 0 || doc.Offset < 0 {
		return nil, fmt.Errorf("invalid DTQL: limit and offset must be non-negative")
	}
	from, err := fromFromYAML(doc.From, "from")
	if err != nil {
		return nil, err
	}
	if err := dal.ValidateJoinTree(from); err != nil {
		return nil, fmt.Errorf("invalid DTQL: %w", err)
	}
	qb := from.NewQuery()

	if doc.Where != nil {
		cond, err := condFromYAML(*doc.Where)
		if err != nil {
			return nil, err
		}
		qb.Where(cond)
	}
	if len(doc.GroupBy) > 0 {
		groups := make([]dal.Expression, len(doc.GroupBy))
		for i, encoded := range doc.GroupBy {
			expr, err := exprFromYAML(encoded)
			if err != nil {
				return nil, fmt.Errorf("invalid DTQL: groupBy #%d: %w", i, err)
			}
			groups[i] = expr
		}
		qb.GroupBy(groups...)
	}
	if doc.Having != nil {
		condition, err := condFromYAML(*doc.Having)
		if err != nil {
			return nil, fmt.Errorf("invalid DTQL: having: %w", err)
		}
		qb.Having(condition)
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

	columns, err := columnsFromYAML(doc.Columns, doc.From)
	if err != nil {
		return nil, err
	}

	base := reconstructedQuery{StructuredQuery: qb.SelectIntoRecordset(), columns: columns}
	if err := dal.ValidateAggregation(base); err != nil {
		return nil, fmt.Errorf("invalid DTQL: aggregation: %w", err)
	}
	return base, nil
}

func fromFromYAML(encoded fromYAML, path string) (dal.FromSource, error) {
	if encoded.Name == "" {
		return nil, fmt.Errorf("invalid DTQL: join_shape at %s.name: name is required", path)
	}
	var source dal.CollectionRef
	if encoded.Schema == nil {
		source = dal.NewRootCollectionRef(encoded.Name, encoded.Alias)
	} else {
		if *encoded.Schema == "" {
			return nil, fmt.Errorf("invalid DTQL: %s.schema must not be empty (join_shape at %s.schema)", path, path)
		}
		source = dal.NewQualifiedRootCollectionRef(*encoded.Schema, encoded.Name, encoded.Alias)
	}
	from := dal.From(source)
	for i, join := range encoded.Joins {
		joinPath := fmt.Sprintf("%s.joins[%d]", path, i)
		if join.From == nil {
			return nil, fmt.Errorf("invalid DTQL: join_shape at %s.from: from is required", joinPath)
		}
		if len(join.On) == 0 {
			return nil, fmt.Errorf("invalid DTQL: join_shape at %s.on: on must contain at least one predicate", joinPath)
		}
		child, err := fromFromYAML(*join.From, joinPath+".from")
		if err != nil {
			return nil, err
		}
		joinType := dal.JoinInner
		switch join.Type {
		case "", "inner":
		case "left":
			joinType = dal.JoinLeft
		default:
			return nil, fmt.Errorf("invalid DTQL: join_type at %s.type: unsupported join type %q", joinPath, join.Type)
		}
		on := make([]dal.Condition, 0, len(join.On))
		for j, predicate := range join.On {
			if predicate.Op == "eq" {
				predicate.Op = string(dal.Equal)
			}
			condition, err := condFromYAML(predicate)
			if err != nil {
				return nil, fmt.Errorf("invalid DTQL: join_shape at %s.on[%d]: %w", joinPath, j, err)
			}
			on = append(on, condition)
		}
		from.Join(dal.NewJoinedFrom(child, joinType, on...))
	}
	return from, nil
}

func columnsFromYAML(cols []columnYAML, from fromYAML) ([]dal.Column, error) {
	if len(cols) == 0 {
		return nil, nil
	}
	out := make([]dal.Column, 0, len(cols))
	for i, c := range cols {
		if c.Wildcard != nil {
			if c.expressionKeyCount != 0 || c.sourcePresent {
				return nil, fmt.Errorf("invalid DTQL: column #%d mixes wildcard and expression forms", i)
			}
			if c.asPresent {
				return nil, fmt.Errorf("invalid DTQL: column #%d wildcard cannot have an alias", i)
			}
			if err := validateWildcardYAML(*c.Wildcard, from); err != nil {
				return nil, fmt.Errorf("invalid DTQL: column #%d: %w", i, err)
			}
			out = append(out, dal.AllColumnsExceptFrom(c.Wildcard.Source, c.Wildcard.Exclude...))
			continue
		}
		expr, err := exprFromYAML(c.exprYAML)
		if err != nil {
			return nil, fmt.Errorf("invalid DTQL: column #%d: %w", i, err)
		}
		out = append(out, dal.Column{Expression: expr, Alias: c.As})
	}
	return out, nil
}

func expressionFieldsSet(e exprYAML) int {
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
	if e.Param != "" {
		set++
	}
	if e.Star {
		set++
	}
	if e.Aggregate != nil {
		set++
	}
	if e.Binary != nil {
		set++
	}
	return set
}

func validateWildcardYAML(wildcard wildcardYAML, from fromYAML) error {
	if len(wildcard.Exclude) == 0 {
		return fmt.Errorf("wildcard.exclude must contain at least one column name")
	}
	for i, name := range wildcard.Exclude {
		if name == "" {
			return fmt.Errorf("wildcard.exclude #%d must not be empty", i)
		}
	}
	if wildcard.Source != "" && wildcard.Source != from.Alias && wildcard.Source != from.Name {
		return fmt.Errorf("wildcard source %q does not match from name or alias", wildcard.Source)
	}
	return nil
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
	if e.sourcePresent && e.Field == "" {
		return nil, fmt.Errorf("source is valid only with field")
	}
	set := expressionFieldsSet(e)
	if set == 0 {
		return nil, fmt.Errorf("expression must set exactly one expression form")
	}
	if set > 1 {
		return nil, fmt.Errorf("expression must set exactly one expression form, but several are set")
	}
	switch {
	case e.Field != "":
		return dal.NewFieldRef(e.Source, e.Field), nil
	case e.Value != nil:
		if !portableScalar(*e.Value) {
			return nil, fmt.Errorf("value must be a scalar")
		}
		return dal.Constant{Value: *e.Value}, nil
	case e.Param != "":
		if !dal.ValidParamName(e.Param) {
			return nil, fmt.Errorf("invalid parameter name %q", e.Param)
		}
		return dal.Param{Name: e.Param}, nil
	case e.Star:
		return dal.Star(), nil
	case e.Aggregate != nil:
		if e.Aggregate.Function == "" {
			return nil, fmt.Errorf("aggregate.function is required")
		}
		args := make([]dal.Expression, len(e.Aggregate.Args))
		for i, encoded := range e.Aggregate.Args {
			arg, err := exprFromYAML(encoded)
			if err != nil {
				return nil, fmt.Errorf("aggregate argument #%d: %w", i, err)
			}
			args[i] = arg
		}
		aggregate := dal.NewAggregate(e.Aggregate.Function, e.Aggregate.Distinct, args...)
		return aggregate, nil
	case e.Binary != nil:
		if e.Binary.Left == nil || e.Binary.Right == nil {
			return nil, fmt.Errorf("binary requires left and right")
		}
		left, err := exprFromYAML(*e.Binary.Left)
		if err != nil {
			return nil, fmt.Errorf("binary left: %w", err)
		}
		right, err := exprFromYAML(*e.Binary.Right)
		if err != nil {
			return nil, fmt.Errorf("binary right: %w", err)
		}
		return dal.Binary(left, dal.ArithmeticOperator(e.Binary.Op), right), nil
	default: // e.Values != nil
		if !portableScalarArray(e.Values) {
			return nil, fmt.Errorf("values must be an array of scalars")
		}
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
