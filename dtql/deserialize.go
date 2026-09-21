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
	query, err := documentToQueryAt(doc, "")
	if err != nil {
		return nil, err
	}
	if dal.HasSubquery(query) {
		if err := dal.ValidateQueryScope(query); err != nil {
			return nil, fmt.Errorf("invalid DTQL: %w", err)
		}
	}
	return query, nil
}

func documentToQueryAt(doc document, path string) (dal.StructuredQuery, error) {
	if doc.Limit < 0 || doc.Offset < 0 {
		return nil, fmt.Errorf("invalid DTQL: query_shape at %slimit and offset must be non-negative", pathPrefix(path))
	}
	fromPath := "from"
	if path != "" {
		fromPath = path + ".from"
	}
	from, err := fromFromYAML(doc.From, fromPath)
	if err != nil {
		return nil, err
	}
	if err := dal.ValidateJoinTree(from); err != nil {
		return nil, fmt.Errorf("invalid DTQL: %w", err)
	}
	qb := from.NewQuery()

	if doc.Where != nil {
		cond, err := condFromYAMLAt(*doc.Where, pathJoin(path, "where"))
		if err != nil {
			return nil, err
		}
		qb.Where(cond)
	}
	if len(doc.GroupBy) > 0 {
		groups := make([]dal.Expression, len(doc.GroupBy))
		for i, encoded := range doc.GroupBy {
			expr, err := exprFromYAMLAt(encoded, fmt.Sprintf("%s[%d]", pathJoin(path, "groupBy"), i))
			if err != nil {
				return nil, fmt.Errorf("invalid DTQL: groupBy #%d: %w", i, err)
			}
			groups[i] = expr
		}
		qb.GroupBy(groups...)
	}
	if doc.Having != nil {
		condition, err := condFromYAMLAt(*doc.Having, pathJoin(path, "having"))
		if err != nil {
			return nil, fmt.Errorf("invalid DTQL: having: %w", err)
		}
		qb.Having(condition)
	}

	orderBy, err := orderFromYAMLAt(doc.OrderBy, pathJoin(path, "orderBy"))
	if err != nil {
		return nil, err
	}
	if len(orderBy) > 0 {
		qb.OrderBy(orderBy...)
	}
	qb.Limit(doc.Limit)
	qb.Offset(doc.Offset)

	columns, err := columnsFromYAMLAt(doc.Columns, doc.From, pathJoin(path, "columns"))
	if err != nil {
		return nil, err
	}

	base := reconstructedQuery{StructuredQuery: qb.SelectIntoRecordset(), columns: columns}
	if len(from.Joins()) > 0 && !dal.HasSubquery(base) {
		if err := validateJoinClauseFields(base); err != nil {
			return nil, fmt.Errorf("invalid DTQL: %w", err)
		}
	}
	if err := dal.ValidateAggregation(base); err != nil {
		return nil, fmt.Errorf("invalid DTQL: aggregation: %w", err)
	}
	return base, nil
}

func fromFromYAML(encoded fromYAML, path string) (dal.FromSource, error) {
	if encoded.Name == "" && encoded.Query == nil {
		if path == "from" {
			return nil, fmt.Errorf("invalid DTQL: from.name is required")
		}
		return nil, fmt.Errorf("invalid DTQL: join_shape at %s.name: name is required", path)
	}
	if encoded.Name != "" && encoded.Query != nil {
		return nil, fmt.Errorf("invalid DTQL: query_shape at %s: exactly one of name or query is required", path)
	}
	var source dal.RecordsetSource
	if encoded.Query != nil {
		if encoded.Schema != nil || encoded.Alias != "" {
			return nil, fmt.Errorf("invalid DTQL: query_shape at %s: query source cannot set schema or alias", path)
		}
		if encoded.Query.As == "" {
			return nil, fmt.Errorf("invalid DTQL: query_shape at %s.query.as: as is required", path)
		}
		query, err := documentToQueryAt(*encoded.Query, path+".query")
		if err != nil {
			return nil, err
		}
		source = dal.NewQuerySource(query, encoded.Query.As)
	} else if encoded.Schema == nil {
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
		if join.hintsRepeated {
			return nil, &dal.JoinValidationError{Category: "join_algorithm", Path: joinPath + ".hints.algorithms", Message: "hints must appear once"}
		}
		algorithms, err := algorithmsFromYAML(join.Hints, joinPath)
		if err != nil {
			return nil, fmt.Errorf("invalid DTQL: %w", err)
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
		joined := dal.NewJoinedFrom(child, joinType, on...)
		if join.Hints != nil {
			joined = joined.WithAlgorithms(algorithms...)
		}
		from.Join(joined)
	}
	return from, nil
}

func columnsFromYAML(cols []columnYAML, from fromYAML) ([]dal.Column, error) {
	return columnsFromYAMLAt(cols, from, "columns")
}

func columnsFromYAMLAt(cols []columnYAML, from fromYAML, path string) ([]dal.Column, error) {
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
		expr, err := exprFromYAMLAt(c.exprYAML, fmt.Sprintf("%s[%d]", path, i))
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
	if e.Query != nil {
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
	validSource := wildcard.Source == "" || fromHasAlias(from, wildcard.Source)
	if len(from.Joins) == 0 && wildcard.Source == from.Name {
		validSource = true
	} // legacy single source
	if !validSource {
		return fmt.Errorf("wildcard source %q does not match from name or alias", wildcard.Source)
	}
	return nil
}

func fromHasAlias(from fromYAML, alias string) bool {
	name := from.Alias
	if name == "" {
		name = from.Name
	}
	if alias == name {
		return true
	}
	for _, join := range from.Joins {
		if join.From != nil && fromHasAlias(*join.From, alias) {
			return true
		}
	}
	return false
}

func orderFromYAML(orders []orderYAML) ([]dal.OrderExpression, error) {
	return orderFromYAMLAt(orders, "orderBy")
}

func orderFromYAMLAt(orders []orderYAML, path string) ([]dal.OrderExpression, error) {
	if len(orders) == 0 {
		return nil, nil
	}
	out := make([]dal.OrderExpression, 0, len(orders))
	for i, o := range orders {
		expr, err := exprFromYAMLAt(o.exprYAML, fmt.Sprintf("%s[%d]", path, i))
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
	return exprFromYAMLAt(e, "expression")
}

func exprFromYAMLAt(e exprYAML, path string) (dal.Expression, error) {
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
			arg, err := exprFromYAMLAt(encoded, fmt.Sprintf("%s.aggregate.args[%d]", path, i))
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
		left, err := exprFromYAMLAt(*e.Binary.Left, path+".binary.left")
		if err != nil {
			return nil, fmt.Errorf("binary left: %w", err)
		}
		right, err := exprFromYAMLAt(*e.Binary.Right, path+".binary.right")
		if err != nil {
			return nil, fmt.Errorf("binary right: %w", err)
		}
		return dal.Binary(left, dal.ArithmeticOperator(e.Binary.Op), right), nil
	case e.Query != nil:
		query, err := documentToQueryAt(*e.Query, path+".query")
		if err != nil {
			return nil, err
		}
		return dal.NewQueryExpression(query, e.Query.As), nil
	default: // e.Values != nil
		if !portableScalarArray(e.Values) {
			return nil, fmt.Errorf("values must be an array of scalars")
		}
		return dal.Array{Value: e.Values}, nil
	}
}

func condFromYAML(c condYAML) (dal.Condition, error) {
	return condFromYAMLAt(c, "condition")
}

func condFromYAMLAt(c condYAML, path string) (dal.Condition, error) {
	isComparison := c.Op != "" || c.Left != nil || c.Right != nil
	hasAnd := c.And != nil
	hasOr := c.Or != nil
	hasExists := c.Exists != nil
	hasNotExists := c.NotExists != nil

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
	if hasExists {
		forms++
	}
	if hasNotExists {
		forms++
	}
	switch {
	case forms == 0:
		return nil, fmt.Errorf("invalid DTQL: condition must be a comparison (op/left/right) or a group (and/or)")
	case forms > 1:
		return nil, fmt.Errorf("invalid DTQL: condition mixes comparison and group forms")
	}

	if isComparison {
		return comparisonFromYAMLAt(c, path)
	}
	if hasAnd {
		return groupFromYAMLAt(dal.And, c.And, path+".and")
	}
	if hasOr {
		return groupFromYAMLAt(dal.Or, c.Or, path+".or")
	}
	if c.Exists != nil {
		if c.Exists.Query == nil {
			return nil, fmt.Errorf("invalid DTQL: query_shape at %s.exists.query: query is required", path)
		}
		query, err := documentToQueryAt(*c.Exists.Query, path+".exists.query")
		if err != nil {
			return nil, err
		}
		return dal.NewExistsCondition(query), nil
	}
	if c.NotExists.Query == nil {
		return nil, fmt.Errorf("invalid DTQL: query_shape at %s.notExists.query: query is required", path)
	}
	query, err := documentToQueryAt(*c.NotExists.Query, path+".notExists.query")
	if err != nil {
		return nil, err
	}
	return dal.NewNotExistsCondition(query), nil
}

func comparisonFromYAML(c condYAML) (dal.Condition, error) {
	return comparisonFromYAMLAt(c, "condition")
}
func comparisonFromYAMLAt(c condYAML, path string) (dal.Condition, error) {
	op := dal.Operator(c.Op)
	if !inScopeComparisonOps[op] {
		return nil, fmt.Errorf("invalid DTQL: unknown comparison operator %q", c.Op)
	}
	if c.Left == nil || c.Right == nil {
		return nil, fmt.Errorf("invalid DTQL: comparison requires both left and right")
	}
	left, err := exprFromYAMLAt(*c.Left, path+".left")
	if err != nil {
		return nil, fmt.Errorf("invalid DTQL: comparison left: %w", err)
	}
	right, err := exprFromYAMLAt(*c.Right, path+".right")
	if err != nil {
		return nil, fmt.Errorf("invalid DTQL: comparison right: %w", err)
	}
	return dal.NewComparison(left, op, right), nil
}

func groupFromYAML(op dal.Operator, subs []condYAML) (dal.Condition, error) {
	return groupFromYAMLAt(op, subs, "condition")
}
func groupFromYAMLAt(op dal.Operator, subs []condYAML, path string) (dal.Condition, error) {
	if len(subs) == 0 {
		return nil, fmt.Errorf("invalid DTQL: %q group must have at least one condition", op)
	}
	conditions := make([]dal.Condition, 0, len(subs))
	for i, sub := range subs {
		cond, err := condFromYAMLAt(sub, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			return nil, fmt.Errorf("group condition #%d: %w", i, err)
		}
		conditions = append(conditions, cond)
	}
	return dal.NewGroupCondition(op, conditions...), nil
}

func pathJoin(path, suffix string) string {
	if path == "" {
		return suffix
	}
	return path + "." + suffix
}
func pathPrefix(path string) string {
	if path == "" {
		return ""
	}
	return path + "."
}
