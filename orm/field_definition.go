package orm

import (
	"reflect"
	"strings"

	"github.com/dal-go/dalgo/dal"
)

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
	return dal.NewComparison(dal.Field(v.name), operator, expression)
}

func (v FieldDefinition[T]) EqualTo(value T) dal.Condition {
	return v.CompareTo(dal.Equal, dal.Constant{Value: value})
}

func NewField[T any](name string, options ...FieldOption[T]) FieldDefinition[T] {
	var v T
	return NewFieldWithType(name, reflect.TypeOf(v).String(), options...)
}

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
