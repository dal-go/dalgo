package dal

import (
	"fmt"
	"reflect"
)

// maxQueryTreeDepth bounds how many nodes (queries, source trees, conditions and
// expressions) the walk of a query tree follows along one path. It is the bound
// the access layer applies to a query's structure.
const maxQueryTreeDepth = 64

// inspectQueryTree reports whether found accepts a query nested anywhere in q:
// a derived source, a subquery in any clause (the select list, WHERE, ON,
// GROUP BY, HAVING, ORDER BY, an aggregate argument, an arithmetic operand or a
// null test) or a subquery in the scan order of a source. A node is read the
// same whether it is held by value or by non-nil pointer. A node that is on the
// path being walked is not walked again, so a graph of pointers that holds itself
// ends, and so does one held by value, which the walk stops following at
// maxQueryTreeDepth nodes along one path: there it reports a query, so that a
// tree the walk cannot follow to its end is handled as one with nested queries.
func inspectQueryTree(q StructuredQuery, found func(StructuredQuery) bool) bool {
	seen := map[uintptr]bool{}
	depth := 0
	// enter admits a node to the current path and returns the function that leaves
	// it. It returns nil when the node must not be walked, and says whether the
	// walk reports a query instead: when the path is already maxQueryTreeDepth
	// nodes long it does, and when a reference-typed node, given by its address, is
	// already on the path it does not. A node held by value has no address (0).
	enter := func(id uintptr) (leave func(), report bool) {
		if depth >= maxQueryTreeDepth {
			return nil, true
		}
		if id != 0 {
			if seen[id] {
				return nil, false
			}
			seen[id] = true
		}
		depth++
		return func() {
			depth--
			delete(seen, id)
		}, false
	}
	var visitQuery func(StructuredQuery) bool
	var visitExpr func(Expression) bool
	var visitCondition func(Condition) bool
	var visitFrom func(FromSource) bool
	visitExpr = func(expr Expression) bool {
		leave, report := enter(nodePointerID(expr))
		if leave == nil {
			return report
		}
		defer leave()
		switch value := expr.(type) {
		case QueryExpression:
			return found(value.Query()) || visitQuery(value.Query())
		case *QueryExpression:
			return value != nil && visitExpr(*value)
		case BinaryExpression:
			return visitExpr(value.Left) || visitExpr(value.Right)
		case *BinaryExpression:
			return value != nil && visitExpr(*value)
		case AggregateFunc:
			for _, arg := range value.FuncArgs() {
				if visitExpr(arg) {
					return true
				}
			}
		}
		return false
	}
	visitCondition = func(condition Condition) bool {
		leave, report := enter(nodePointerID(condition))
		if leave == nil {
			return report
		}
		defer leave()
		switch value := condition.(type) {
		case ExistsCondition:
			return found(value.Query()) || visitQuery(value.Query())
		case *ExistsCondition:
			return value != nil && visitCondition(*value)
		case IsNullCondition:
			return visitExpr(value.Operand())
		case *IsNullCondition:
			return value != nil && visitCondition(*value)
		case Comparison:
			return visitExpr(value.Left) || visitExpr(value.Right)
		case *Comparison:
			return value != nil && visitCondition(*value)
		case GroupCondition:
			for _, child := range value.Conditions() {
				if visitCondition(child) {
					return true
				}
			}
		case *GroupCondition:
			return value != nil && visitCondition(*value)
		}
		return false
	}
	// visitSource looks at a source on its own: a derived source is a query, and
	// a collection's scan orders are expressions that may hold one.
	visitSource := func(source RecordsetSource) bool {
		if source == nil {
			return false
		}
		if derived, ok := asQuerySource(source); ok {
			return found(derived.Query()) || visitQuery(derived.Query())
		}
		for _, order := range collectionScanOrders(source) {
			if order != nil && visitExpr(order.Expression()) {
				return true
			}
		}
		return false
	}
	visitFrom = func(from FromSource) bool {
		if from == nil || from.Base() == nil {
			return false
		}
		leave, report := enter(nodePointerID(from))
		if leave == nil {
			return report
		}
		defer leave()
		if visitSource(from.Base()) {
			return true
		}
		for _, join := range from.Joins() {
			child := join.From()
			if child == nil && join.RecordsetSource != nil {
				child = From(join.RecordsetSource)
			}
			if visitFrom(child) {
				return true
			}
			// A join also names its source directly. When that differs from the
			// base of its tree, it is looked at too.
			if tree := join.From(); tree != nil && tree.Base() != nil && !reflect.DeepEqual(join.RecordsetSource, tree.Base()) && visitSource(join.RecordsetSource) {
				return true
			}
			for _, on := range join.On() {
				if visitCondition(on) {
					return true
				}
			}
		}
		return false
	}
	visitQuery = func(query StructuredQuery) bool {
		if query == nil {
			return false
		}
		leave, report := enter(queryPointerID(query))
		if leave == nil {
			return report
		}
		defer leave()
		if visitFrom(query.From()) || visitCondition(query.Where()) || visitCondition(query.Having()) {
			return true
		}
		for _, column := range query.Columns() {
			if visitExpr(column.Expression) {
				return true
			}
		}
		for _, expression := range query.GroupBy() {
			if visitExpr(expression) {
				return true
			}
		}
		for _, order := range query.OrderBy() {
			if order != nil && visitExpr(order.Expression()) {
				return true
			}
		}
		return false
	}
	return visitQuery(q)
}

// collectionScanOrders lists the scan orders of a source that is a collection,
// given by value or by non-nil pointer.
func collectionScanOrders(source RecordsetSource) []OrderExpression {
	switch source := source.(type) {
	case CollectionRef:
		return source.ScanOrders()
	case *CollectionRef:
		if source != nil {
			return source.ScanOrders()
		}
	}
	return nil
}

func validateQueryScope(q StructuredQuery, outer map[string]bool, path string, visiting map[uintptr]bool) error {
	if q == nil {
		return queryValidationError("query_shape", path, "query is required")
	}
	if id := queryPointerID(q); id != 0 {
		if visiting[id] {
			return queryValidationError("query_cycle", path, "recursive query reference")
		}
		visiting[id] = true
		defer delete(visiting, id)
	}
	if q.From() == nil || q.From().Base() == nil {
		return queryValidationError("query_shape", joinPath(path, "from"), "from is required")
	}
	visible := cloneQueryAliases(outer)
	local := map[string]bool{}
	if err := validateFromScope(q.From(), visible, local, joinPath(path, "from"), visiting); err != nil {
		return err
	}
	for alias := range local {
		visible[alias] = true
	}
	if err := validateConditionScope(q.Where(), visible, joinPath(path, "where"), visiting); err != nil {
		return err
	}
	if err := validateConditionScope(q.Having(), visible, joinPath(path, "having"), visiting); err != nil {
		return err
	}
	for i, column := range q.Columns() {
		if column.Wildcard != nil && column.Wildcard.Source != "" && !visible[column.Wildcard.Source] {
			return queryValidationError("query_scope", fmt.Sprintf("%s[%d].source", joinPath(path, "columns"), i), fmt.Sprintf("unknown alias %q", column.Wildcard.Source))
		}
		if err := validateExpressionScope(column.Expression, visible, fmt.Sprintf("%s[%d]", joinPath(path, "columns"), i), visiting); err != nil {
			return err
		}
	}
	for i, expression := range q.GroupBy() {
		if err := validateExpressionScope(expression, visible, fmt.Sprintf("%s[%d]", joinPath(path, "groupBy"), i), visiting); err != nil {
			return err
		}
	}
	for i, order := range q.OrderBy() {
		if err := validateExpressionScope(order.Expression(), visible, fmt.Sprintf("%s[%d]", joinPath(path, "orderBy"), i), visiting); err != nil {
			return err
		}
	}
	return nil
}

func validateFromScope(from FromSource, outer, local map[string]bool, path string, visiting map[uintptr]bool) error {
	base := from.Base()
	if source, ok := base.(*QuerySource); ok && source == nil {
		return queryValidationError("query_shape", path+".query", "query is required")
	}
	alias := base.Alias()
	if alias == "" {
		alias = base.Name()
	}
	if alias == "" {
		return queryValidationError("query_shape", path+".name", "source name is required")
	}
	if local[alias] {
		return queryValidationError("query_scope", path+".alias", fmt.Sprintf("duplicate alias %q", alias))
	}
	if source, ok := asQuerySource(base); ok {
		if source.Query() == nil {
			return queryValidationError("query_shape", path+".query", "query is required")
		}
		if err := validateQueryScope(source.Query(), outer, path+".query", visiting); err != nil {
			return err
		}
	}
	local[alias] = true
	visible := cloneQueryAliases(outer)
	for name := range local {
		visible[name] = true
	}
	for i, join := range from.Joins() {
		joinPath := fmt.Sprintf("%s.joins[%d]", path, i)
		child := join.From()
		if child == nil && join.RecordsetSource != nil {
			child = From(join.RecordsetSource)
		}
		if child == nil {
			return queryValidationError("query_shape", joinPath+".from", "from is required")
		}
		childLocal := map[string]bool{}
		if err := validateFromScope(child, visible, childLocal, joinPath+".from", visiting); err != nil {
			return err
		}
		childVisible := cloneQueryAliases(visible)
		for name := range childLocal {
			childVisible[name] = true
		}
		for j, condition := range join.On() {
			if err := validateConditionScope(condition, childVisible, fmt.Sprintf("%s.on[%d]", joinPath, j), visiting); err != nil {
				return err
			}
		}
		for name := range childLocal {
			if local[name] {
				return queryValidationError("query_scope", joinPath+".from.alias", fmt.Sprintf("duplicate alias %q", name))
			}
			local[name] = true
		}
		visible = cloneQueryAliases(outer)
		for name := range local {
			visible[name] = true
		}
	}
	return nil
}

func validateConditionScope(condition Condition, visible map[string]bool, path string, visiting map[uintptr]bool) error {
	if condition == nil {
		return nil
	}
	switch value := condition.(type) {
	case ExistsCondition:
		return validateQueryScope(value.Query(), visible, path+".query", visiting)
	case IsNullCondition:
		return validateExpressionScope(value.Operand(), visible, path+".operand", visiting)
	case Comparison:
		if err := validateExpressionScope(value.Left, visible, path+".left", visiting); err != nil {
			return err
		}
		return validateExpressionScope(value.Right, visible, path+".right", visiting)
	case GroupCondition:
		for i, child := range value.Conditions() {
			if err := validateConditionScope(child, visible, fmt.Sprintf("%s.conditions[%d]", path, i), visiting); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateExpressionScope(expression Expression, visible map[string]bool, path string, visiting map[uintptr]bool) error {
	if expression == nil {
		return nil
	}
	switch value := expression.(type) {
	case FieldRef:
		if value.Source() != "" && !visible[value.Source()] {
			return queryValidationError("query_scope", path+".source", fmt.Sprintf("unknown alias %q", value.Source()))
		}
	case QueryExpression:
		return validateQueryScope(value.Query(), visible, path+".query", visiting)
	case BinaryExpression:
		if err := validateExpressionScope(value.Left, visible, path+".left", visiting); err != nil {
			return err
		}
		return validateExpressionScope(value.Right, visible, path+".right", visiting)
	case AggregateFunc:
		for i, arg := range value.FuncArgs() {
			if err := validateExpressionScope(arg, visible, fmt.Sprintf("%s.args[%d]", path, i), visiting); err != nil {
				return err
			}
		}
	}
	return nil
}

func queryPointerID(q StructuredQuery) uintptr {
	v := reflect.ValueOf(q)
	if !v.IsValid() {
		return 0
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func:
		if !v.IsNil() {
			return v.Pointer()
		}
	}
	return 0
}

// nodePointerID returns the address a pointer node refers to, or 0 for a node
// held by value, which cannot be reached again from inside itself.
func nodePointerID(node any) uintptr {
	if v := reflect.ValueOf(node); v.IsValid() && v.Kind() == reflect.Pointer && !v.IsNil() {
		return v.Pointer()
	}
	return 0
}

func cloneQueryAliases(source map[string]bool) map[string]bool {
	out := map[string]bool{}
	for name := range source {
		out[name] = true
	}
	return out
}
func queryValidationError(category, path, message string) error {
	return &QueryValidationError{Category: category, Path: path, Message: message}
}
func joinPath(path, suffix string) string {
	if path == "" {
		return suffix
	}
	return path + "." + suffix
}
