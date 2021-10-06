package orm

import (
	"github.com/strongo/dalgo/query"
)

// Field defines field
type Field interface {
	Name() string
	Type() string
}

// StringField defines a string field
type StringField interface {
	Field
	EqualTo(v string) query.Condition
}

type field struct {
	name string
}

func (v field) Name() string {
	return v.name
}

type stringField struct {
	field
}

func (v stringField) Type() string {
	return "string"
}

func (v stringField) EqualTo(s string) query.Condition {
	return nil // TODO: implement
}

// NewStringField defines a new string field
func NewStringField(name string) StringField {
	return stringField{field: field{name: name}}
}
