package dal

import (
	"fmt"
	"strings"
)

type GroupCondition struct {
	operator   Operator
	conditions []Condition
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
	return fmt.Sprintf("(%v)", strings.Join(s, string(v.operator)))
}
