package dal

import "fmt"

// Expression represent either a FieldRef, constantCondition or a formula
type Expression interface {
	fmt.Stringer
}
