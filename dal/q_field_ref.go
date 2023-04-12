package dal

import "fmt"

type FieldRef struct {
	Name string
	IsID bool
}

// String returns string representation of a field
func (f FieldRef) String() string {
	return fmt.Sprintf("[%v]", f.Name)
}

// EqualTo creates equality condition for a field
func (f FieldRef) EqualTo(v any) Condition {
	var val Expression
	switch v := v.(type) {
	case string, int:
		val = constantCondition{Value: v}
	case constantCondition:
		val = v
	case FieldRef:
		val = v
	}
	return equal{Comparison: Comparison{Operator: Equal, Expressions: []Expression{f, val}}}
}
