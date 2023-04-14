package dal

// Operator defines a Comparison operator
type Operator string

const (
	// Equal is a Comparison operator
	Equal Operator = "=="

	// In is a Comparison operator
	In Operator = "In"

	// GreaterThen is a Comparison operator
	GreaterThen Operator = ">"

	// GreaterOrEqual is a Comparison operator
	GreaterOrEqual Operator = ">="

	// LessThen is a Comparison operator
	LessThen Operator = "<"

	// LessOrEqual is a Comparison operator
	LessOrEqual Operator = "<="

	// And is a Comparison operator // TODO: Is it an operator?
	And = "AND"

	// Or is a Comparison operator // TODO: Is it an operator?
	Or = "OR"
)
