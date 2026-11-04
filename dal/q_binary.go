package dal

import "fmt"

// ArithmeticOperator is a portable arithmetic operation in an expression.
type ArithmeticOperator string

const (
	// Add is the portable addition operator.
	Add ArithmeticOperator = "+"
	// Subtract is the portable subtraction operator.
	Subtract ArithmeticOperator = "-"
	// Multiply is the portable multiplication operator.
	Multiply ArithmeticOperator = "*"
	// Divide is the portable division operator.
	Divide ArithmeticOperator = "/"
)

// BinaryExpression combines two scalar expressions. Keeping operands as
// Expression preserves compatibility with future qualified fields and scalar
// functions without changing aggregate argument shapes.
type BinaryExpression struct {
	Left     Expression
	Operator ArithmeticOperator
	Right    Expression
}

func (v BinaryExpression) String() string {
	return fmt.Sprintf("(%s %s %s)", v.Left, v.Operator, v.Right)
}

// Binary constructs a portable arithmetic expression.
func Binary(left Expression, operator ArithmeticOperator, right Expression) BinaryExpression {
	return BinaryExpression{Left: left, Operator: operator, Right: right}
}
