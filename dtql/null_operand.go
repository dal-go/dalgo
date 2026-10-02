package dtql

import (
	"fmt"

	"github.com/dal-go/dalgo/dal"
)

// validateNullTests rejects an isNull / isNotNull whose operand is not
// something that can be NULL: a values list, a star and a param are not
// scalars a query can test, and an aggregate has no value outside HAVING.
// Fields, literals, arithmetic and scalar subqueries are accepted. The path is
// built the way condFromYAMLAt builds it, so an error points at the operand's
// own key.
func validateNullTests(condition dal.Condition, path string, having bool) error {
	switch c := condition.(type) {
	case dal.IsNullCondition:
		key := ".isNull"
		if c.Negated() {
			key = ".isNotNull"
		}
		if reason := nullOperandProblem(c.Operand(), having); reason != "" {
			return fmt.Errorf("invalid DTQL: query_shape at %s%s: %s", path, key, reason)
		}
	case dal.GroupCondition:
		key := ".and"
		if c.Operator() == dal.Or {
			key = ".or"
		}
		for i, child := range c.Conditions() {
			if err := validateNullTests(child, fmt.Sprintf("%s%s[%d]", path, key, i), having); err != nil {
				return err
			}
		}
	}
	return nil
}

// nullOperandProblem explains why an expression cannot be the operand of a
// null test, or returns "" when it can.
func nullOperandProblem(operand dal.Expression, having bool) string {
	switch e := operand.(type) {
	case dal.FieldRef, dal.Constant, dal.QueryExpression:
		return ""
	case dal.BinaryExpression:
		if reason := nullOperandProblem(e.Left, having); reason != "" {
			return reason
		}
		return nullOperandProblem(e.Right, having)
	case dal.AggregateFunc:
		if !having {
			return "an aggregate has no value in where; test it in having"
		}
		return ""
	case dal.Array:
		return "a values list is not a scalar to test"
	case dal.StarExpression:
		return "star is not a value to test"
	case dal.Param:
		return "a param is not a value to test"
	}
	return fmt.Sprintf("operand %T cannot be tested", operand)
}
