package dal

import "reflect"

// IsNullCondition tests whether an expression evaluates to NULL. Negated
// distinguishes IS NOT NULL while keeping Condition's existing method set, the
// way ExistsCondition does for NOT EXISTS.
//
// It is TRUE or FALSE for every input, never UNKNOWN, so it composes with AND
// and OR without surprises. A field absent from a record counts as NULL, the
// same rule the join and aggregation executors apply when they read a missing
// field, and so does a nil pointer (see IsNullValue).
//
// IsNullCondition does not change what the comparison operators mean, and they
// do not all mean the same thing against NULL:
//
//   - WHERE (and JOIN ON) evaluated by the generic executor, that is any query
//     with a join, a nested query or a federated aggregation, uses SQL
//     three-valued logic: a comparison with NULL is UNKNOWN and keeps no row,
//     so `x == nil` matches nothing there.
//   - HAVING of a local aggregation is two-valued: `x == nil` is true when x is
//     NULL (so HAVING g == nil returns the NULL group), `<`, `<=`, `>` and `>=`
//     are false when either side is NULL, and In/NotIn are not supported.
//   - A single-source query handed to an adapter gets that adapter's meaning of
//     `x == nil` (the in-memory adapter and the SQLite emitter match NULL and,
//     for the in-memory adapter, an absent field).
//
// That WHERE/HAVING difference predates IsNullCondition and is not changed by
// it. IsNullCondition is the one test whose meaning is the same in every
// position; an adapter that cannot translate it must fail the query rather
// than ignore it.
type IsNullCondition struct {
	operand Expression
	negated bool
}

var _ Condition = IsNullCondition{}

// NewIsNullCondition creates an `operand IS NULL` predicate.
func NewIsNullCondition(operand Expression) IsNullCondition {
	return IsNullCondition{operand: operand}
}

// NewIsNotNullCondition creates an `operand IS NOT NULL` predicate.
func NewIsNotNullCondition(operand Expression) IsNullCondition {
	return IsNullCondition{operand: operand, negated: true}
}

// Operand returns the expression tested for NULL.
func (c IsNullCondition) Operand() Expression { return c.operand }

// Negated reports whether the predicate is IS NOT NULL.
func (c IsNullCondition) Negated() bool { return c.negated }

// String renders the predicate as an SQL fragment: `x IS NULL` or
// `x IS NOT NULL`.
func (c IsNullCondition) String() string {
	operand := "{NO_OPERAND}"
	if c.operand != nil {
		operand = c.operand.String()
	}
	if c.negated {
		return operand + " IS NOT NULL"
	}
	return operand + " IS NULL"
}

// IsNull creates a condition that is true when the field is NULL or absent.
func (f FieldRef) IsNull() Condition { return NewIsNullCondition(f) }

// IsNotNull creates a condition that is true when the field is present and
// not NULL.
func (f FieldRef) IsNotNull() Condition { return NewIsNotNullCondition(f) }

// IsNullValue reports whether a runtime value counts as NULL for an
// IsNullCondition: an untyped nil, or a nil pointer held in an interface (a
// "typed nil", as an adapter may hand back for an unset optional field).
// Zero values (0, "", false), empty collections and a nil slice or map are
// values, not NULL.
func IsNullValue(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Pointer && reflected.IsNil()
}
