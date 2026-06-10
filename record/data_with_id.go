package record

import (
	"github.com/dal-go/dalgo/dal"
)

// DataWithID is a backward-compatible alias for dal.RecordWithDataAndID.
//
// The type now lives in package dal so the typed Collection layer can return it
// without a dal -> record import cycle. New code should prefer
// dal.RecordWithDataAndID.
type DataWithID[K comparable, D any] = dal.RecordWithDataAndID[K, D]

// NewDataWithID forwards to dal.NewRecordWithDataAndID, kept for backward
// compatibility.
func NewDataWithID[K comparable, D any](id K, key *dal.Key, data D) DataWithID[K, D] {
	return dal.NewRecordWithDataAndID(id, key, data)
}
