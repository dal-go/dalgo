package dal

import "fmt"

// JoinValidationError is a stable structural JOIN diagnostic. Category is one
// of the DTQL JOIN categories and Path addresses the offending tree node.
type JoinValidationError struct {
	Category string
	Path     string
	Message  string
}

func (e *JoinValidationError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("%s at %s", e.Category, e.Path)
	}
	return fmt.Sprintf("%s at %s: %s", e.Category, e.Path, e.Message)
}

// ValidateJoinTree validates a recursive From tree without schema metadata.
// It therefore owns relation shape, aliases, ON form and lexical scope; field
// existence and key type checks belong to schema-aware validation and execution.
func ValidateJoinTree(from FromSource) error {
	if from == nil {
		return joinError("join_shape", "from", "from is required")
	}
	return validateJoinFrom(from, "from", nil, map[FromSource]bool{})
}

func validateJoinFrom(from FromSource, path string, visible map[string]bool, visiting map[FromSource]bool) error {
	if from == nil || from.Base() == nil {
		return joinError("join_shape", path, "from is required")
	}
	if visiting[from] {
		return joinError("join_cycle", path, "recursive from reference")
	}
	visiting[from] = true
	defer delete(visiting, from)

	aliases := cloneAliases(visible)
	name := from.Base().Name()
	alias := from.Base().Alias()
	if alias == "" {
		alias = name
	}
	if alias == "" {
		return joinError("join_shape", path+".name", "name is required")
	}
	if aliases[alias] {
		return joinError("join_scope", path+".alias", fmt.Sprintf("duplicate alias %q", alias))
	}
	aliases[alias] = true

	for i, join := range from.Joins() {
		joinPath := fmt.Sprintf("%s.joins[%d]", path, i)
		if join.JoinType() != JoinInner && join.JoinType() != JoinLeft {
			return joinError("join_type", joinPath+".type", fmt.Sprintf("unsupported join type %q", join.JoinType()))
		}
		child := join.From()
		if child == nil {
			if join.RecordsetSource == nil {
				return joinError("join_shape", joinPath+".from", "from is required")
			}
			child = From(join.RecordsetSource)
		}
		if len(join.On()) == 0 {
			return joinError("join_shape", joinPath+".on", "on must contain at least one predicate")
		}
		childAliases, err := relationAliases(child, joinPath+".from", visiting)
		if err != nil {
			return err
		}
		for childAlias := range childAliases {
			if aliases[childAlias] {
				return joinError("join_scope", joinPath+".from.alias", fmt.Sprintf("duplicate alias %q", childAlias))
			}
		}
		for onIndex, condition := range join.On() {
			if err := validateJoinCondition(condition, joinPath+fmt.Sprintf(".on[%d]", onIndex), aliases, childAliases); err != nil {
				return err
			}
		}
		if err := validateJoinFrom(child, joinPath+".from", aliases, visiting); err != nil {
			return err
		}
		for childAlias := range childAliases {
			aliases[childAlias] = true
		}
	}
	return nil
}

func relationAliases(from FromSource, path string, visiting map[FromSource]bool) (map[string]bool, error) {
	if from == nil || from.Base() == nil {
		return nil, joinError("join_shape", path, "from is required")
	}
	if visiting[from] {
		return nil, joinError("join_cycle", path, "recursive from reference")
	}
	aliases := map[string]bool{}
	walking := map[FromSource]bool{}
	var walk func(FromSource, string) error
	walk = func(node FromSource, nodePath string) error {
		if node == nil || node.Base() == nil {
			return joinError("join_shape", nodePath, "from is required")
		}
		if walking[node] {
			return joinError("join_cycle", nodePath, "recursive from reference")
		}
		walking[node] = true
		defer delete(walking, node)
		name, alias := node.Base().Name(), node.Base().Alias()
		if alias == "" {
			alias = name
		}
		if alias == "" {
			return joinError("join_shape", nodePath+".name", "name is required")
		}
		if aliases[alias] {
			return joinError("join_scope", nodePath+".alias", fmt.Sprintf("duplicate alias %q", alias))
		}
		aliases[alias] = true
		for i, join := range node.Joins() {
			child := join.From()
			if child == nil {
				child = From(join.RecordsetSource)
			}
			if err := walk(child, fmt.Sprintf("%s.joins[%d].from", nodePath, i)); err != nil {
				return err
			}
		}
		return nil
	}
	return aliases, walk(from, path)
}

func validateJoinCondition(condition Condition, path string, parent, child map[string]bool) error {
	comparison, ok := condition.(Comparison)
	if !ok {
		return joinError("join_shape", path, "ON predicate must be a comparison")
	}
	if comparison.Operator != Equal {
		return joinError("join_operator", path+".op", "only == is supported")
	}
	left, leftOK := comparison.Left.(FieldRef)
	right, rightOK := comparison.Right.(FieldRef)
	if !leftOK || !rightOK || left.Name() == "" || right.Name() == "" || left.Source() == "" || right.Source() == "" {
		return joinError("join_shape", path, "ON operands must be qualified field references")
	}
	leftInParent, rightInParent := parent[left.Source()], parent[right.Source()]
	leftInChild, rightInChild := child[left.Source()], child[right.Source()]
	if !leftInParent && !leftInChild {
		return joinError("join_scope", path+".left.source", fmt.Sprintf("unknown or forward alias %q", left.Source()))
	}
	if !rightInParent && !rightInChild {
		return joinError("join_scope", path+".right.source", fmt.Sprintf("unknown or forward alias %q", right.Source()))
	}
	return nil
}

func cloneAliases(source map[string]bool) map[string]bool {
	copy := make(map[string]bool, len(source)+1)
	for alias := range source {
		copy[alias] = true
	}
	return copy
}

func joinError(category, path, message string) error {
	return &JoinValidationError{Category: category, Path: path, Message: message}
}
