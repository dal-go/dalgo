package dal

import (
	"fmt"
	"regexp"
)

var _ Expression = Param{}

// Param is a named parameter expression, written "$<name>" in query text and
// DTQL. It stands for a runtime value that is substituted before a query or a
// policy condition is executed: access policies resolve it from the operation
// context ($currentUser, $now, named variables), and a saved DTQL query can
// carry it until execution time. Adapters never receive a Param; the layer that
// owns the values replaces it with a Constant (scalar) or an Array (slice).
type Param struct {
	Name string
}

var reParamName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)

// ValidParamName reports whether name is a legal parameter name: an
// identifier, optionally dotted (for example "currentUser" or "principal.roles").
func ValidParamName(name string) bool {
	return reParamName.MatchString(name)
}

// NewParam creates a parameter expression and panics on an invalid name.
func NewParam(name string) Param {
	if !ValidParamName(name) {
		panic(fmt.Errorf("dal: invalid parameter name %q", name))
	}
	return Param{Name: name}
}

// String returns the parameter in its "$name" form.
func (p Param) String() string {
	return "$" + p.Name
}

// Equal reports whether two parameters have the same name.
func (p Param) Equal(b Param) bool {
	return p.Name == b.Name
}
