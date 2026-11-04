package dal

// NewExtraField returns an extra field with the given name and value.
func NewExtraField(name string, value any) ExtraField {
	return &field{name: name, value: value}
}

// ExtraField is a named value stored with a record.
type ExtraField interface {
	// Name returns the field name.
	Name() string
	// Value returns the field value.
	Value() any
}

type field struct {
	name  string
	value any
}

// Name returns the field name.
func (f field) Name() string {
	return f.name
}

// Value returns the field value.
func (f field) Value() any {
	return f.value
}
