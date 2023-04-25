package orm

import (
	"github.com/dal-go/dalgo/dal"
)

// Field defines field
type Field interface {
	Name() string
	Type() string
	IsRequired() bool
	CompareTo(operator dal.Operator, v dal.Expression) dal.Condition
}

// StringField defines a string field
type StringField interface {
	Field
	EqualToString(s string) dal.Condition
}

type FieldDefinition[T any] struct {
	name       string
	valueType  string
	isRequired bool
	defaultVal T
}

func (v FieldDefinition[T]) DefaultValue() T {
	return v.defaultVal
}

func (v FieldDefinition[T]) Name() string {
	return v.name
}

func (v FieldDefinition[T]) IsRequired() bool {
	return v.isRequired
}

func (v FieldDefinition[T]) Type() string {
	return v.valueType
}

func (v FieldDefinition[T]) CompareTo(operator dal.Operator, expression dal.Expression) dal.Condition {
	return dal.NewComparison(dal.FieldRef{Name: v.name}, operator, expression)
}

func (v FieldDefinition[T]) EqualTo(value T) dal.Condition {
	return v.CompareTo(dal.Equal, dal.Constant{Value: value})
}

func NewField[T any](name string, options ...FieldOption[T]) FieldDefinition[T] {
	f := FieldDefinition[T]{name: name}
	for _, o := range options {
		f = o(f)
	}
	return f
}

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
