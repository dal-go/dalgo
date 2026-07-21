package dal

import "github.com/dal-go/record"

func NewSchema(keyToField KeyToFieldsFunc, dataToKey DataToKeyFunc) Schema {
	return &SchemaBase{keyToFields: keyToField, dataToKey: dataToKey}
}

// KeyToFieldsFunc takes key and should either populate fields on a `data` struct
// or return extra fields to be stored to the target table.
type KeyToFieldsFunc func(key *record.Key, data any) (fields []ExtraField, err error)

// DataToKeyFunc takes data retrieved from DB table/view/query and maps primary key columns to the record key.
type DataToKeyFunc func(incompleteKey *record.Key, data any) (key *record.Key, err error)

type Schema interface {
	DataToKey(incompleteKey *record.Key, data any) (key *record.Key, err error)
	KeyToFields(key *record.Key, data any) (fields []ExtraField, err error)
}

// SchemaBase provides rules for mapping of fields, keys and collections
type SchemaBase struct {
	keyToFields KeyToFieldsFunc
	dataToKey   DataToKeyFunc
}

// DataToKey creates a *record.Key from data read from DB
func (s *SchemaBase) DataToKey(incompleteKey *record.Key, data any) (key *record.Key, err error) {
	return s.dataToKey(incompleteKey, data)
}

// KeyToFields maps key intoRecord DB fields.
// This is needed as relational DBs usually have key column(s) that are part of the record set,
// while key-value DBs can have key and data separated and data would not include the key.
func (s *SchemaBase) KeyToFields(key *record.Key, data any) (fields []ExtraField, err error) {
	if s.keyToFields == nil {
		return nil, nil
	}
	return s.keyToFields(key, data)
}
