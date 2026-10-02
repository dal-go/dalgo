package dalgo2memory

import (
	"fmt"
	"strings"

	"github.com/dal-go/dalgo/dal"
)

// comparisonMatcher decides one Comparison for the current row. The two WHERE
// paths differ only here: a single-source query keeps the Firestore-shaped
// comparison rules (matchesComparison), a joined query resolves both operands
// per source (matchesJoinComparison).
type comparisonMatcher func(dal.Comparison) (bool, error)

// evalWhere evaluates a WHERE condition tree for one row, for both the
// single-source and the joined path, so AND/OR composition and IS [NOT] NULL
// mean the same in both. A nil condition matches. A condition the adapter
// cannot evaluate is an error: ignoring it would return the wrong rows.
//
// An IS NULL operand is a field (an absent field, a null-extended LEFT JOIN
// side and an explicit nil all read as NULL) or a literal; any other operand is
// an error.
func evalWhere(cond dal.Condition, sources map[string]map[string]any, known map[string]bool, compare comparisonMatcher) (bool, error) {
	switch c := cond.(type) {
	case nil:
		return true, nil
	case dal.IsNullCondition:
		value, _, err := resolveJoinExpr(c.Operand(), sources, known)
		if err != nil {
			return false, err
		}
		return dal.IsNullValue(value) != c.Negated(), nil
	case dal.GroupCondition:
		isOr := c.Operator() == dal.Or
		if !isOr && c.Operator() != dal.And {
			return false, fmt.Errorf("dalgo2memory: unsupported group operator %q in WHERE", c.Operator())
		}
		for _, child := range c.Conditions() {
			ok, err := evalWhere(child, sources, known, compare)
			if err != nil {
				return false, err
			}
			if ok == isOr {
				return isOr, nil
			}
		}
		return !isOr, nil
	case dal.Comparison:
		return compare(c)
	default:
		return false, fmt.Errorf("dalgo2memory: unsupported condition %T in WHERE", cond)
	}
}

// matchesWhere evaluates a single-source WHERE. Comparisons keep the shapes
// dalgo2firestore translates to native Firestore filters, so memory-backed
// tests behave like the Firestore adapter:
//
//   - FieldRef op Constant for ==, >, >=, <, <=
//   - Constant In FieldRef    → Firestore's "array-contains"
//   - FieldRef op dal.Array   → Firestore's "array-contains-any"
//
// Any other comparison shape does not match. Unlike Firestore, which cannot
// run them, OR groups are evaluated, and so are IS [NOT] NULL tests.
func matchesWhere(cond dal.Condition, sources map[string]map[string]any, known map[string]bool) (bool, error) {
	return evalWhere(cond, sources, known, func(c dal.Comparison) (bool, error) {
		return matchesComparison(sources[""], c), nil
	})
}

// matchesJoinWhere evaluates a joined query's WHERE over the per-source data.
func matchesJoinWhere(cond dal.Condition, sources map[string]map[string]any, known map[string]bool) (bool, error) {
	return evalWhere(cond, sources, known, func(c dal.Comparison) (bool, error) {
		return matchesJoinComparison(c, sources, known)
	})
}

// matchesJoinCondition evaluates one ON condition, which must be a Comparison
// (ValidateJoinTree enforces the same for DTQL). A nil condition matches.
func matchesJoinCondition(cond dal.Condition, sources map[string]map[string]any, known map[string]bool) (bool, error) {
	switch c := cond.(type) {
	case nil:
		return true, nil
	case dal.Comparison:
		return matchesJoinComparison(c, sources, known)
	default:
		return false, fmt.Errorf("dalgo2memory: unsupported ON condition %T: an ON predicate must be a comparison", cond)
	}
}

// matchesJoinComparison evaluates a Comparison over the per-source data:
// equality, and the ordered operators (< <= > >=), which are false when either
// side is absent or null. Any other operator is an error rather than a silent
// non-match.
func matchesJoinComparison(cmp dal.Comparison, sources map[string]map[string]any, known map[string]bool) (bool, error) {
	switch cmp.Operator {
	case dal.Equal, dal.LessThen, dal.LessOrEqual, dal.GreaterThen, dal.GreaterOrEqual:
	default:
		return false, fmt.Errorf("dalgo2memory: unsupported operator %q in a joined query", cmp.Operator)
	}
	l, lok, err := resolveJoinExpr(cmp.Left, sources, known)
	if err != nil {
		return false, err
	}
	r, rok, err := resolveJoinExpr(cmp.Right, sources, known)
	if err != nil {
		return false, err
	}
	if !lok || !rok {
		return false, nil
	}
	if cmp.Operator == dal.Equal {
		return valuesEqual(l, r), nil
	}
	return !dal.IsNullValue(l) && !dal.IsNullValue(r) && compareOp(cmp.Operator, l, r), nil
}

// fieldValue reads a field from a record's data. A name that is a key of the
// map is used as it stands; otherwise a dotted name is a path into nested
// maps (address.city), as the generic executor and Firestore resolve it. The
// bool reports whether the field is present.
func fieldValue(data map[string]any, name string) (any, bool) {
	if value, ok := data[name]; ok {
		return value, true
	}
	if !strings.Contains(name, ".") {
		return nil, false
	}
	var current any = data
	for _, part := range strings.Split(name, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		if current, ok = object[part]; !ok {
			return nil, false
		}
	}
	return current, true
}
