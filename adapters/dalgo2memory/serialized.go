package dalgo2memory

import (
	"encoding/json"
	"fmt"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/update"
)

// serializedEngine is the default storage engine: it stores each record as
// JSON bytes keyed by id, and owns the marshal/unmarshal, duplicate-on-insert,
// not-found, and unknown-field validation that the adapter performed before the
// storage-engine seam. Its formal behavioral contract is owned by the
// serialized-storage Feature; here it is the unselected default.
//
// collection and factory carry the schema context needed for unknown-field
// rejection: factory is nil when the collection has no registered record type
// (schemaless), in which case no field validation is performed.
type serializedEngine struct {
	collection string
	factory    func() any
	records    map[string][]byte
}

var _ storageEngine = (*serializedEngine)(nil)

// newSerializedEngine builds a Serialized engine for a collection with the
// given record-type factory (nil when schemaless).
func newSerializedEngine(collection string, factory func() any) *serializedEngine {
	return &serializedEngine{
		collection: collection,
		factory:    factory,
		records:    make(map[string][]byte),
	}
}

func (e *serializedEngine) exists(id string) bool {
	_, ok := e.records[id]
	return ok
}

func (e *serializedEngine) store(id string, record dal.Record, overwrite bool) error {
	if !overwrite {
		if _, ok := e.records[id]; ok {
			return fmt.Errorf("record already exists: %s", record.Key())
		}
	}
	b, err := json.Marshal(record.Data())
	if err != nil {
		return err
	}
	if e.factory != nil {
		if err := checkUnknownFields(e.collection, e.factory, b); err != nil {
			return err
		}
	}
	e.records[id] = b
	return nil
}

func (e *serializedEngine) load(id string, record dal.Record) error {
	b, ok := e.records[id]
	if !ok {
		return dal.NewErrNotFoundByKey(record.Key(), nil)
	}
	return json.Unmarshal(b, record.Data())
}

func (e *serializedEngine) delete(id string) {
	delete(e.records, id)
}

func (e *serializedEngine) update(id string, updates []update.Update) error {
	b, ok := e.records[id]
	if !ok {
		return dal.NewErrNotFoundByKey(dal.NewKeyWithID(e.collection, id), nil)
	}
	var data map[string]any
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	if err := applyUpdatesToMap(data, updates); err != nil {
		return err
	}
	next, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if e.factory != nil {
		if err := checkUnknownFields(e.collection, e.factory, next); err != nil {
			return err
		}
	}
	e.records[id] = next
	return nil
}

func (e *serializedEngine) rows() ([]engineRow, error) {
	rows := make([]engineRow, 0, len(e.records))
	for id, b := range e.records {
		var data map[string]any
		if err := json.Unmarshal(b, &data); err != nil {
			return nil, err
		}
		raw := b
		rows = append(rows, engineRow{
			id:   id,
			data: data,
			materialize: func(target any) error {
				return json.Unmarshal(raw, target)
			},
		})
	}
	return rows, nil
}
