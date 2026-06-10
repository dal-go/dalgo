package record

import (
	"context"

	"github.com/dal-go/dalgo/dal"
)

// GetWithID reads the record stored at id from collection c and returns it as a
// typed WithID[K] (an alias for dal.RecordWithID[K]). The decoded value of type T
// is reachable via the returned record (Record.Data() is a *T).
//
// Deprecated: use c.GetRecordWithID(ctx, s, id) directly. Now that the
// RecordWithID type and the accessor both live in package dal, this thin
// collection-wrapping forwarder is redundant; it is retained only for backward
// compatibility.
func GetWithID[K comparable, T any](ctx context.Context, c dal.Collection[K, T], s dal.ReadSession, id K) (WithID[K], error) {
	return c.GetRecordWithID(ctx, s, id)
}
