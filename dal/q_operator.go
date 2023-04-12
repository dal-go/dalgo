package dal

// Operator defines a Comparison operator
type Operator string

const (
	// Equal is a Comparison operator
	Equal Operator = "=="

	// And is a Comparison operator
	And = "AND"

	// Or is a Comparison operator
	Or = "OR"

	// In is a Comparison operator
	In = "In"
)
