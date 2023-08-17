package constant

import (
	"strconv"
)

// IntConst defines a constant integer value.
type IntConst struct {
	value int
}

// String returns a string representation of the constant integer value.
func (i IntConst) String() string {
	return strconv.Itoa(i.value)
}

// Int returns a constant integer value.
func Int(v int) IntConst {
	return IntConst{value: v}
}
