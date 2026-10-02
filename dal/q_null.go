package dal

// IsNullCondition tests whether an expression evaluates to NULL. Negated
// distinguishes IS NOT NULL while keeping Condition's existing method set, the
// way ExistsCondition does for NOT EXISTS.
//
// It is the one portable way to select or exclude NULLs. The comparison
// operators (Equal, In, NotIn, GreaterThen, ...) keep their own per-engine
// semantics; a recursive or joined query evaluates them with SQL's
// three-valued logic, in which a comparison with NULL is UNKNOWN and never
// matches. IsNullCondition is never UNKNOWN: it is TRUE or FALSE for every
// input, so it composes with AND and OR without surprises.
//
// A field absent from a record counts as NULL, the same rule the join and
// aggregation executors apply when they read a missing field.
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
