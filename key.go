package dalgo

import (
	"errors"
	"fmt"
	"strings"
)

// Field hold a reference to a single record within a root or nested recordset.
type Field struct {
	Name  string      `json:"name"`
	Value interface{} `json:"value"`
}

func (v Field) Validate() error {
	if strings.TrimSpace(v.Name) == "" {
		return errors.New("name is a required property")
	}
	if v.Value == nil {
		return errors.New("Value is a required property")
	}
	return nil
}

// Key represents a full path to a given record (no parent in case of root recordset)
type Key struct {
	parent *Key
	kind   string
	ID     interface{}
}

//func (v *Key) Child(key *Key) *Key {
//	key.parent = v
//	return key
//}

func (v Key) Level() int {
	if v.parent == nil {
		return 0
	}
	return v.parent.Level() + 1
}

func (v Key) Parent() *Key {
	return v.parent
}

func (v Key) Kind() string {
	return v.kind
}

func (v Key) Validate() error {
	if strings.TrimSpace(v.kind) == "" {
		return errors.New("child must have 'kind'")
	}
	if v.parent != nil {
		return v.parent.Validate()
	}
	if fields, ok := v.ID.([]Field); ok {
		for i, field := range fields {
			if err := field.Validate(); err != nil {
				return fmt.Errorf("child is referencing invalid field # %v: %w", i, err)
			}
		}
	}
	if id, ok := v.ID.(Validatable); ok {
		return id.Validate()
	}
	return nil
}

type KeyOption = func(*Key)

func setKeyOptions(key *Key, options ...KeyOption) {
	for _, setOptions := range options {
		setOptions(key)
	}
}

func NewKeyWithID(kind string, id interface{}, options ...KeyOption) (key *Key) {
	key = &Key{kind: kind, ID: id}
	setKeyOptions(key, options...)
	return
}

func NewKey(kind string, options ...KeyOption) (key *Key) {
	if len(options) == 0 {
		panic("at least 1 child option should be specified")
	}
	key = &Key{
		kind: kind,
	}
	setKeyOptions(key, options...)
	return
}

func WithID(id interface{}) KeyOption {
	return func(key *Key) {
		key.ID = id
	}
}

func WithFields(fields []Field) KeyOption {
	return func(key *Key) {
		key.ID = fields
	}
}

// NewKeyWithStrID create child with a single string ID
func NewKeyWithStrID(kind string, id string, options ...KeyOption) *Key {
	return NewKeyWithID(kind, id, options...)
}

// NewKeyWithIntID create child with a single integer ID
func NewKeyWithIntID(kind string, id int, options ...KeyOption) *Key {
	return NewKeyWithID(kind, id, options...)
}

// NewKeyWithFields creates a new record child from a sequence of record's references
func NewKeyWithFields(kind string, fields ...Field) *Key {
	return &Key{kind: kind, ID: fields}
}
