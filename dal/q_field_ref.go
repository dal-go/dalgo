package dal

import (
	"fmt"
	"regexp"
	"time"
)

// FieldRef names a field by source and name, and can mark that field as an ID.
type FieldRef struct {
	source string
	name   string
	isID   bool
}

// IsID reports whether the reference names an ID field.
func (f FieldRef) IsID() bool {
	return f.isID
}

// Name returns the field name.
func (f FieldRef) Name() string {
	return f.name
}

// Source returns the recordset qualifier of the field. An empty source
// denotes the single From base recordset.
func (f FieldRef) Source() string {
	return f.source
}

// Equal reports whether f and b have the same source, ID flag, and name.
func (f FieldRef) Equal(b FieldRef) bool {
	return f.source == b.source && f.isID == b.isID && f.name == b.name
}

// String returns string representation of a field
func (f FieldRef) String() string {
	name := f.name
	if RequiresEscaping(f.name) {
		name = fmt.Sprintf("[%v]", f.name)
	}
	if f.source != "" {
		return f.source + "." + name
	}
	return name
}

// Empty string requires escaping!
var reRegularName = regexp.MustCompile(`^\w+$`)

// RequiresEscaping reports whether s is not a single word of letters, digits, and underscore.
// An empty string requires escaping.
func RequiresEscaping(s string) bool {
	return !reRegularName.MatchString(s)
}

// EqualTo creates equality condition for a field
func (f FieldRef) EqualTo(v any) Condition {
	return WhereField(f.name, Equal, v)
}

// WhereField returns a comparison of the named field with v using operator.
// Literals become constants. Slices and Array values are accepted only with
// In or NotIn. Constant, FieldRef, and Param values are kept as given, and any other type panics.
func WhereField(name string, operator Operator, v any) Condition {
	var val Expression
	switch v := v.(type) {
	case
		nil,
		bool,
		string,
		float32, float64,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		time.Time:
		val = Constant{Value: v}
	case []any, []string, []int, []int8, []int16, []int32, []int64, []uint, []uint8, []uint16, []uint32, []uint64, []float32, []float64:
		if operator != In && operator != NotIn {
			panic("arrays must use with `In` or `NotIn` operator")
		}
		val = Array{Value: v}
	case Constant:
		val = v
	case FieldRef:
		val = v
	case Param:
		val = v
	case Array:
		if operator != In && operator != NotIn {
			panic("arrays must use with `In` or `NotIn` operator")
		}
		val = v
	default:
		panic(fmt.Errorf("unsupported type %T", v))
	}
	return Comparison{Operator: operator, Left: Field(name), Right: val}
}
