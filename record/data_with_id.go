package record

import (
	"fmt"
	"reflect"

	"github.com/dal-go/dalgo/dal"
)

type DataWithID[K comparable, D any] struct {
	WithID[K]
	Data D // we can't use *D here as consumer might want to pass an interface value instead of a pointer
}

func NewDataWithID[K comparable, D any](id K, key *dal.Key, data D) DataWithID[K, D] {
	if key == nil {
		panic(fmt.Sprintf("key is nil for (id=%v)", id))
	}
	//if data == nil {
	//	panic(fmt.Sprintf("data is nil for (id=%v, key=%v)", id, key))
	//}
	v := reflect.ValueOf(data)
	kind := v.Kind()
	switch kind {
	case reflect.Ptr, reflect.Interface:
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
	return DataWithID[K, D]{
		WithID: NewWithID(id, key, data),
		Data:   data,
	}
}
