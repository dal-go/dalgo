package dalgo2memory

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Option configures an in-memory database created by NewDB.
type Option func(*database)

// collectionDef describes a single collection in an in-memory schema.
// It is produced by WithCollection and consumed by WithSchema.
type collectionDef struct {
	name      string
	newRecord func() any
}

// WithCollection registers a collection backed by the concrete record type T.
//
// If newRecord is nil, a zero value (new(T)) is used to materialize each record
// read by a query. Provide a factory to populate default field values instead.
func WithCollection[T any](name string, newRecord func() *T) collectionDef {
	factory := func() any {
		if newRecord != nil {
			return newRecord()
		}
		return new(T)
	}
	return collectionDef{name: name, newRecord: factory}
}

// WithSchema registers per-collection record types so that queries return records
// populated into the concrete Go type of the collection.
//
// allowUndefinedCollections controls what happens when a query targets a collection
// that is not part of the schema: when false (the default intent) such a query
// returns an error; when true it falls back to the schemaless behavior
// (map[string]any / keys-only records).
func WithSchema(allowUndefinedCollections bool, collections ...collectionDef) Option {
	return func(db *database) {
		factories := make(map[string]func() any, len(collections))
		for _, c := range collections {
			factories[c.name] = c.newRecord
		}
		db.schema = &memorySchema{
			collections:    factories,
			allowUndefined: allowUndefinedCollections,
		}
	}
}

// memorySchema holds the registered record factories for the in-memory database.
type memorySchema struct {
	collections    map[string]func() any
	allowUndefined bool
}

// recordFactory returns the factory for a collection.
//
// It returns (nil, nil) when no schema is registered, or when the collection is
// undefined but undefined collections are allowed. It returns an error when a
// schema is registered, the collection is undefined, and undefined collections
// are not allowed.
func (db *database) recordFactory(collection string) (func() any, error) {
	if db.schema == nil {
		return nil, nil
	}
	if factory, ok := db.schema.collections[collection]; ok {
		return factory, nil
	}
	if db.schema.allowUndefined {
		return nil, nil
	}
	return nil, fmt.Errorf("collection %q is not defined in the schema", collection)
}

// guardCollection returns an error if a schema is registered and the collection
// is undefined while undefined collections are not allowed.
func (db *database) guardCollection(collection string) error {
	_, err := db.recordFactory(collection)
	return err
}

// guardFields validates that the marshaled record data contains no fields that
// are undefined in the collection's schema type. It also enforces the collection
// guard. It is a no-op when no schema is registered or the collection is an
// allowed undefined collection.
func (db *database) guardFields(collection string, marshaled []byte) error {
	factory, err := db.recordFactory(collection)
	if err != nil {
		return err
	}
	if factory == nil {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(marshaled))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(factory()); err != nil {
		return fmt.Errorf("record for collection %q has fields not defined in the schema: %w", collection, err)
	}
	return nil
}
