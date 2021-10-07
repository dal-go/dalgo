package orm

import (
	"github.com/strongo/dalgo/query"
)

// Field defines field
type Field interface {
	Name() string
	Type() string
	Required() bool
	CompareTo(operator query.Operator, v query.Expression) query.Condition
}

// StringField defines a string field
type StringField interface {
	Field
	EqualToString(s string) query.Condition
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

func (v stringField) EqualToString(s string) query.Condition {
	return v.CompareTo(query.Equal, query.String(s))
}

func (v stringField) CompareTo(operator query.Operator, expression query.Expression) query.Condition {
	return query.NewComparison(operator, nil, expression)
}

// NewStringField defines a new string field
func NewStringField(name string) StringField {
	return stringField{field: field{name: name}}
}
