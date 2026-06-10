package dal

import "fmt"

// RecordWithID is a record with a strongly typed ID.
//
// It lives in package dal (formerly record.WithID) so that the typed Collection
// layer can expose it without a dal -> record import cycle. record.WithID is
// kept as a backward-compatible alias.
type RecordWithID[K comparable] struct {
	ID     K      `json:"id"`               // Unique id of the record in collection
	FullID string `json:"fullID,omitempty"` // Custom id of the record fully unique across all DB collections
	Key    *Key   `json:"-"`
	Record Record `json:"-"`
}

// String returns string representation of a record with an ID
func (v RecordWithID[K]) String() string {
	if v.FullID == "" {
		return fmt.Sprintf("{ID=%v, FullID=nil, Key=%v, Record=%v}", v.ID, v.Key, v.Record)
	}
	if id, ok := any(v.ID).(string); ok {
		return fmt.Sprintf(`{ID="%s", FullID="%s", Key=%v, Record=%v}`, id, v.FullID, v.Key, v.Record)
	}
	return fmt.Sprintf(`{ID=%+v, FullID="%s", Key=%v, Record=%v}`, v.ID, v.FullID, v.Key, v.Record)
}

// NewRecordWithID creates a RecordWithID wrapping a Record built from key + data.
func NewRecordWithID[T comparable](id T, key *Key, data any) RecordWithID[T] {
	return RecordWithID[T]{
		ID:     id,
		Key:    key,
		Record: NewRecordWithData(key, data),
	}
}
