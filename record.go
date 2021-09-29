package dalgo

import (
	"errors"
	"fmt"
)

var doesNotExist = errors.New("does not exist")

func DoesNotExist() error {
	return doesNotExist
}

// Record is a gateway to a database record.
type Record interface {
	// Key keeps a `table` name of an entity and an ID within that table or a chain of nested keys
	Key() *Key

	// Error keeps an error for the last operation on the record. Not found is not treated as an error
	Error() error

	// Exists indicates if record was found in database. Throws panic if called before a `Get` or `Set`.
	Exists() bool // indicates if the record exists in DB

	// SetError sets error relevant to specific record. Intended to be used only by DALgo DB drivers.
	SetError(err error)

	// Data returns record data (without ID/key).
	// Requires either DataTo() or NewRecordWithData() to be called first, otherwise panics.
	Data() interface{}

	// DataTo deserializes record data into a struct. Throws panic if called before `Get`.
	DataTo(target interface{}) error //
}

type record struct {
	key    *Key
	err    error
	data   interface{}
	dataTo func(target interface{}) error
}

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

func (v record) Key() *Key {
	return v.key
}

func (v record) Data() interface{} {
	if v.err != nil {
		if !IsNotFound(v.err) {
			panic("an attempt to retrieve data from a record with an error")
		}
	}
	if v.data != nil {
		return v.data
	}
	panic("an attempt to retrieve data before record got retrieved")
}

func (v record) DataTo(target interface{}) error {
	if target == nil {
		panic("not possible to marshall data into a nil value")
	}
	if err := v.dataTo(target); err != nil {
		return err
	}
	v.data = target
	return nil
}

//func (v *record) SetData(data interface{}) {
//	v.data = data
//}

func (v record) Error() error {
	if IsNotFound(v.err) {
		return nil
	}
	return v.err
}

func (v *record) SetError(err error) {
	v.err = err
}

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

func NewRecordWithData(key *Key, data interface{}) Record {
	record := newRecord(key)
	record.data = data
	return record
}

type RecordWithIntID interface {
	Record
	GetID() int64
	SetIntID(id int64)
}

type RecordWithStrID interface {
	Record
	GetID() string
	SetStrID(id string)
}
