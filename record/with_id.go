package record

import (
	"github.com/dal-go/dalgo/dal"
)

// WithID is a backward-compatible alias for dal.RecordWithID.
//
// The type now lives in package dal so the typed Collection layer can return it
// without a dal -> record import cycle. New code should prefer dal.RecordWithID.
type WithID[K comparable] = dal.RecordWithID[K]

// NewWithID forwards to dal.NewRecordWithID, kept for backward compatibility.
func NewWithID[T comparable](id T, key *dal.Key, data any) WithID[T] {
	return dal.NewRecordWithID(id, key, data)
}
