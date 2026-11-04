package orm

import (
	"reflect"
	"strings"

	"github.com/dal-go/dalgo/dal"
)

// FieldDefinition is a named field of type T, with a required flag and a default value.
type FieldDefinition[T any] struct {
	name       string
	valueType  string
	isRequired bool
	defaultVal T
}

// DefaultValue returns the field's default value.
func (v FieldDefinition[T]) DefaultValue() T {
	return v.defaultVal
}

// Name returns the field name.
func (v FieldDefinition[T]) Name() string {
	return v.name
}

// IsRequired reports whether the field is required.
func (v FieldDefinition[T]) IsRequired() bool {
	return v.isRequired
}

// Type returns the field's value type name.
func (v FieldDefinition[T]) Type() string {
	return v.valueType
}

// CompareTo returns a comparison of this field with expression using operator.
func (v FieldDefinition[T]) CompareTo(operator dal.Operator, expression dal.Expression) dal.Condition {
	return dal.NewComparison(dal.Field(v.name), operator, expression)
}

// EqualTo returns an equality comparison of this field with value.
func (v FieldDefinition[T]) EqualTo(value T) dal.Condition {
	return v.CompareTo(dal.Equal, dal.Constant{Value: value})
}

// NewField returns a field named name whose value type is the string form of T, with options applied.
func NewField[T any](name string, options ...FieldOption[T]) FieldDefinition[T] {
	var v T
	return NewFieldWithType(name, reflect.TypeOf(v).String(), options...)
}

// NewFieldWithType returns a field named name with value type valueType, after applying options.
// It panics when name is empty, or when T's kind name is neither "any" nor valueType.
func NewFieldWithType[T any](name, valueType string, options ...FieldOption[T]) FieldDefinition[T] {
	if strings.TrimSpace(name) == "" {
		panic("name cannot be empty")
	}
	var v T
	kind := reflect.TypeOf(v).Kind()
	if kindName := kind.String(); kindName != "any" && kindName != valueType {
		panic("valueType must be " + kind.String())
	}
	f := FieldDefinition[T]{
		name:      name,
		valueType: valueType,
	}
	for _, o := range options {
		f = o(f)
	}
	return f
}
