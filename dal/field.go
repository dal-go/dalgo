package dal

// FieldName represents a field name as string (for backward compatibility)
type FieldName string

// String implements Expression interface
func (f FieldName) String() string {
	return string(f)
}

// Field creates an unqualified FieldRef with the given name (empty source,
// i.e. the single From base recordset). Use NewFieldRef to qualify by source.
func Field(name string) FieldRef {
	return NewFieldRef("", name)
}
