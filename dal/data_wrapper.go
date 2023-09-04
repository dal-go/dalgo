package dal

import (
	"context"
	"fmt"
)

// DataWrapper is a wrapper for data transfer objects (DTOs).
// TODO: document intended usage or consider removing as it makes implementation of Reader more complex.
type DataWrapper interface {
	Data() any
}

type RecordBeforeSaveHook interface {
	BeforeSave(ctx context.Context, key *Key) (err error)
}

type RecordAfterLoadHook interface {
	AfterLoad(ctx context.Context, key *Key) (err error)
}

type RecordHook = func(ctx context.Context, record Record) error

type RecordDataHook = func(ctx context.Context, db DB, key *Key, data any) (err error)

type recordData struct {
	data       any
	beforeSave RecordDataHook
	afterLoad  RecordDataHook
}

func (v recordData) Data() any {
	return v.data
}

func (v recordData) String() string {
	return fmt.Sprintf("%v", v.data)
}

func (v recordData) BeforeSave(ctx context.Context, db DB, key *Key) (err error) {
	if v.beforeSave != nil {
		err = v.beforeSave(ctx, db, key, v.data)
	}
	return
}

func (v recordData) AfterLoad(ctx context.Context, db DB, key *Key) (err error) {
	if v.afterLoad != nil {
		err = v.afterLoad(ctx, db, key, v.data)
	}
	return
}

// MakeRecordData creates a DataWrapper with the given data and options.
func MakeRecordData(data any, options ...RecordDataOption) DataWrapper {
	rd := &recordData{
		data: data,
	}
	for _, o := range options {
		o(rd)
	}
	return rd
}

type RecordDataOption = func(rd *recordData)

func WithBeforeSave(hook RecordDataHook) func(rd *recordData) {
	return func(rd *recordData) {
		rd.beforeSave = hook
	}
}

func WithAfterLoad(hook RecordDataHook) func(rd *recordData) {
	return func(rd *recordData) {
		rd.afterLoad = hook
	}
}

//var _ DataWrapper = (*RecordFields)(nil)
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
