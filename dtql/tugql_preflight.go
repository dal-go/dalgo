package dtql

import "fmt"

// The typed parser tree is smaller than its exported YAML/JSON map form, but
// may be larger for leaf metadata such as aliases. Bound the parser-owned walk
// conservatively at four times the public semantic-node budget, then apply the
// exact 5,000-node budget to the canonical exported tree. This prevents an
// attacker-controlled wide AST from being recursively exported without
// rejecting ordinary canonical trees that fit the public limit.
const maxTugQLPreflightNodes = 4 * maxTugQLSemanticNodes

type tugqlPreflightNode struct {
	value            any
	depth            int
	selected         bool
	bodyQueryIsChild bool
	nestedDefinition bool
}

// preflightTugQLTree bounds the parser-owned semantic tree before export can
// recursively walk it. Its depth edges mirror the canonical tree: query map,
// projection list, column map, binary map, then left/right expression maps.
func preflightTugQLTree(tree tugqlTree) []tugqlDiagnostic {
	return preflightTugQLTreeWithSelection(tree, true)
}

func preflightTugQLExpandedTree(tree tugqlTree) []tugqlDiagnostic {
	return preflightTugQLTreeWithSelection(tree, false)
}

func preflightTugQLTreeWithSelection(tree tugqlTree, selected bool) []tugqlDiagnostic {
	stack := make([]tugqlPreflightNode, 0, 32)
	limitError := ""
	visited := 0
	var rootQuerySpan, rootSelectSpan tugqlSpan
	if tree.Query.tugqlQuerySpan.Start.Line > 0 {
		rootQuerySpan = tree.Query.tugqlQuerySpan
	} else {
		rootQuerySpan = tugqlSpan{Start: tugqlPosition{1, 1}, End: tugqlPosition{1, 1}}
	}
	if tree.Query.tugqlSelectSpan.Start.Line > 0 {
		rootSelectSpan = tree.Query.tugqlSelectSpan
	} else {
		rootSelectSpan = rootQuerySpan
	}
	pushNode := func(value any, depth int, selected, bodyQueryIsChild, nestedDefinition bool) bool {
		if depth > maxTugQLStructuralDepth {
			limitError = "depth"
			return false
		}
		if visited+len(stack) >= maxTugQLPreflightNodes {
			limitError = "nodes"
			return false
		}
		stack = append(stack, tugqlPreflightNode{
			value:            value,
			depth:            depth,
			selected:         selected,
			bodyQueryIsChild: bodyQueryIsChild,
			nestedDefinition: nestedDefinition,
		})
		return true
	}
	push := func(value any, depth int, selected bool) bool {
		return pushNode(value, depth, selected, false, false)
	}
	pushBody := func(value tugqlBody, depth int, selected, queryIsChild bool) bool {
		return pushNode(value, depth, selected, queryIsChild, false)
	}
	pushDefinition := func(value tugqlDefinition, depth int, selected, nested bool) bool {
		return pushNode(value, depth, selected, false, nested)
	}
	push(tree.Query, 0, selected)
	for _, parameter := range tree.Parameters {
		if !push(parameter, 0, false) {
			return []tugqlDiagnostic{diagnostic("document_node_limit", fmt.Sprintf("TugQL semantic tree exceeds %d nodes", maxTugQLSemanticNodes), rootQuerySpan)}
		}
	}
	for _, definition := range tree.Definitions {
		if !push(definition, 0, false) {
			return []tugqlDiagnostic{diagnostic("document_node_limit", fmt.Sprintf("TugQL semantic tree exceeds %d nodes", maxTugQLSemanticNodes), rootQuerySpan)}
		}
	}
	for len(stack) > 0 {
		index := len(stack) - 1
		item := stack[index]
		stack = stack[:index]
		visited++
		childDepth := item.depth + 1
		add := func(value any, depth int, selected bool) bool { return push(value, depth, selected) }
		ok := true
		switch value := item.value.(type) {
		case document:
			// Root-level query clauses are independent of its SELECT projection.
			// A document nested inside a selected scalar expression keeps that
			// context through all of its clauses and local definitions.
			selectedBody := item.selected && item.depth > 0
			ok = add(value.From, childDepth, selectedBody)
			if ok && value.Where != nil {
				ok = add(*value.Where, childDepth, selectedBody)
			}
			if ok && value.Having != nil {
				ok = add(*value.Having, childDepth, selectedBody)
			}
			if ok && len(value.GroupBy) > 0 {
				ok = add(value.GroupBy, childDepth, selectedBody)
			}
			if ok && len(value.OrderBy) > 0 {
				ok = add(value.OrderBy, childDepth, selectedBody)
			}
			if ok && len(value.Columns) > 0 {
				ok = add(value.Columns, childDepth, item.selected)
			}
		case fromYAML:
			if value.Query != nil {
				ok = add(*value.Query, childDepth, item.selected)
			}
			if ok && len(value.Joins) > 0 {
				ok = add(value.Joins, childDepth, false)
			}
		case joinYAML:
			if value.From != nil {
				ok = add(*value.From, childDepth, false)
			}
			if ok && len(value.On) > 0 {
				ok = add(value.On, childDepth, false)
			}
		case []joinYAML:
			for _, child := range value {
				if !add(child, childDepth, item.selected) {
					ok = false
					break
				}
			}
		case []columnYAML:
			for _, child := range value {
				if !add(child, childDepth, item.selected) {
					ok = false
					break
				}
			}
		case columnYAML:
			if value.exprYAML.Binary != nil || value.exprYAML.Aggregate != nil || value.exprYAML.Query != nil || value.exprYAML.tugqlQueryBody != nil || value.exprYAML.tugqlCall != nil || value.exprYAML.Value != nil || value.exprYAML.Values != nil || value.exprYAML.Param != "" || value.exprYAML.Field != "" || value.exprYAML.Star {
				// A projection item and its expression share one mapping node in
				// the canonical wire shape.
				ok = add(value.exprYAML, item.depth, item.selected)
			}
			if value.Wildcard != nil && ok {
				ok = add(*value.Wildcard, childDepth, item.selected)
			}
		case []exprYAML:
			for _, child := range value {
				if !add(child, childDepth, item.selected) {
					ok = false
					break
				}
			}
		case []orderYAML:
			for _, child := range value {
				if !add(child, childDepth, item.selected) {
					ok = false
					break
				}
			}
		case orderYAML:
			// orderYAML embeds exprYAML in the same canonical mapping.
			ok = add(value.exprYAML, item.depth, item.selected)
		case exprYAML:
			if value.Binary != nil {
				ok = add(*value.Binary, childDepth, item.selected)
			}
			if ok && value.Aggregate != nil {
				ok = add(*value.Aggregate, childDepth, item.selected)
			}
			if ok && value.tugqlQueryBody != nil {
				ok = pushBody(*value.tugqlQueryBody, childDepth, item.selected, true)
			} else if ok && value.Query != nil {
				ok = add(*value.Query, childDepth, item.selected)
			}
			if ok && value.tugqlCall != nil {
				ok = add(*value.tugqlCall, childDepth, item.selected)
			}
			if ok && value.Binary == nil && value.Aggregate == nil && value.Query == nil && value.tugqlQueryBody == nil && value.tugqlCall == nil {
				if value.Field != "" || value.Value != nil || value.Values != nil || value.Param != "" || value.Star {
					ok = add(struct{}{}, childDepth, item.selected)
				}
				if ok && value.Source != "" {
					ok = add(struct{}{}, childDepth, item.selected)
				}
			}
		case binaryYAML:
			if value.Left != nil {
				ok = add(*value.Left, childDepth, item.selected)
			}
			if ok && value.Right != nil {
				ok = add(*value.Right, childDepth, item.selected)
			}
		case aggregateYAML:
			if len(value.Args) > 0 {
				ok = add(value.Args, childDepth, item.selected)
			}
			if ok && len(value.OrderBy) > 0 {
				ok = add(value.OrderBy, childDepth, item.selected)
			}
		case tugqlCallYAML:
			if len(value.Args) > 0 {
				ok = add(value.Args, childDepth, item.selected)
			}
		case tugqlBody:
			queryDepth := item.depth
			if item.bodyQueryIsChild {
				queryDepth = childDepth
			}
			ok = add(value.Query, queryDepth, item.selected)
			for _, definition := range value.Definitions {
				if ok && !pushDefinition(definition, childDepth, item.selected, item.bodyQueryIsChild) {
					ok = false
				}
			}
		case tugqlDefinition:
			if value.Query != nil {
				queryDepth := item.depth
				if item.nestedDefinition {
					queryDepth++
				}
				ok = pushBody(*value.Query, queryDepth, item.selected, item.nestedDefinition)
			}
			// Mapping wrappers are not canonical semantic nodes here; enqueue
			// each mapping's expression directly so pending-work accounting
			// matches the exported tree.
			for _, mapping := range value.Using {
				if ok && !add(mapping.Expression, childDepth, item.selected) {
					ok = false
				}
			}
		case tugqlParameter:
			if value.Default != nil {
				ok = add(*value.Default, childDepth, item.selected)
			}
		case condYAML:
			if value.Left != nil {
				ok = add(*value.Left, childDepth, item.selected)
			}
			if ok && value.Right != nil {
				ok = add(*value.Right, childDepth, item.selected)
			}
			if ok && value.IsNull != nil {
				ok = add(*value.IsNull, childDepth, item.selected)
			}
			if ok && value.IsNotNull != nil {
				ok = add(*value.IsNotNull, childDepth, item.selected)
			}
			if ok && value.Exists != nil {
				ok = add(*value.Exists, childDepth, item.selected)
			}
			if ok && value.NotExists != nil {
				ok = add(*value.NotExists, childDepth, item.selected)
			}
			if ok && len(value.And) > 0 {
				ok = add(value.And, childDepth, item.selected)
			}
			if ok && len(value.Or) > 0 {
				ok = add(value.Or, childDepth, item.selected)
			}
		case []condYAML:
			for _, child := range value {
				if !add(child, childDepth, item.selected) {
					ok = false
					break
				}
			}
		case existsYAML:
			if value.Query != nil {
				ok = add(*value.Query, childDepth, item.selected)
			}
		case wildcardYAML:
			// Wildcard options are scalar leaves.
		}
		if !ok {
			if limitError == "nodes" {
				return []tugqlDiagnostic{diagnostic("document_node_limit", fmt.Sprintf("TugQL semantic tree exceeds %d nodes", maxTugQLSemanticNodes), rootQuerySpan)}
			}
			if item.selected {
				return []tugqlDiagnostic{diagnostic("invalid_select", fmt.Sprintf("expression nesting exceeds %d", maxTugQLStructuralDepth), rootSelectSpan)}
			}
			return []tugqlDiagnostic{diagnostic("document_depth_exceeded", fmt.Sprintf("TugQL semantic tree depth exceeds %d", maxTugQLStructuralDepth), rootQuerySpan)}
		}
	}
	return nil
}
