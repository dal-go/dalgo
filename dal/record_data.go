package dal

import (
	"context"
	"fmt"
)

type RecordData interface {
	DTO() any
}

type RecordBeforeSaveHook interface {
	BeforeSave(c context.Context, key *Key) (err error)
}

type RecordAfterLoadHook interface {
	AfterLoad(c context.Context, key *Key) (err error)
}

type Data = any

type RecordEvent = func(c context.Context, key *Key, data Data) error

var _ RecordBeforeSaveHook = (*recordMaker[any])(nil)
var _ RecordAfterLoadHook = (*recordMaker[any])(nil)

type recordMaker[T any] struct {
	Data
	beforeSave RecordEvent
	afterLoad  RecordEvent
}

func (v recordMaker[T]) DTO() any {
	return v.Data
}

func (v recordMaker[T]) String() string {
	return fmt.Sprintf("%v", v.Data)
}

func (v recordMaker[T]) BeforeSave(c context.Context, key *Key) (err error) {
	if v.beforeSave != nil {
		err = v.beforeSave(c, key, v.Data.(T))
	}
	return
}

func (v recordMaker[T]) AfterLoad(c context.Context, key *Key) (err error) {
	if v.afterLoad != nil {
		err = v.afterLoad(c, key, v.Data.(T))
	}
	return
}

func MakeRecordData[T any](data T) RecordData {
	return MakeRecordDataWithCallbacks(data, nil, nil)
}

func MakeRecordDataWithCallbacks[T any](
	data T,
	beforeSave RecordEvent,
	afterLoad RecordEvent,
) RecordData {
	return &recordMaker[T]{
		Data:       data,
		beforeSave: beforeSave,
		afterLoad:  afterLoad,
	}
}

//var _ RecordData = (*RecordFields)(nil)
//
//type RecordFields = map[string]any
//
//func (RecordFields) BeforeSave(key *Key) (err error) {
//	return nil
//}
//
//func (RecordFields) AfterLoad(key *Key) (err error) {
//	return nil
//}
