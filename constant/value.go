package constant

import (
	"fmt"
)

// ValueConst defines a constant value of any type.
type ValueConst struct {
	value any
}

// Value returns a constant value of any type.
func Value(v any) ValueConst {
	return ValueConst{value: v}
}

// String returns a string representation of the constant value.
func (v ValueConst) String() string {
	switch val := v.value.(type) {
	case string:
		return fmt.Sprintf("'%s'", val)
	default:
		return fmt.Sprintf("%+v", val)
	}
}
