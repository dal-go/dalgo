package dal

import (
	"fmt"
)

type FieldRef struct {
	Name string
	IsID bool
}

func (f FieldRef) Equal(b FieldRef) bool {
	return f.Name == b.Name && f.IsID == b.IsID
}

// String returns string representation of a field
func (f FieldRef) String() string {
	return fmt.Sprintf("[%v]", f.Name)
}

// EqualTo creates equality condition for a field
func (f FieldRef) EqualTo(v any) Condition {
	return WhereField(f.Name, Equal, v)
}

func WhereField(name string, operator Operator, v any) Condition {
	var val Expression
	switch v := v.(type) {
	case string, int:
		val = Constant{Value: v}
	case Constant:
		val = v
	case FieldRef:
		val = v
	default:

	}
	return Comparison{Operator: operator, Left: Field(name), Right: val}
}
