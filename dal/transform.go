package dal

// Transform defines a transform operation
type Transform interface {

	// Name returns Name of a transform
	Name() string

	// Value returns arguments of transform
	Value() any
}

type transform struct {
	name  string
	value any
}

// Name returns the name of the transform.
func (v transform) Name() string {
	return v.name
}

// Value returns the arguments of the transform.
func (v transform) Value() any {
	return v.value
}

// Increment defines an increment transform operation
func Increment(v int) Transform {
	return transform{name: "increment", value: v}
}

// IsTransform reports whether v is a concrete transform value and returns it.
func IsTransform(v any) (t Transform, ok bool) {
	var t1 transform
	t1, ok = v.(transform)
	return t1, ok
}
