package dtql

import (
	"fmt"

	"github.com/dal-go/dalgo/dal"
	"gopkg.in/yaml.v3"
)

// Serialize converts an in-scope dal.StructuredQuery into a canonical DTQL-YAML
// document. It first gates the query through checkInScope: an out-of-scope query
// yields a descriptive error and no document.
func Serialize(q dal.StructuredQuery) ([]byte, error) {
	if err := checkInScope(q); err != nil {
		return nil, fmt.Errorf("query is not representable as DTQL: %w", err)
	}
	doc, err := queryToDocument(q)
	if err != nil {
		return nil, err
	}
	return marshalCanonical(doc)
}

// marshalCanonical marshals a document with stable 2-space indentation. Struct
// field order is fixed by the shape types, so the output is canonical.
func marshalCanonical(doc document) ([]byte, error) {
	var buf yamlBuffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("failed to marshal DTQL document: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize DTQL document: %w", err)
	}
	return buf.b, nil
}

type yamlBuffer struct{ b []byte }

func (w *yamlBuffer) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}

func queryToDocument(q dal.StructuredQuery) (document, error) {
	from, err := fromToYAML(q.From())
	if err != nil {
		return document{}, err
	}
	doc := document{
		From:   from,
		Limit:  q.Limit(),
		Offset: q.Offset(),
	}
	for _, col := range q.Columns() {
		expr, err := exprToYAML(col.Expression)
		if err != nil {
			return document{}, err
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
	for _, o := range q.OrderBy() {
		expr, err := exprToYAML(o.Expression())
		if err != nil {
			return document{}, err
		}
		doc.OrderBy = append(doc.OrderBy, orderYAML{exprYAML: expr, Desc: o.Descending()})
	}
	return doc, nil
}

func fromToYAML(from dal.FromSource) (fromYAML, error) {
	switch base := from.Base().(type) {
	case dal.CollectionRef:
		return fromYAML{Name: base.Name(), Alias: base.Alias()}, nil
	case *dal.CollectionRef:
		return fromYAML{Name: base.Name(), Alias: base.Alias()}, nil
	default:
		return fromYAML{}, fmt.Errorf("unsupported From source %T", base)
	}
}

func exprToYAML(expr dal.Expression) (exprYAML, error) {
	switch e := expr.(type) {
	case dal.FieldRef:
		return exprYAML{Field: e.Name()}, nil
	case *dal.FieldRef:
		return exprYAML{Field: e.Name()}, nil
	case dal.Constant:
		return exprYAML{Value: encodeNode(e.Value)}, nil
	case *dal.Constant:
		return exprYAML{Value: encodeNode(e.Value)}, nil
	case dal.Array:
		return exprYAML{Values: encodeNode(e.Value)}, nil
	case *dal.Array:
		return exprYAML{Values: encodeNode(e.Value)}, nil
	default:
		return exprYAML{}, fmt.Errorf("unsupported expression %T", expr)
	}
}

func encodeNode(v any) *yaml.Node {
	node := &yaml.Node{}
	// Encode never fails for the scalar/slice value types DTQL admits.
	_ = node.Encode(v)
	return node
}

func condToYAML(cond dal.Condition) (*condYAML, error) {
	switch c := cond.(type) {
	case dal.Comparison:
		return comparisonToYAML(c)
	case *dal.Comparison:
		return comparisonToYAML(*c)
	case dal.GroupCondition:
		return groupToYAML(c)
	case *dal.GroupCondition:
		return groupToYAML(*c)
	default:
		return nil, fmt.Errorf("unsupported condition %T", cond)
	}
}

func comparisonToYAML(c dal.Comparison) (*condYAML, error) {
	left, err := exprToYAML(c.Left)
	if err != nil {
		return nil, err
	}
	right, err := exprToYAML(c.Right)
	if err != nil {
		return nil, err
	}
	return &condYAML{Op: string(c.Operator), Left: &left, Right: &right}, nil
}

func groupToYAML(g dal.GroupCondition) (*condYAML, error) {
	children := make([]condYAML, 0, len(g.Conditions()))
	for _, sub := range g.Conditions() {
		c, err := condToYAML(sub)
		if err != nil {
			return nil, err
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
