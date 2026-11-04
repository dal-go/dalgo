package dal

import (
	"context"
	"fmt"

	"github.com/dal-go/record"
)

// DataWrapper is a wrapper for data transfer objects (DTOs).
// TODO: document intended usage or consider removing as it makes implementation of RecordsReader more complex.
type DataWrapper interface {
	// Data returns the wrapped data.
	Data() any
}

// RecordBeforeSaveHook is data that can run before a record is saved.
type RecordBeforeSaveHook interface {
	// BeforeSave runs before a save of the record at key.
	BeforeSave(ctx context.Context, key *record.Key) (err error)
}

// RecordAfterLoadHook is data that can run after a record is loaded.
type RecordAfterLoadHook interface {
	// AfterLoad runs after a load of the record at key.
	AfterLoad(ctx context.Context, key *record.Key) (err error)
}

// RecordHook is called with a record. The write pipeline uses it for before-save and before-delete hooks.
type RecordHook = func(ctx context.Context, record record.Record) error

// RecordDataHook is called with a database, key, and data value.
// MakeRecordData stores it as a before-save or after-load hook.
type RecordDataHook = func(ctx context.Context, db DB, key *record.Key, data any) (err error)

type recordData struct {
	data       any
	beforeSave RecordDataHook
	afterLoad  RecordDataHook
}

// Data returns the wrapped data.
func (v recordData) Data() any {
	return v.data
}

func (v recordData) String() string {
	return fmt.Sprintf("%v", v.data)
}

// BeforeSave calls the before-save hook with db, key, and the wrapped data when a hook is set.
func (v recordData) BeforeSave(ctx context.Context, db DB, key *record.Key) (err error) {
	if v.beforeSave != nil {
		err = v.beforeSave(ctx, db, key, v.data)
	}
	return
}

// AfterLoad calls the after-load hook with db, key, and the wrapped data when a hook is set.
func (v recordData) AfterLoad(ctx context.Context, db DB, key *record.Key) (err error) {
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

// RecordDataOption configures a recordData value built by MakeRecordData.
type RecordDataOption = func(rd *recordData)

// WithBeforeSave returns an option that sets the before-save hook.
func WithBeforeSave(hook RecordDataHook) func(rd *recordData) {
	return func(rd *recordData) {
		rd.beforeSave = hook
	}
}

// WithAfterLoad returns an option that sets the after-load hook.
func WithAfterLoad(hook RecordDataHook) func(rd *recordData) {
	return func(rd *recordData) {
		rd.afterLoad = hook
	}
}

//var _ DataWrapper = (*RecordFields)(nil)
//
//type RecordFields = map[string]any
//
//func (RecordFields) BeforeSave(key *record.Key) (err error) {
//	return nil
//}
//
//func (RecordFields) AfterLoad(key *record.Key) (err error) {
//	return nil
//}
