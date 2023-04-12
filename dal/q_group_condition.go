package dal

import (
	"fmt"
	"strings"
)

type groupCondition struct {
	operator   Operator
	conditions []Condition
}

func (v groupCondition) Operator() Operator {
	return v.operator
}

func (v groupCondition) Conditions() []Condition {
	return v.conditions
}

func (v groupCondition) String() string {
	s := make([]string, len(v.conditions))
	for i, condition := range v.conditions {
		s[i] = condition.String()
	}
	return fmt.Sprintf("(%v)", strings.Join(s, string(v.operator)))
}
