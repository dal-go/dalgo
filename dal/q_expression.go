package dal

import "fmt"

// Expression represent either a FieldRef, constantExpression or a formula
type Expression interface {
	fmt.Stringer
}
