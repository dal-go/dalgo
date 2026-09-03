package access

import (
	"fmt"

	"github.com/dal-go/dalgo/dal"
)

// DocumentCondition is a row condition in a portable policy document. It uses
// the DTQL condition syntax so a policy and a saved query are written alike: a
// comparison sets op/left/right; a group sets and or or.
type DocumentCondition struct {
	Op    string              `json:"op,omitempty" yaml:"op,omitempty"`
	Left  *DocumentExpression `json:"left,omitempty" yaml:"left,omitempty"`
	Right *DocumentExpression `json:"right,omitempty" yaml:"right,omitempty"`
	And   []DocumentCondition `json:"and,omitempty" yaml:"and,omitempty"`
	Or    []DocumentCondition `json:"or,omitempty" yaml:"or,omitempty"`
}

// DocumentExpression is one operand: exactly one of field, value, values or
// param is set.
type DocumentExpression struct {
	Field  string `json:"field,omitempty" yaml:"field,omitempty"`
	Value  any    `json:"value,omitempty" yaml:"value,omitempty"`
	Values any    `json:"values,omitempty" yaml:"values,omitempty"`
	Param  string `json:"param,omitempty" yaml:"param,omitempty"`
}

var documentOperators = map[string]dal.Operator{
	string(dal.Equal):          dal.Equal,
	string(dal.In):             dal.In,
	string(dal.GreaterThen):    dal.GreaterThen,
	string(dal.GreaterOrEqual): dal.GreaterOrEqual,
	string(dal.LessThen):       dal.LessThen,
	string(dal.LessOrEqual):    dal.LessOrEqual,
}

func conditionFromDocument(condition DocumentCondition) (dal.Condition, error) {
	isComparison := condition.Op != "" || condition.Left != nil || condition.Right != nil
	forms := 0
	if isComparison {
		forms++
	}
	if condition.And != nil {
		forms++
	}
	if condition.Or != nil {
		forms++
	}
	switch {
	case forms == 0:
		return nil, fmt.Errorf("condition must be a comparison (op/left/right) or a group (and/or)")
	case forms > 1:
		return nil, fmt.Errorf("condition mixes comparison and group forms")
	}
	if isComparison {
		operator, ok := documentOperators[condition.Op]
		if !ok {
			return nil, fmt.Errorf("unknown comparison operator %q", condition.Op)
		}
		if condition.Left == nil || condition.Right == nil {
			return nil, fmt.Errorf("comparison requires both left and right")
		}
		left, err := expressionFromDocument(*condition.Left)
		if err != nil {
			return nil, fmt.Errorf("comparison left: %w", err)
		}
		right, err := expressionFromDocument(*condition.Right)
		if err != nil {
			return nil, fmt.Errorf("comparison right: %w", err)
		}
		return dal.NewComparison(left, operator, right), nil
	}
	operator, members := dal.Operator(dal.And), condition.And
	if condition.Or != nil {
		operator, members = dal.Or, condition.Or
	}
	if len(members) == 0 {
		return nil, fmt.Errorf("%s group must have at least one condition", operator)
	}
	conditions := make([]dal.Condition, 0, len(members))
	for i, member := range members {
		sub, err := conditionFromDocument(member)
		if err != nil {
			return nil, fmt.Errorf("group condition #%d: %w", i, err)
		}
		conditions = append(conditions, sub)
	}
	return dal.NewGroupCondition(operator, conditions...), nil
}

func expressionFromDocument(expression DocumentExpression) (dal.Expression, error) {
	set := 0
	for _, present := range []bool{expression.Field != "", expression.Value != nil, expression.Values != nil, expression.Param != ""} {
		if present {
			set++
		}
	}
	if set != 1 {
		return nil, fmt.Errorf("expression must set exactly one of field, value, values or param")
	}
	switch {
	case expression.Field != "":
		return dal.NewFieldRef("", expression.Field), nil
	case expression.Value != nil:
		return dal.Constant{Value: expression.Value}, nil
	case expression.Param != "":
		if !dal.ValidParamName(expression.Param) {
			return nil, fmt.Errorf("invalid parameter name %q", expression.Param)
		}
		return dal.Param{Name: expression.Param}, nil
	default:
		return dal.Array{Value: expression.Values}, nil
	}
}

func documentFromCondition(condition dal.Condition) (*DocumentCondition, error) {
	switch c := condition.(type) {
	case dal.Comparison:
		if _, ok := documentOperators[string(c.Operator)]; !ok {
			return nil, fmt.Errorf("unsupported comparison operator %q", c.Operator)
		}
		left, err := documentFromExpression(c.Left)
		if err != nil {
			return nil, fmt.Errorf("comparison left: %w", err)
		}
		right, err := documentFromExpression(c.Right)
		if err != nil {
			return nil, fmt.Errorf("comparison right: %w", err)
		}
		return &DocumentCondition{Op: string(c.Operator), Left: left, Right: right}, nil
	case dal.GroupCondition:
		members := make([]DocumentCondition, 0, len(c.Conditions()))
		for i, sub := range c.Conditions() {
			member, err := documentFromCondition(sub)
			if err != nil {
				return nil, fmt.Errorf("group condition #%d: %w", i, err)
			}
			members = append(members, *member)
		}
		switch c.Operator() {
		case dal.And:
			return &DocumentCondition{And: members}, nil
		case dal.Or:
			return &DocumentCondition{Or: members}, nil
		default:
			return nil, fmt.Errorf("unsupported group operator %q", c.Operator())
		}
	default:
		return nil, fmt.Errorf("unsupported condition %T", condition)
	}
}

func documentFromExpression(expression dal.Expression) (*DocumentExpression, error) {
	switch e := expression.(type) {
	case dal.FieldRef:
		return &DocumentExpression{Field: e.Name()}, nil
	case dal.Constant:
		return &DocumentExpression{Value: e.Value}, nil
	case dal.Array:
		return &DocumentExpression{Values: e.Value}, nil
	case dal.Param:
		return &DocumentExpression{Param: e.Name}, nil
	default:
		return nil, fmt.Errorf("unsupported expression %T", expression)
	}
}
