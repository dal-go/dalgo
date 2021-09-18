package dalgo

import (
	"fmt"
)

// Record is an interface a struct should satisfy to comply with "strongo/db" library
type Record interface {
	Key() *Key
	Data() interface{}
	SetData(data interface{})
	Validate() error
	Error() error
	SetError(err error)
}

type record struct {
	key  *Key
	data interface{}
	err  error
}

func (v record) Key() *Key {
	return v.key
}

func (v record) Data() interface{} {
	return v.data
}

func (v *record) SetData(data interface{}) {
	v.data = data
}

func (v record) Error() error {
	return v.err
}

func (v *record) SetError(err error) {
	v.err = err
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
