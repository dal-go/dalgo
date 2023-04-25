package orm

type FieldOption[T any] func(f FieldDefinition[T]) FieldDefinition[T]

func Required[T any]() FieldOption[T] {
	return func(f FieldDefinition[T]) FieldDefinition[T] {
		f.isRequired = true
		return f
	}
}

func Default[T any](value T) FieldOption[T] {
	return func(f FieldDefinition[T]) FieldDefinition[T] {
		f.defaultVal = value
		return f
	}
}
