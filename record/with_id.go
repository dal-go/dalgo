package record

import (
	"fmt"

	"github.com/dal-go/dalgo/dal"
)

// WithID is a record with a strongly typed ID
type WithID[K comparable] struct {
	ID     K          `json:"id"`               // Unique id of the record in collection
	FullID string     `json:"fullID,omitempty"` // Custom id of the record fully unique across all DB collections
	Key    *dal.Key   `json:"-"`
	Record dal.Record `json:"-"`
}

// String returns string representation of a record with an ID
func (v WithID[K]) String() string {
	if v.FullID == "" {
		return fmt.Sprintf("{ID=%v, FullID=nil, Key=%v, Record=%v}", v.ID, v.Key, v.Record)
	}
	if id, ok := any(v.ID).(string); ok {
		return fmt.Sprintf(`{ID="%s", FullID="%s", Key=%v, Record=%v}`, id, v.FullID, v.Key, v.Record)
	}
	return fmt.Sprintf(`{ID=%+v, FullID="%s", Key=%v, Record=%v}`, v.ID, v.FullID, v.Key, v.Record)
}

func NewWithID[T comparable](id T, key *dal.Key, data any) WithID[T] {
	return WithID[T]{
		ID:     id,
		Key:    key,
		Record: dal.NewRecordWithData(key, data),
	}
}
