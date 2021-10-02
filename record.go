package dalgo

import (
	"errors"
	"fmt"
)

var DoesNotExist = errors.New("does not exist")

// Record is a gateway to a database record.
type Record interface {
	// Key keeps a `table` name of an entity and an ID within that table or a chain of nested keys
	Key() *Key

	// Error keeps an error for the last operation on the record. Not found is not treated as an error
	Error() error

	// Exists indicates if record was found in database. Throws panic if called before a `Get` or `Set`.
	Exists() bool

	// SetError sets error relevant to specific record. Intended to be used only by DALgo DB drivers.
	SetError(err error)

	// Data returns record data (without ID/key).
	// Requires either record to be created by NewRecordWithData()
	// or DataTo() to be called first, otherwise panics.
	Data() interface{}

	// SetDataTo sets DataTo handler
	SetDataTo(dataTo func(target interface{}) error)

	// DataTo deserializes record data into a struct. Throws panic if called before `Get`.
	// Uses a handler set by SetDataTo.
	DataTo(target interface{}) error
}

type record struct {
	key    *Key
	err    error
	data   interface{}
	dataTo func(target interface{}) error
}

// Exists returns if records exists. Panics if there was no a `get` operation on a record before.
func (v record) Exists() bool {
	if v.err != nil {
		if IsNotFound(v.err) {
			return false
		}
		panic("an attempt to check for existence a record with an error")
	}
	if v.dataTo == nil {
		panic("tried to check if record exists before receiving the record data")
	}
	return true
}

// Key returns key of a record
func (v record) Key() *Key {
	return v.key
}

func (v record) Data() interface{} {
	if v.err != nil {
		if !IsNotFound(v.err) {
			panic("an attempt to retrieve data from a record with an error")
		}
	}
	return v.data
}

// SetDataTo sets DataTo handler
func (v record) SetDataTo(dataTo func(target interface{}) error) {
	v.dataTo = dataTo
}

// DataTo marshals record data into target
func (v record) DataTo(target interface{}) error {
	if target == nil {
		panic("not possible to marshall data into a nil value")
	}
	if v.dataTo == nil {
		panic(fmt.Sprintf("method DataTo(%T) is called before data retrieval", target))
	}
	if err := v.dataTo(target); err != nil {
		return fmt.Errorf("failed to marshal record data into %T: %w", target, err)
	}
	v.data = target
	return nil
}

// Error returns error associated with a record
func (v record) Error() error {
	if IsNotFound(v.err) {
		return nil
	}
	return v.err
}

// SetError sets error associated with a record
func (v *record) SetError(err error) {
	v.err = err
}

// NewRecord creates a new record
func NewRecord(key *Key) Record {
	return newRecord(key)
}

func newRecord(key *Key) *record {
	if key == nil {
		panic("parameter 'key' is required for dalgo.NewRecord()")
	}
	if err := key.Validate(); err != nil {
		panic(fmt.Errorf("invalid key: %w", err))
	}
	return &record{key: key}
}

// NewRecordWithData creates a new record with a data target struct
func NewRecordWithData(key *Key, data interface{}) Record {
	record := newRecord(key)
	record.data = data
	return record
}
