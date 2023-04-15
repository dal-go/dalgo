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

type RecordHook = func(c context.Context, record Record) error

type RecordDataHook = func(c context.Context, db Database, key *Key, data any) (err error)

type recordData[T any] struct {
	Data
	beforeSave RecordDataHook
	afterLoad  RecordDataHook
}

func (v recordData[T]) DTO() any {
	return v.Data
}

func (v recordData[T]) String() string {
	return fmt.Sprintf("%v", v.Data)
}

func (v recordData[T]) BeforeSave(c context.Context, db Database, key *Key) (err error) {
	if v.beforeSave != nil {
		err = v.beforeSave(c, db, key, v)
	}
	return
}

func (v recordData[T]) AfterLoad(c context.Context, db Database, key *Key) (err error) {
	if v.afterLoad != nil {
		err = v.afterLoad(c, db, key, v.Data.(T))
	}
	return
}

func MakeRecordData[T any](data T) RecordData {
	return MakeRecordDataWithCallbacks(data, nil, nil)
}

func MakeRecordDataWithCallbacks[T any](
	data T,
	beforeSave RecordDataHook,
	afterLoad RecordDataHook,
) RecordData {
	return &recordData[T]{
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
