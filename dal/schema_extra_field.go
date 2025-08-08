package dal

func NewExtraField(name string, value any) ExtraField {
	return &field{name: name, value: value}
}

type ExtraField interface {
	Name() string
	Value() any
}

type field struct {
	name  string
	value any
}

func (f field) Name() string {
	return f.name
}

func (f field) Value() any {
	return f.value
}
