package dal

import (
	"context"

	"github.com/dal-go/record"
)

// GetRecordWithIDIntoData fetches the record at key, decoding it INTO the
// caller-supplied data value, and returns a typed record.DataWithID[K, D].
//
// Unlike Collection.GetRecordWithDataAndID (which allocates new(T) and therefore
// needs a concrete T), this takes the data value from the caller, so D may be an
// interface holding a concrete pointer (the factory pattern used by frameworks
// whose model types are interfaces). It is a free function because Go forbids
// type parameters on methods, so the decoupled data type D cannot be expressed
// as a Collection[K, T] method.
//
// data must be a non-nil pointer or interface referencing a struct or a map (see
// record.NewDataWithID, which validates it). On not-found it returns the built
// value together with the session's not-found error.
func GetRecordWithIDIntoData[K comparable, D any](ctx context.Context, s ReadSession, key *record.Key, id K, data D) (record.DataWithID[K, D], error) {
	rec := record.NewDataWithID(id, key, data)
	return rec, s.Get(ctx, rec.Record)
}
