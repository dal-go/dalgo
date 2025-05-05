package constant

import (
	"fmt"
	"strings"
)

// StrConst defines a constant string value.
type StrConst struct {
	value string
}

// Str returns a constant string value.
func Str(v string) StrConst {
	return StrConst{value: v}
}

// String returns a string representation of the constant string value.
func (v StrConst) String() string {
	return fmt.Sprintf("'%s'", strings.ReplaceAll(v.value, "'", "''"))
}
