package dalgo

import (
	"errors"
	"fmt"
	"time"
)

var doesNotExist = errors.New("does not exist")

func DoesNotExist() error {
	return doesNotExist
}

// Record is an interface a struct should satisfy to comply with "strongo/db" library
type Record interface {
	Key() *Key          // defines `table` name of the entity
	Data() interface{}  // value to be stored/retrieved (without ID)
	Validate() error    // validate record
	Error() error       // holds error for the record
	SetError(err error) // sets error relevant to specific record
	IsReceived() bool   // indicates if an attempt to retrieve a record has been peformed
	Exists() bool       // indicates if the record exists in DB
}

type record struct {
	key        *Key
	data       interface{}
	receivedAt time.Time
	err        error
}

func (v record) IsReceived() bool {
	return !v.receivedAt.IsZero()
}

func (v record) Exists() bool {
	if v.err != nil {
		if IsNotFound(v.err) {
			return false
		}
		panic("an attempt to check for existence a record with an error")
	}
	if v.receivedAt.IsZero() {
		panic("tried to check exists before receiving the record data")
	}
	return true
}

func (v record) Key() *Key {
	return v.key
}

func (v record) Data() interface{} {
	return v.data
}

//func (v *record) SetData(data interface{}) {
//	v.data = data
//}

func (v record) Error() error {
	return v.err
}

func (v *record) SetError(err error) {
	v.err = err
	if err == nil {
		v.receivedAt = time.Now()
	}
}

func (v record) Validate() error {
	if err := v.key.Validate(); err != nil {
		return fmt.Errorf("invalid record child: %w", err)
	}
	if data, ok := v.data.(Validatable); ok {
		if err := data.Validate(); err != nil {
			return fmt.Errorf("invalid record data: %v", err)
		}
	}
	return nil
}

func NewRecord(key *Key, data interface{}) Record {
	if key == nil {
		panic("parameter 'key' is required for dalgo.NewRecord()")
	}
	if data == nil {
		panic("parameter 'data' is required for dalgo.NewRecord()")
	}
	if err := key.Validate(); err != nil {
		panic(fmt.Errorf("invalid key: %w", err))
	}
	return &record{key: key, data: data}
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
