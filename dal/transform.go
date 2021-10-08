package dal

// Transform defines a transform operation
type Transform interface {

	// Name returns name of a transform
	Name() string

	// Value returns arguments of transform
	Value() interface{}
}

type transform struct {
	name  string
	value interface{}
}

func (v transform) Name() string {
	return v.name
}

func (v transform) Value() interface{} {
	return v.name
}

// Increment defines an increment transform operation
func Increment(v int) Transform {
	return transform{name: "increment", value: v}
}
