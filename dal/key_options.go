package dal

import (
	"context"
	"reflect"

	"github.com/dal-go/record"
	"github.com/strongo/random"
)

var DefaultRandomStringIDLength = 16

// WithRandomStringID sets ID generator to random string
func WithRandomStringID(options ...randomStringOption) record.KeyOption {
	var rso randomStringOptions
	for _, setOption := range options {
		setOption(&rso)
	}
	return func(key *record.Key) error {
		key.IDKind = reflect.String
		var ctx context.Context = nil // intentionally nil as isn't required by any option
		return WithIDGenerator(ctx, func(_ context.Context, record record.Record) error {
			length := rso.Length()
			prefix := rso.Prefix()
			key.ID = prefix + random.ID(length)
			return nil
		})(key)
	}
}

//// WithParent sets Parent
//func WithParent[T comparable](recordsetSource string, id T, options ...KeyOption) record.KeyOption {
//	return func(key *record.Key) (err error) {
//		options = append(options, WithID(id))
//		key.parent, err = record.NewKeyWithOptions(recordsetSource, options...)
//		return err
//	}
//}
