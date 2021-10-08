package dal

import (
	"errors"
	"fmt"
	"strings"
)

// FieldVal hold a reference to a single record within a root or nested recordset.
type FieldVal struct {
	Name  string      `json:"name"`
	Value interface{} `json:"value"`
}

// Validate valodates field value
func (v FieldVal) Validate() error {
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
	parent     *Key
	collection string
	ID         interface{}
}

// String returns string representation of a key instance
func (k Key) String() string {
	key := k
	if err := key.Validate(); err != nil {
		panic(fmt.Sprintf("will not generate path for invalid child: %v", err))
	}
	s := make([]string, 0, (key.Level())*2)
	for {
		s = append(s, fmt.Sprintf("%v", key.ID))
		s = append(s, key.collection)
		if key.parent == nil {
			break
		} else {
			key = *key.parent
		}
	}
	return reverseStringsJoin(s, "/")
}

// CollectionPath return path to parent
func (k *Key) CollectionPath() string {
	key := k
	var s []string
	for {
		if strings.TrimSpace(key.collection) == "" {
			panic("k is referencing an empty kind")
		}
		s = append(s, key.collection)
		if key.parent == nil {
			break
		} else {
			key = key.parent
		}
	}
	return reverseStringsJoin(s, "/")
}

func reverseStringsJoin(elems []string, sep string) string {
	if len(elems) == 0 {
		return ""
	}
	n := len(sep) * (len(elems) - 1)
	for i := 0; i < len(elems); i++ {
		n += len(elems[i])
	}

	var b strings.Builder
	b.Grow(n)
	for i := len(elems) - 1; i >= 0; i-- {
		if _, err := b.WriteString(elems[i]); err != nil {
			panic(err)
		}
		if i > 0 {
			if _, err := b.WriteString(sep); err != nil {
				panic(err)
			}
		}
	}
	return b.String()
}

//func (v *Key) Child(key *Key) *Key {
//	key.parent = v
//	return key
//}

// Level returns level of key (e.g. how many parents it have)
func (k Key) Level() int {
	if k.parent == nil {
		return 0
	}
	return k.parent.Level() + 1
}

// Parent return a reference to the parent key
func (k Key) Parent() *Key {
	return k.parent
}

// Collection returns reference to colection
func (k Key) Collection() string {
	return k.collection
}

// Validate validate key
func (k Key) Validate() error {
	if strings.TrimSpace(k.collection) == "" {
		return errors.New("child must have 'collection'")
	}
	if k.parent != nil {
		return k.parent.Validate()
	}
	if fields, ok := k.ID.([]FieldVal); ok {
		for i, field := range fields {
			if err := field.Validate(); err != nil {
				return fmt.Errorf("child is referencing invalid field # %v: %w", i, err)
			}
		}
	}
	if id, ok := k.ID.(interface{ Validate() error }); ok {
		return id.Validate()
	}
	return nil
}

// KeyOption defines contract for key option
type KeyOption = func(*Key)

func setKeyOptions(key *Key, options ...KeyOption) {
	for _, setOptions := range options {
		setOptions(key)
	}
}

// NewKeyWithID creates a new key with ID
func NewKeyWithID(collection string, id interface{}, options ...KeyOption) (key *Key) {
	key = &Key{collection: collection, ID: id}
	setKeyOptions(key, options...)
	return
}

// NewKey creates a new key
func NewKey(collection string, options ...KeyOption) (key *Key) {
	if len(options) == 0 {
		panic("at least 1 child option should be specified")
	}
	key = &Key{
		collection: collection,
	}
	setKeyOptions(key, options...)
	return
}

// WithID sets ID of a key
func WithID(id interface{}) KeyOption {
	return func(key *Key) {
		key.ID = id
	}
}

// WithFields sets a list of field values as key ID
func WithFields(fields []FieldVal) KeyOption {
	return func(key *Key) {
		key.ID = fields
	}
}

// NewKeyWithStrID create child with a single string ID
func NewKeyWithStrID(collection string, id string, options ...KeyOption) *Key {
	return NewKeyWithID(collection, id, options...)
}

// NewKeyWithIntID create child with a single integer ID
func NewKeyWithIntID(collection string, id int, options ...KeyOption) *Key {
	return NewKeyWithID(collection, id, options...)
}

// NewKeyWithFields creates a new record child from a sequence of record's references
func NewKeyWithFields(collection string, fields ...FieldVal) *Key {
	return &Key{collection: collection, ID: fields}
}
