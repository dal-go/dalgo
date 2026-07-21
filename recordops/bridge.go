// specscore: feat-recordops/diff
package recordops

import (
	"errors"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

// SliceToSeq turns an already-sorted slice into a RecordSeq.
// The slice MUST be sorted ascending by ID; SliceToSeq does NOT sort.
// A nil or empty slice produces a stream that yields zero items.
func SliceToSeq[K comparable](records []record.WithID[K]) RecordSeq[K] {
	return func(yield func(record.WithID[K], error) bool) {
		for _, r := range records {
			if !yield(r, nil) {
				return
			}
		}
	}
}

// ReaderToSeq adapts a dalgo dal.RecordsReader to a RecordSeq.
// idOf extracts the ID from each record.Record yielded by the reader.
// Reader errors propagate via the seq2 error channel.
//
// The underlying reader is Closed exactly once when iteration ends —
// whether by exhausting records (dal.ErrNoMoreRecords), by the consumer
// breaking out of the range loop early, or by any upstream stream error.
//
// dal.Reader.Cursor() is NOT surfaced through this bridge in MVP;
// callers needing pagination must drive the reader directly. See
// spec/ideas/dal-records-reader-iter-seq.md.
func ReaderToSeq[K comparable](r dal.RecordsReader, idOf func(record.Record) (K, error)) RecordSeq[K] {
	return func(yield func(record.WithID[K], error) bool) {
		defer func() { _ = r.Close() }()
		var zero record.WithID[K]
		for {
			rec, err := r.Next()
			if err != nil {
				if errors.Is(err, dal.ErrNoMoreRecords) {
					return
				}
				yield(zero, err)
				return
			}
			id, err := idOf(rec)
			if err != nil {
				yield(zero, err)
				return
			}
			wid := record.WithID[K]{ID: id, Record: rec}
			if !yield(wid, nil) {
				return
			}
		}
	}
}
