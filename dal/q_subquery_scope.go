package dal

import (
	"fmt"
	"reflect"
)

func inspectQueryTree(q StructuredQuery, found func(StructuredQuery) bool) bool {
	seen := map[uintptr]bool{}
	var visitQuery func(StructuredQuery) bool
	visitQuery = func(query StructuredQuery) bool {
		if query == nil {
			return false
		}
		id := queryPointerID(query)
		if id != 0 {
			if seen[id] {
				return false
			}
			seen[id] = true
			defer delete(seen, id)
		}
		var visitExpr func(Expression) bool
		var visitCondition func(Condition) bool
		visitExpr = func(expr Expression) bool {
			switch value := expr.(type) {
			case QueryExpression:
				return found(value.Query()) || visitQuery(value.Query())
			case BinaryExpression:
				return visitExpr(value.Left) || visitExpr(value.Right)
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
			switch value := condition.(type) {
			case ExistsCondition:
				return found(value.Query()) || visitQuery(value.Query())
			case Comparison:
				return visitExpr(value.Left) || visitExpr(value.Right)
			case GroupCondition:
				for _, child := range value.Conditions() {
					if visitCondition(child) {
						return true
					}
				}
			}
			return false
		}
		var visitFrom func(FromSource) bool
		visitFrom = func(from FromSource) bool {
			if from == nil || from.Base() == nil {
				return false
			}
			if source, ok := asQuerySource(from.Base()); ok {
				return found(source.Query()) || visitQuery(source.Query())
			}
			for _, join := range from.Joins() {
				child := join.From()
				if child == nil && join.RecordsetSource != nil {
					child = From(join.RecordsetSource)
				}
				if visitFrom(child) {
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
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Func:
		if !v.IsNil() {
			return v.Pointer()
		}
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
