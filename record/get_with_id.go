package record

import (
	"context"

	"github.com/dal-go/dalgo/dal"
)

// GetWithID reads the record stored at id from collection c and returns it as a
// typed WithID[K]. The decoded value of type T is reachable via the returned
// WithID's Record (Record.Data() is a *T).
//
// It lives in package record (not as a Collection[K, T] method) because it
// returns record.WithID[K]: package dal must stay free of any import of the
// record package, so the typed-with-id accessor is provided here as a free
// function over the dal.Collection[K, T] handle.
//
// On not-found it returns the zero WithID[K] together with the not-found error
// from the underlying Get (use dal.IsNotFound to test it).
func GetWithID[K comparable, T any](ctx context.Context, c dal.Collection[K, T], s dal.ReadSession, id K) (WithID[K], error) {
	r, err := c.GetRecord(ctx, s, id)
	if err != nil {
		return WithID[K]{}, err
	}
	return WithID[K]{ID: id, Key: r.Key(), Record: r}, nil
}
