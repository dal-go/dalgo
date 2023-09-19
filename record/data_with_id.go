package record

import (
	"fmt"
	"github.com/dal-go/dalgo/dal"
)

type DataWithID[K comparable, D any] struct {
	WithID[K]
	Data *D
}

func NewDataWithID[K comparable, D any](id K, key *dal.Key, data *D) DataWithID[K, D] {
	if data == nil {
		panic(fmt.Sprintf("data is nil for (id=%v, key=%v)", id, key))
	}
	return DataWithID[K, D]{
		WithID: NewWithID(id, key, data),
		Data:   data,
	}
}
