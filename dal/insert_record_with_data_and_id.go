package dal

import (
	"context"

	"github.com/dal-go/record"
)

// InsertRecordWithDataAndID inserts the caller-supplied data value at key (under
// id) and returns a typed record.DataWithID[K, D].
//
// It is the write twin of GetRecordWithIDIntoData: data is used as-is (never
// new(T)), so D may be an interface holding a concrete pointer — the factory
// pattern used by frameworks whose model types are interfaces. It is a free
// function because Go forbids type parameters on methods, so the decoupled data
// type D cannot be expressed as a Collection[K, T] method.
//
// data must be a non-nil pointer or interface referencing a struct or a map (see
// record.NewDataWithID, which validates it). On failure it returns the built
// value together with the session Insert error.
func InsertRecordWithDataAndID[K comparable, D any](ctx context.Context, s WriteSession, key *record.Key, id K, data D) (record.DataWithID[K, D], error) {
	rec := record.NewDataWithID(id, key, data)
	return rec, s.Insert(ctx, rec.Record)
}
