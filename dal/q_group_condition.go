package dal

import (
	"fmt"
	"strings"
)

type GroupCondition struct {
	operator   Operator
	conditions []Condition
}

// NewGroupCondition creates a GroupCondition combining the given conditions with
// a group operator (e.g. And or Or). It is the public constructor for the
// otherwise unexported fields, enabling callers outside the dal package (such as
// the dtql serializer) to reconstruct group conditions.
func NewGroupCondition(operator Operator, conditions ...Condition) GroupCondition {
	return GroupCondition{operator: operator, conditions: conditions}
}

func (v GroupCondition) Operator() Operator {
	return v.operator
}

func (v GroupCondition) Conditions() []Condition {
	return v.conditions
}

func (v GroupCondition) String() string {
	s := make([]string, len(v.conditions))
	for i, condition := range v.conditions {
		s[i] = condition.String()
	}
	// Surround the operator with spaces so emitted SQL fragments like
	// `(a = 1 AND b = 2)` are well-formed for text-passthrough drivers.
	// Empty group (no conditions) collapses to "()" — the join produces
	// the empty string, and the parens come from the Sprintf wrap.
	return fmt.Sprintf("(%v)", strings.Join(s, " "+string(v.operator)+" "))
}
