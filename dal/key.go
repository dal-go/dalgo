package dal

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// FieldVal hold a reference to a single record within a root or nested recordset.
type FieldVal struct {
	Name  string `json:"Name"`
	Value any    `json:"value"`
}

// Validate validates field value
func (v FieldVal) Validate() error {
	if strings.TrimSpace(v.Name) == "" {
		return errors.New("Name is a required property")
	}
	if v.Value == nil {
		return errors.New("Value is a required property")
	}
	return nil
}

// Key represents a full path to a given record (no Parent in case of root recordset)
type Key struct {
	parent     *Key
	collection string
	ID         any
	IDKind     reflect.Kind
}

var idCharsReplacer = strings.NewReplacer(
	".", "%2E",
	"$", "%24",
	"#", "%23",
	"[", "%5B",
	"]", "%5D",
	"/", "%2F",
)

func EscapeID(id string) string {
	return idCharsReplacer.Replace(id)
}

// String returns string representation of a key instance
func (k *Key) String() string {
	key := k // This is intended as we want to traverse the key ancestors
	if err := key.Validate(); err != nil {
		panic(fmt.Sprintf("will not generate path for invalid child: %v", err))
	}
	s := make([]string, 0, (key.Level())*2)
	for {
		id := fmt.Sprintf("%v", key.ID)
		id = EscapeID(id)
		s = append(s, id)
		s = append(s, key.collection)
		if key.parent == nil {
			break
		} else {
			key = key.parent
		}
	}
	return reverseStringsJoin(s, "/")
}

// CollectionPath return path to Parent
func (k *Key) CollectionPath() string {
	key := k // This is intended as we want to traverse the key ancestors
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
//	key.Parent = v
//	return key
//}

// Level returns level of key (e.g. how many parents it have)
func (k *Key) Level() int {
	if k.parent == nil {
		return 0
	}
	return k.parent.Level() + 1
}

// Parent return a reference to the Parent key
func (k *Key) Parent() *Key {
	return k.parent
}

// Collection returns reference to colection
func (k *Key) Collection() string {
	return k.collection
}

// Validate validate key
func (k *Key) Validate() error {
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
	for _, setOption := range options {
		setOption(key)
	}
}

// NewKeyWithID creates a new key with an ID
func NewKeyWithID[T comparable](collection string, id T, options ...KeyOption) (key *Key) {
	if collection == "" {
		panic("collection is a required parameter")
	}
	key = &Key{collection: collection, ID: id}
	setKeyOptions(key, options...)
	return
}

func NewIncompleteKey(collection string, idKind reflect.Kind, parent *Key) *Key {
	if idKind == reflect.Invalid {
		panic("idKind == reflect.Invalid")
	}
	return &Key{
		parent:     parent,
		collection: collection,
		IDKind:     idKind,
	}
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
func WithID[T comparable](id T) KeyOption {
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

// NewKeyWithFields creates a new record child from a sequence of record's references
func NewKeyWithFields(collection string, fields ...FieldVal) *Key {
	return &Key{collection: collection, ID: fields}
}

func EqualKeys(k1 *Key, k2 *Key) bool {

	k1s := make([]*Key, 0, k1.Level())
	k2s := make([]*Key, 0, k2.Level())

	panicIfCircular := func(key *Key, keys []*Key) {
		for _, k := range keys {
			if EqualKeys(k, key) {
				panic(fmt.Sprintf("circular key: %s=%v", k.collection, k.ID))
			}
		}
	}
	for {
		if k1 == nil && k2 == nil {
			return true
		}
		if k1 == nil || k2 == nil {
			return false
		}
		if k1.Collection() != k2.Collection() {
			return false
		}
		if k1.ID == nil && k2.ID != nil || k2.ID == nil && k1.ID != nil {
			return false
		}
		if k1.ID != k2.ID {
			return false
		}
		k1s = append(k1s, k1)
		k2s = append(k2s, k2)

		k1 = k1.Parent()
		k2 = k2.Parent()

		panicIfCircular(k1, k1s)
		panicIfCircular(k2, k2s)
	}
}

func (k *Key) Equal(key *Key) bool {
	return EqualKeys(k, key)
}
