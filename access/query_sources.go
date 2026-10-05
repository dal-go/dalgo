package access

import (
	"fmt"
	"reflect"

	"github.com/dal-go/dalgo/dal"
)

// maxQueryNesting bounds how deeply the access layer follows a query's
// structure: each nested query, source tree, condition or expression adds one
// level. A query nested deeper than this is not analysed. Listing its sources
// treats it as an opaque resource, and checking its expressions against a field
// list refuses it.
const maxQueryNesting = 64

// resourcesForQuery lists the resource of every source a query reads, so a
// policy authorises each of them before any is read. The base source comes
// first; after it come, in the order they are met, every join at any depth,
// every source inside a derived source's query, and every source of a subquery
// in any clause. A non-structured query, and any part of a structured query
// that cannot be analysed (an unrecognised node, a cycle, nesting past
// maxQueryNesting), is an opaque resource, which a policy denies unless it
// explicitly allows opaque queries. The list always has at least one entry.
func resourcesForQuery(query dal.Query) []Resource {
	structured, ok := query.(dal.StructuredQuery)
	if !ok {
		return []Resource{OpaqueQuery(query.String())}
	}
	walk := querySourceWalk{onPath: map[uintptr]bool{}}
	walk.query(structured, 0)
	return walk.resources
}

// querySourceWalk collects the resources of a query tree. onPath holds the
// reference-typed nodes being walked, so a node reached again from inside
// itself is recognised as a cycle.
type querySourceWalk struct {
	resources []Resource
	onPath    map[uintptr]bool
}

func (w *querySourceWalk) unknown(description string) {
	w.resources = append(w.resources, OpaqueQuery(description))
}

// enter admits a query, a source tree or a node to the walk. It returns the
// function that leaves it, or nil when it must not be walked: it is missing
// (a query or a source tree is required where it appears), nested past the
// depth bound, or already on the current path. Each of those is an unknown
// shape and is recorded as an opaque resource.
func (w *querySourceWalk) enter(node any, depth int) func() {
	if isNilNode(node) {
		w.unknown("missing query or source tree")
		return nil
	}
	if depth > maxQueryNesting {
		w.unknown("query nested too deeply")
		return nil
	}
	id := nodeIdentity(node)
	if id != 0 {
		if w.onPath[id] {
			w.unknown("query refers to itself")
			return nil
		}
		w.onPath[id] = true
	}
	return func() { delete(w.onPath, id) }
}

func (w *querySourceWalk) query(query dal.StructuredQuery, depth int) {
	leave := w.enter(query, depth)
	if leave == nil {
		return
	}
	defer leave()
	w.from(query.From(), depth+1)
	for _, column := range query.Columns() {
		w.node(column.Expression, depth+1)
	}
	w.node(query.Where(), depth+1)
	for _, expression := range query.GroupBy() {
		w.node(expression, depth+1)
	}
	w.node(query.Having(), depth+1)
	for _, order := range query.OrderBy() {
		w.order(order, depth+1)
	}
}

func (w *querySourceWalk) from(from dal.FromSource, depth int) {
	leave := w.enter(from, depth)
	if leave == nil {
		return
	}
	defer leave()
	w.source(from.Base(), depth+1)
	for _, join := range from.Joins() {
		tree := join.From()
		if !isNilNode(tree) {
			w.from(tree, depth+1)
		}
		// A join also names its source directly, and an adapter may read that
		// instead of the tree. Both are authorised when they differ.
		if isNilNode(tree) || !reflect.DeepEqual(join.RecordsetSource, tree.Base()) {
			w.source(join.RecordsetSource, depth+1)
		}
		for _, on := range join.On() {
			w.node(on, depth+1)
		}
	}
}

// source records the resource of a stored source, or walks the query of a
// derived source. A derived source is not itself a stored source: it reads
// only what its query reads.
func (w *querySourceWalk) source(source dal.RecordsetSource, depth int) {
	switch source := source.(type) {
	case dal.QuerySource:
		w.query(source.Query(), depth+1)
		return
	case *dal.QuerySource:
		if source == nil {
			w.unknown("missing derived source")
			return
		}
		w.query(source.Query(), depth+1)
		return
	}
	w.resources = append(w.resources, resourceForRecordsetSource(source))
	for _, order := range scanOrders(source) {
		w.order(order, depth+1)
	}
}

func scanOrders(source dal.RecordsetSource) []dal.OrderExpression {
	switch source := source.(type) {
	case dal.CollectionRef:
		return source.ScanOrders()
	case *dal.CollectionRef:
		if source != nil {
			return source.ScanOrders()
		}
	}
	return nil
}

func (w *querySourceWalk) order(order dal.OrderExpression, depth int) {
	if isNilNode(order) {
		return
	}
	w.node(order.Expression(), depth)
}

// node walks an expression or a condition. The two share one method set, and a
// node of one kind can stand where the other is expected, so one walk serves
// both. A missing node holds no source and a node that holds no query is a
// leaf; a node of a type the walk does not know is an unknown shape.
func (w *querySourceWalk) node(node fmt.Stringer, depth int) {
	if isNilNode(node) {
		return
	}
	leave := w.enter(node, depth)
	if leave == nil {
		return
	}
	defer leave()
	switch node := node.(type) {
	case dal.FieldRef, *dal.FieldRef, dal.FieldName, dal.Constant, *dal.Constant, dal.Param, *dal.Param, dal.Array, *dal.Array, dal.StarExpression:
	case dal.AggregateFunc:
		for _, argument := range node.FuncArgs() {
			w.node(argument, depth+1)
		}
	case dal.BinaryExpression:
		w.node(node.Left, depth+1)
		w.node(node.Right, depth+1)
	case *dal.BinaryExpression:
		w.node(*node, depth+1)
	case dal.QueryExpression:
		w.query(node.Query(), depth+1)
	case *dal.QueryExpression:
		w.node(*node, depth+1)
	case dal.ExistsCondition:
		w.query(node.Query(), depth+1)
	case *dal.ExistsCondition:
		w.node(*node, depth+1)
	case dal.IsNullCondition:
		w.node(node.Operand(), depth+1)
	case *dal.IsNullCondition:
		w.node(*node, depth+1)
	case dal.Comparison:
		w.node(node.Left, depth+1)
		w.node(node.Right, depth+1)
	case *dal.Comparison:
		w.node(*node, depth+1)
	case dal.GroupCondition:
		for _, child := range node.Conditions() {
			w.node(child, depth+1)
		}
	case *dal.GroupCondition:
		w.node(*node, depth+1)
	default:
		w.unknown(fmt.Sprintf("unrecognised query node %T", node))
	}
}

// isNilNode reports whether node is nil, including a nil pointer, map, slice or
// function held in an interface.
func isNilNode(node any) bool {
	if node == nil {
		return true
	}
	value := reflect.ValueOf(node)
	switch value.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func:
		return value.IsNil()
	}
	return false
}

// nodeIdentity returns the address a pointer node refers to, or 0 for a value
// node, which cannot be reached again from inside itself.
func nodeIdentity(node any) uintptr {
	if value := reflect.ValueOf(node); value.Kind() == reflect.Pointer {
		return value.Pointer()
	}
	return 0
}
