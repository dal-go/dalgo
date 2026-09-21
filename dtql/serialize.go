package dtql

import (
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/dal-go/dalgo/dal"
	"gopkg.in/yaml.v3"
)

// Serialize converts an in-scope dal.StructuredQuery into a canonical DTQL-YAML
// document. queryToDocument is the single validating pass: an out-of-scope query
// yields a descriptive error and no document.
func Serialize(q dal.StructuredQuery) ([]byte, error) {
	if q == nil {
		return nil, fmt.Errorf("query is not representable as DTQL: query is nil")
	}
	doc, err := queryToDocument(q)
	if err != nil {
		return nil, fmt.Errorf("query is not representable as DTQL: %w", err)
	}
	return marshalCanonical(doc), nil
}

// marshalCanonical marshals a document with stable 2-space indentation. Struct
// field order is fixed by the shape types, so the output is canonical. The
// document holds only strings, ints, bools and slices of those, so encoding it
// cannot fail.
func marshalCanonical(doc document) []byte {
	var buf yamlBuffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	_ = enc.Encode(doc)
	_ = enc.Close()
	return buf.b
}

type yamlBuffer struct{ b []byte }

func (w *yamlBuffer) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}

// queryToDocument validates and builds in one pass. It is the single enforcement
// point of the lossless guarantee: it rejects any out-of-scope construct
// (GroupBy, cursor, a non-root source, an unsupported expression,
// condition or operator) with a descriptive error rather than dropping it.
func queryToDocument(q dal.StructuredQuery) (document, error) {
	from := q.From()
	if from == nil {
		return document{}, fmt.Errorf("query has no From source")
	}
	if err := dal.ValidateJoinTree(from); err != nil {
		return document{}, err
	}
	if err := validateJoinClauseFields(q); err != nil {
		return document{}, err
	}
	if err := dal.ValidateAggregation(q); err != nil {
		return document{}, fmt.Errorf("invalid aggregation: %w", err)
	}
	if q.StartFrom() != "" {
		return document{}, fmt.Errorf("cursor (StartFrom) is not supported by DTQL")
	}
	if q.StartAfter() != "" {
		return document{}, fmt.Errorf("cursor (StartAfter) is not supported by DTQL")
	}
	if q.Limit() < 0 || q.Offset() < 0 {
		return document{}, fmt.Errorf("limit and offset must be non-negative")
	}
	fromDoc, err := fromToYAML(from)
	if err != nil {
		return document{}, err
	}
	doc := document{
		From:   fromDoc,
		Limit:  q.Limit(),
		Offset: q.Offset(),
	}
	for i, col := range q.Columns() {
		if col.Wildcard != nil {
			if col.Expression != nil {
				return document{}, fmt.Errorf("column #%d mixes wildcard and expression forms", i)
			}
			if col.Alias != "" {
				return document{}, fmt.Errorf("column #%d wildcard cannot have an alias", i)
			}
			wildcard := wildcardYAML{Source: col.Wildcard.Source, Exclude: append([]string(nil), col.Wildcard.Exclude...)}
			if err := validateWildcardYAML(wildcard, fromDoc); err != nil {
				return document{}, fmt.Errorf("column #%d: %w", i, err)
			}
			doc.Columns = append(doc.Columns, columnYAML{Wildcard: &wildcard})
			continue
		}
		expr, err := exprToYAML(col.Expression)
		if err != nil {
			return document{}, fmt.Errorf("column #%d: %w", i, err)
		}
		doc.Columns = append(doc.Columns, columnYAML{exprYAML: expr, As: col.Alias})
	}
	if where := q.Where(); where != nil {
		c, err := condToYAML(where)
		if err != nil {
			return document{}, err
		}
		doc.Where = c
	}
	for i, group := range q.GroupBy() {
		expr, err := exprToYAML(group)
		if err != nil {
			return document{}, fmt.Errorf("groupBy #%d: %w", i, err)
		}
		doc.GroupBy = append(doc.GroupBy, expr)
	}
	if having := q.Having(); having != nil {
		c, err := condToYAML(having)
		if err != nil {
			return document{}, fmt.Errorf("having: %w", err)
		}
		doc.Having = c
	}
	for i, o := range q.OrderBy() {
		expr, err := exprToYAML(o.Expression())
		if err != nil {
			return document{}, fmt.Errorf("orderBy #%d: %w", i, err)
		}
		doc.OrderBy = append(doc.OrderBy, orderYAML{exprYAML: expr, Desc: o.Descending()})
	}
	return doc, nil
}

func fromToYAML(from dal.FromSource) (fromYAML, error) {
	base, ok := from.Base().(dal.CollectionRef)
	if !ok {
		return fromYAML{}, fmt.Errorf("unsupported From source %T (only root collection references are supported)", from.Base())
	}
	if base.Parent() != nil {
		return fromYAML{}, fmt.Errorf("parented collection reference %q is not supported by DTQL (only root collections)", base.Path())
	}
	result := fromYAML{Name: base.Name(), Alias: base.Alias()}
	if schema := base.Schema(); schema != "" {
		result.Schema = &schema
	}
	for i, join := range from.Joins() {
		child := join.From()
		if child == nil {
			child = dal.From(join.RecordsetSource)
		}
		childDoc, err := fromToYAML(child)
		if err != nil {
			return fromYAML{}, fmt.Errorf("from.joins[%d].from: %w", i, err)
		}
		on := make([]condYAML, 0, len(join.On()))
		for _, condition := range join.On() {
			encoded, _ := condToYAML(condition) // ValidateJoinTree checked every ON shape and operator.
			on = append(on, *encoded)
		}
		joinDoc := joinYAML{From: &childDoc, On: on, Hints: algorithmsToYAML(join.Algorithms())}
		if join.JoinType() == dal.JoinLeft {
			joinDoc.Type = "left"
		}
		result.Joins = append(result.Joins, joinDoc)
	}
	return result, nil
}

func exprToYAML(expr dal.Expression) (exprYAML, error) {
	switch e := expr.(type) {
	case dal.FieldRef:
		return exprYAML{Field: e.Name(), Source: e.Source()}, nil
	case dal.Constant:
		if !portableScalar(e.Value) {
			return exprYAML{}, fmt.Errorf("unsupported constant value %T", e.Value)
		}
		value := e.Value
		return exprYAML{Value: &value}, nil
	case dal.Array:
		if !portableScalarArray(e.Value) {
			return exprYAML{}, fmt.Errorf("unsupported array value %T", e.Value)
		}
		return exprYAML{Values: e.Value}, nil
	case dal.Param:
		return exprYAML{Param: e.Name}, nil
	case dal.StarExpression:
		return exprYAML{Star: true}, nil
	case dal.AggregateFunc:
		args := make([]exprYAML, len(e.FuncArgs()))
		for i, arg := range e.FuncArgs() {
			encoded, err := exprToYAML(arg)
			if err != nil {
				return exprYAML{}, fmt.Errorf("aggregate argument #%d: %w", i, err)
			}
			args[i] = encoded
		}
		distinct := false
		if d, ok := e.(dal.DistinctAggregateFunc); ok {
			distinct = d.IsDistinct()
		}
		return exprYAML{Aggregate: &aggregateYAML{Function: strings.ToLower(e.FuncName()), Distinct: distinct, Args: args}}, nil
	case dal.BinaryExpression:
		left, err := exprToYAML(e.Left)
		if err != nil {
			return exprYAML{}, fmt.Errorf("binary left: %w", err)
		}
		right, err := exprToYAML(e.Right)
		if err != nil {
			return exprYAML{}, fmt.Errorf("binary right: %w", err)
		}
		return exprYAML{Binary: &binaryYAML{Op: string(e.Operator), Left: &left, Right: &right}}, nil
	default:
		return exprYAML{}, fmt.Errorf("unsupported expression %T", expr)
	}
}

func portableScalar(value any) bool {
	if value == nil {
		return true
	}
	switch value := value.(type) {
	case string, bool:
		return true
	case float32:
		return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
	case float64:
		return !math.IsNaN(value) && !math.IsInf(value, 0)
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v := rv.Int()
		return v >= -(1<<53)+1 && v <= (1<<53)-1
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint() <= (1<<53)-1
	default:
		return false
	}
}

func portableScalarArray(value any) bool {
	if value == nil {
		return false
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return false
	}
	for i := 0; i < rv.Len(); i++ {
		if !portableScalar(rv.Index(i).Interface()) {
			return false
		}
	}
	return true
}

func condToYAML(cond dal.Condition) (*condYAML, error) {
	switch c := cond.(type) {
	case dal.Comparison:
		return comparisonToYAML(c)
	case dal.GroupCondition:
		return groupToYAML(c)
	default:
		return nil, fmt.Errorf("unsupported condition %T", cond)
	}
}

func comparisonToYAML(c dal.Comparison) (*condYAML, error) {
	if !inScopeComparisonOps[c.Operator] {
		return nil, fmt.Errorf("unsupported comparison operator %q", c.Operator)
	}
	left, err := exprToYAML(c.Left)
	if err != nil {
		return nil, fmt.Errorf("comparison left: %w", err)
	}
	right, err := exprToYAML(c.Right)
	if err != nil {
		return nil, fmt.Errorf("comparison right: %w", err)
	}
	return &condYAML{Op: string(c.Operator), Left: &left, Right: &right}, nil
}

func groupToYAML(g dal.GroupCondition) (*condYAML, error) {
	children := make([]condYAML, 0, len(g.Conditions()))
	for i, sub := range g.Conditions() {
		c, err := condToYAML(sub)
		if err != nil {
			return nil, fmt.Errorf("group condition #%d: %w", i, err)
		}
		children = append(children, *c)
	}
	switch g.Operator() {
	case dal.And:
		return &condYAML{And: children}, nil
	case dal.Or:
		return &condYAML{Or: children}, nil
	default:
		return nil, fmt.Errorf("unsupported group operator %q", g.Operator())
	}
}
