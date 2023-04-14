package orm

import (
	"github.com/dal-go/dalgo/dal"
)

// Field defines field
type Field interface {
	Name() string
	Type() string
	Required() bool
	CompareTo(operator dal.Operator, v dal.Expression) dal.Condition
}

// StringField defines a string field
type StringField interface {
	Field
	EqualToString(s string) dal.Condition
}

type field struct {
	name     string
	required bool
}

func (v field) Name() string {
	return v.name
}

func (v field) Required() bool {
	return v.required
}

type stringField struct {
	field
}

func (v stringField) Type() string {
	return "string"
}

func (v stringField) EqualToString(s string) dal.Condition {
	return v.CompareTo(dal.Equal, dal.String(s))
}

func (v stringField) CompareTo(operator dal.Operator, expression dal.Expression) dal.Condition {
	return dal.NewComparison(dal.FieldRef{Name: v.name}, operator, expression)
}

// NewStringField defines a new string field
func NewStringField(name string) StringField {
	return stringField{field: field{name: name}}
}
