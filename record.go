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
	Key() *Key
	Data() interface{}
	Validate() error
	Error() error
	SetError(err error)
	IsReceived() bool
	Exists() bool
	// SetData(data interface{})
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
	if v.receivedAt.IsZero() {
		panic("tried to check exists before receiving the record data")
	}
	return v.err == nil
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
