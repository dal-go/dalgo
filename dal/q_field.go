package dal

import "fmt"

// NewFieldRef creates an expression that represents a FieldRef value.
// source qualifies the field with its recordset; an empty source denotes
// the single From base recordset.
func NewFieldRef(source, name string) FieldRef {
	return FieldRef{source: source, name: name}
}

// OrderExpression is an expression paired with a sort direction.
type OrderExpression interface {
	fmt.Stringer
	// Expression returns the expression being ordered.
	Expression() Expression
	// Descending reports whether the order is descending.
	Descending() bool
}

type orderExpression struct {
	expression Expression
	descending bool
}

// Expression returns the expression being ordered.
func (v orderExpression) Expression() Expression {
	return v.expression
}

func (v orderExpression) String() string {
	if v.descending {
		return v.expression.String() + " DESC"
	}
	return v.expression.String()
}

// Descending reports whether the order is descending.
func (v orderExpression) Descending() bool {
	return v.descending
}

// Ascending returns an order of expression with the descending flag clear.
func Ascending(expression Expression) OrderExpression {
	return orderExpression{expression: expression, descending: false}
}

// AscendingField returns an ascending order of the named field.
// The field is unqualified, so it denotes the single From base recordset.
func AscendingField(name string) OrderExpression {
	return Ascending(NewFieldRef("", name))
}

// Descending returns an order of expression with the descending flag set.
func Descending(expression Expression) OrderExpression {
	return orderExpression{expression: expression, descending: true}
}

// DescendingField returns a descending order of the named field.
// The field is unqualified, so it denotes the single From base recordset.
func DescendingField(name string) OrderExpression {
	return Descending(NewFieldRef("", name))
}
