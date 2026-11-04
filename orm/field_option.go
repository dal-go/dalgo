package orm

// FieldOption changes a FieldDefinition.
type FieldOption[T any] func(f FieldDefinition[T]) FieldDefinition[T]

// Required returns an option that marks the field required.
func Required[T any]() FieldOption[T] {
	return func(f FieldDefinition[T]) FieldDefinition[T] {
		f.isRequired = true
		return f
	}
}

// Default returns an option that sets the field's default value.
func Default[T any](value T) FieldOption[T] {
	return func(f FieldDefinition[T]) FieldDefinition[T] {
		f.defaultVal = value
		return f
	}
}
