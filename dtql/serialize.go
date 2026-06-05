package dtql

import (
	"fmt"

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
// (joins, GroupBy, cursor, a non-root source, an unsupported expression,
// condition or operator) with a descriptive error rather than dropping it.
func queryToDocument(q dal.StructuredQuery) (document, error) {
	from := q.From()
	if from == nil {
		return document{}, fmt.Errorf("query has no From source")
	}
	if len(from.Joins()) > 0 {
		return document{}, fmt.Errorf("joins are not supported by DTQL")
	}
	if len(q.GroupBy()) > 0 {
		return document{}, fmt.Errorf("GroupBy is not supported by DTQL")
	}
	if q.StartFrom() != "" {
		return document{}, fmt.Errorf("cursor (StartFrom) is not supported by DTQL")
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
	return fromYAML{Name: base.Name(), Alias: base.Alias()}, nil
}

func exprToYAML(expr dal.Expression) (exprYAML, error) {
	switch e := expr.(type) {
	case dal.FieldRef:
		return exprYAML{Field: e.Name()}, nil
	case dal.Constant:
		return exprYAML{Value: e.Value}, nil
	case dal.Array:
		return exprYAML{Values: e.Value}, nil
	default:
		return exprYAML{}, fmt.Errorf("unsupported expression %T (only field references, constants and arrays are supported)", expr)
	}
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
