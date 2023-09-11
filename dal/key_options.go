package dal

import (
	"context"
	"errors"
	"github.com/strongo/random"
	"reflect"
)

// KeyOption defines contract for key option
type KeyOption = func(*Key) error

func setKeyOptions(key *Key, options ...KeyOption) error {
	for _, o := range options {
		if err := o(key); err != nil {
			return err
		}
	}
	return nil
}

// NewKeyWithOptions creates a new key with an ID
func NewKeyWithOptions(collection string, options ...KeyOption) (key *Key, err error) {
	if collection == "" {
		return nil, errors.New("collection is a required parameter")
	}
	key = &Key{collection: collection}
	if err = setKeyOptions(key, options...); err != nil {
		return nil, err
	}
	return key, err
}

var DefaultRandomStringIDLength = 16

// WithRandomStringID sets ID generator to random string
func WithRandomStringID(options ...randomStringOption) KeyOption {
	var rso randomStringOptions
	for _, setOption := range options {
		setOption(&rso)
	}
	return func(key *Key) error {
		key.IDKind = reflect.String
		var ctx context.Context = nil // intentionally nil as not required by any option
		return WithIDGenerator(ctx, func(_ context.Context, record Record) error {
			length := rso.Length()
			prefix := rso.Prefix()
			key.ID = prefix + random.ID(length)
			return nil
		})(key)
	}
}

//// WithParent sets Parent
//func WithParent[T comparable](collection string, id T, options ...KeyOption) KeyOption {
//	return func(key *Key) (err error) {
//		options = append(options, WithID(id))
//		key.parent, err = NewKeyWithOptions(collection, options...)
//		return err
//	}
//}

// WithParentKey sets Parent key
func WithParentKey(parent *Key) KeyOption {
	if parent == nil {
		panic("parent == nil")
	}
	return func(key *Key) error {
		key.parent = parent
		return nil
	}
}

// WithStringID sets ID as a predefined string
func WithStringID(id string) KeyOption {
	return WithKeyID(id)
}

// WithIntID sets ID as a predefined int
func WithIntID(id int) KeyOption {
	return WithKeyID(id)
}

// WithKeyID sets ID as a predefined value. It's advised to use WithIntID and WithStringID when possible.
func WithKeyID[T comparable](id T) KeyOption {
	return func(key *Key) error {
		key.ID = id
		return nil
	}
}
