package dal

// FieldName represents a field name as string (for backward compatibility)
type FieldName string

// String implements Expression interface
func (f FieldName) String() string {
	return string(f)
}

// Field creates a FieldRef with the given name
func Field(name string) FieldRef {
	return NewFieldRef(name)
}
