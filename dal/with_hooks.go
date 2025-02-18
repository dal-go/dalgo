package dal

//import (
//	"context"
//)

//type KeyOperation = func(ctx context.Context, key Key) (err error)
//type GetFunc = func(ctx context.Context, record Record) (err error)
//type SetFunc = func(ctx context.Context, record Record) (err error)
//type DelFunc = func(ctx context.Context, key Key) (err error)
//type UpdateFunc = func(ctx context.Context, key *Key, updates []Update, preconditions ...Precondition) error

//type HookContext struct {
//	context.Context
//	IsInTransaction bool
//}
//
//type BeforeKeyOperationHook = func(ctx HookContext, key *Key) error
//type BeforeKeysOperationHook = func(ctx HookContext, keys []*Key) error
//type BeforeRecordOperationHook = func(ctx HookContext, record Record) error
//type BeforeRecordsOperationHook = func(ctx HookContext, records []Record) error
//
//type AfterKeyOperationHook = func(ctx HookContext, key *Key, err error) error
//type AfterKeysOperationHook = func(ctx HookContext, keys []*Key, err error) error
//type AfterRecordOperationHook = func(ctx HookContext, record Record, err error) error
//type AfterRecordsOperationHook = func(ctx HookContext, records []Record, err error) error
//
//type BeforeUpdateHook = func(ctx HookContext, key *Key, updates []Update, preconditions ...Precondition) error
//
//type Hooks struct {
//	beforeGet BeforeRecordOperationHook
//	afterGet  AfterRecordOperationHook
//
//	beforeGetMulti BeforeRecordsOperationHook
//	afterGetMulti  AfterRecordsOperationHook
//
//	beforeSet BeforeRecordOperationHook
//	afterSet  AfterRecordOperationHook
//
//	beforeSetMulti BeforeRecordsOperationHook
//	afterSetMulti  AfterRecordsOperationHook
//
//	beforeInsert BeforeRecordOperationHook
//	afterInsert  AfterRecordOperationHook
//
//	beforeInsertMulti BeforeRecordsOperationHook
//	afterInsertMulti  AfterRecordsOperationHook
//
//	beforeUpdate BeforeUpdateHook
//	afterUpdate  AfterKeyOperationHook
//
//	beforeDelete BeforeKeyOperationHook
//	afterDelete  AfterKeyOperationHook
//
//	beforeDeleteMulti BeforeKeysOperationHook
//	afterDeleteMulti  AfterKeysOperationHook
//}
//
//func (v *Hooks) BeforeGet() BeforeRecordOperationHook {
//	return v.beforeGet
//}
//
//func (v *Hooks) BeforeGetMulti() BeforeRecordsOperationHook {
//	return v.beforeGetMulti
//}
//
//func (v *Hooks) AfterGet() AfterRecordOperationHook {
//	return v.afterGet
//}
//
//func (v *Hooks) AfterGetMulti() AfterRecordsOperationHook {
//	return v.afterGetMulti
//}
//
//func (v *Hooks) BeforeInsert() BeforeRecordOperationHook {
//	return v.beforeInsert
//}
//
//func (v *Hooks) AfterInsert() AfterRecordOperationHook {
//	return v.afterInsert
//}
//
//func (v *Hooks) BeforeInsertMulti() BeforeRecordsOperationHook {
//	return v.beforeInsertMulti
//}
//
//func (v *Hooks) AfterInsertMulti() AfterRecordsOperationHook {
//	return v.afterInsertMulti
//}
//
//func (v *Hooks) BeforeSet() BeforeRecordOperationHook {
//	return v.beforeSet
//}
//
//func (v *Hooks) BeforeSetMulti() BeforeRecordsOperationHook {
//	return v.beforeSetMulti
//}
//
//func (v *Hooks) AfterSet() AfterRecordOperationHook {
//	return v.afterSet
//}
//
//func (v *Hooks) AfterSetMulti() AfterRecordsOperationHook {
//	return v.afterSetMulti
//}
//
//func (v *Hooks) BeforeUpdate() BeforeUpdateHook {
//	return v.beforeUpdate
//}
//
//func (v *Hooks) AfterUpdate() AfterKeyOperationHook {
//	return v.afterUpdate
//}
//
//func (v *Hooks) BeforeDelete() BeforeKeyOperationHook {
//	return v.beforeDelete
//}
//
//func (v *Hooks) BeforeDeleteMulti() BeforeKeysOperationHook {
//	return v.beforeDeleteMulti
//}
//
//func (v *Hooks) AfterDelete() AfterKeyOperationHook {
//	return v.afterDelete
//}
//
//func (v *Hooks) AfterDeleteMulti() AfterKeysOperationHook {
//	return v.afterDeleteMulti
//}
//
//func WithHooks(options ...func(bag *Hooks)) (hooks *Hooks) {
//	hooks = new(Hooks)
//	for _, o := range options {
//		o(hooks)
//	}
//	return
//}
//
//func BeforeGet(hook BeforeRecordOperationHook) func(*Hooks) {
//	return func(hooks *Hooks) {
//		hooks.beforeGet = hook
//	}
//}
//
//func AfterGet(hook AfterRecordOperationHook) func(*Hooks) {
//	return func(hooks *Hooks) {
//		hooks.afterGet = hook
//	}
//}
//
//func BeforeGetMulti(hook BeforeRecordsOperationHook) func(*Hooks) {
//	return func(hooks *Hooks) {
//		hooks.beforeGetMulti = hook
//	}
//}
//
//func AfterGetMulti(hook AfterRecordsOperationHook) func(*Hooks) {
//	return func(hooks *Hooks) {
//		hooks.afterGetMulti = hook
//	}
//}

//func BeforeSet(hook BeforeRecordOperationHook) func(*Hooks) {
//	return func(hooks *Hooks) {
//		hooks.beforeSet = hook
//	}
//}
//
//func AfterSet(hook AfterRecordOperationHook) func(*Hooks) {
//	return func(hooks *Hooks) {
//		hooks.afterSet = hook
//	}
//}
//
//func BeforeInsert(hook BeforeRecordOperationHook) func(*Hooks) {
//	return func(hooks *Hooks) {
//		hooks.beforeInsert = hook
//	}
//}
//
//func AfterInsert(hook AfterRecordOperationHook) func(*Hooks) {
//	return func(hooks *Hooks) {
//		hooks.afterInsert = hook
//	}
//}
//
//func BeforeUpdate(hook BeforeUpdateHook) func(*Hooks) {
//	return func(hooks *Hooks) {
//		hooks.beforeUpdate = hook
//	}
//}
//
//func AfterUpdate(hook AfterKeyOperationHook) func(*Hooks) {
//	return func(hooks *Hooks) {
//		hooks.afterUpdate = hook
//	}
//}
//
//func BeforeDelete(hook BeforeKeyOperationHook) func(*Hooks) {
//	return func(hooks *Hooks) {
//		hooks.beforeDelete = hook
//	}
//}
//
//func AfterDelete(hook AfterKeyOperationHook) func(*Hooks) {
//	return func(hooks *Hooks) {
//		hooks.afterDelete = hook
//	}
//}
