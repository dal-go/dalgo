package dalgo

type Transform interface {
	Name() string
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

func Increment(v int) transform {
	return transform{name: "increment", value: v}
}
