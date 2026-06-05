package dal

import (
	"fmt"
	"regexp"
	"time"
)

type FieldRef struct {
	source string
	name   string
	isID   bool
}

func (f FieldRef) IsID() bool {
	return f.isID
}

func (f FieldRef) Name() string {
	return f.name
}

// Source returns the recordset qualifier of the field. An empty source
// denotes the single From base recordset.
func (f FieldRef) Source() string {
	return f.source
}

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

func RequiresEscaping(s string) bool {
	return !reRegularName.MatchString(s)
}

// EqualTo creates equality condition for a field
func (f FieldRef) EqualTo(v any) Condition {
	return WhereField(f.name, Equal, v)
}

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
	case []string, []int, []int8, []int16, []int32, []int64, []uint, []uint8, []uint16, []uint32, []uint64, []float32, []float64:
		if operator != In {
			panic("arrays must use with `In` operator")
		}
		val = Array{Value: v}
	case Constant:
		val = v
	case FieldRef:
		val = v
	case Array:
		if operator != In {
			panic("arrays must use with `In` operator")
		}
		val = v
	default:
		panic(fmt.Errorf("unsupported type %T", v))
	}
	return Comparison{Operator: operator, Left: Field(name), Right: val}
}
