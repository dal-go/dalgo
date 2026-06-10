package dal

import (
	"fmt"
	"reflect"
)

// RecordWithDataAndID is a RecordWithID plus strongly typed Data.
//
// It lives in package dal (formerly record.DataWithID) so the typed Collection
// layer can expose it without a dal -> record import cycle. record.DataWithID is
// kept as a backward-compatible alias.
type RecordWithDataAndID[K comparable, D any] struct {
	RecordWithID[K]
	Data D // we can't use *D here as consumer might want to pass an interface value instead of a pointer
}

// NewRecordWithDataAndID creates a RecordWithDataAndID. It panics unless data is
// a non-nil pointer or interface referencing a struct or a map.
func NewRecordWithDataAndID[K comparable, D any](id K, key *Key, data D) RecordWithDataAndID[K, D] {
	if key == nil {
		panic(fmt.Sprintf("key is nil for (id=%v)", id))
	}
	v := reflect.ValueOf(data)
	kind := v.Kind()
	switch kind {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			t := reflect.TypeOf(data)
			panic(fmt.Sprintf("data of type %v is nil for (id=%v, key=%v)", t.String(), id, key))
		}
		elemType := v.Elem().Type()
		switch elemType.Kind() {
		case reflect.Struct, reflect.Map:
			// OK - expected types
		default:
			panic("data should be a pointer to a struct or map, got " + elemType.String())
		}
	default:
		t := reflect.TypeOf(data)
		if t == nil {
			panic(fmt.Sprintf("data is nil for (id=%v, key=%v)", id, key))
		}
		panic(fmt.Sprintf("data should be a pointer or an interface, got %v for (id=%v, key=%v)", t.String(), id, key))
	}
	return RecordWithDataAndID[K, D]{
		RecordWithID: NewRecordWithID(id, key, data),
		Data:         data,
	}
}
