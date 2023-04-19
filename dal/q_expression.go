package dal

import "fmt"

// Expression represent either a FieldRef, Constant or a formula
type Expression interface {
	fmt.Stringer
}
