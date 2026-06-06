package dalgo2memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

// Option configures an in-memory database created by NewDB.
type Option func(*database)

// collectionDef describes a single collection in an in-memory schema.
// It is produced by WithCollection and consumed by WithSchema.
type collectionDef struct {
	name      string
	newRecord func() any
	// newEngine builds the storage engine backing the collection. It is set by
	// a CollectionOption (default: Serialized) and consumed by WithSchema.
	newEngine engineFactory
}

// engineFactory builds a storage engine for a collection given its name, its
// record-type factory (nil when schemaless), and the schema-wide ref-breaking
// default (faithful unless WithoutSchemaRefBreaking was used). An engine that
// has no per-collection fidelity setting honors schemaRefBreaking; the
// Serialized engine ignores it (it is always faithful).
type engineFactory func(collection string, factory func() any, schemaRefBreaking bool) storageEngine

// CollectionOption configures a collection definition produced by WithCollection
// — currently the per-collection storage-engine selection. Pass it as a trailing
// argument to WithCollection.
type CollectionOption func(*collectionDef)

// WithSerializedStorage selects the Serialized storage engine for a collection.
// It is the default engine, so this option states the default explicitly; an
// option-less collection behaves identically.
func WithSerializedStorage() CollectionOption {
	return func(def *collectionDef) {
		def.newEngine = serializedEngineFactory
	}
}

// ColumnOption configures a single aspect of a columnar collection: it either
// supplies a ColumnStrategy for a named column (WithColumnStrategy) or sets the
// per-collection ref-breaking override (WithColumnarRefBreaking). It is passed
// to WithColumnarStorage. Exported so an out-of-core package can return one
// carrying its own ColumnStrategy without dalgo2memory importing it.
type ColumnOption func(*columnarConfig)

// WithColumnStrategy supplies a ColumnStrategy for the named column of a
// columnar collection. Columns without an explicit strategy use the default
// typed-slice strategy.
func WithColumnStrategy(name string, strategy ColumnStrategy) ColumnOption {
	return func(cfg *columnarConfig) {
		if cfg.strategies == nil {
			cfg.strategies = make(map[string]ColumnStrategy)
		}
		cfg.strategies[name] = strategy
	}
}

// WithDeclaredColumn declares a columnar column by name for a map-backed
// (map[string]any) collection, stored in a strongly-typed []T slice. At least
// one declared column is required to select columnar storage for a map-backed
// collection; undeclared fields are kept in a parallel leftover map. On a
// struct collection a declared column is accepted but redundant (the struct
// path reflects over the record type instead). When the same name is declared
// more than once, the last declaration wins.
func WithDeclaredColumn[T any](name string) ColumnOption {
	return func(cfg *columnarConfig) {
		var zero T
		cfg.declared = append(cfg.declared, declaredColumn{
			name:     name,
			elemType: reflect.TypeOf(&zero).Elem(),
		})
	}
}

// WithColumnarRefBreaking sets the per-collection ref-breaking override for a
// columnar collection, taking precedence over the schema-wide default
// (WithoutSchemaRefBreaking). Pass true to force faithful storage, false to
// store reference-bearing columns without the serialization round-trip.
func WithColumnarRefBreaking(refBreaking bool) ColumnOption {
	return func(cfg *columnarConfig) {
		cfg.refBreakOver = &refBreaking
	}
}

// WithColumnarStorage selects the columnar storage engine for a
// schema-registered WithCollection[T] collection, with optional per-column
// strategies and a per-collection ref-breaking override. Selecting columnar
// storage for a schemaless or non-struct collection fails with a descriptive
// error when the collection is used.
func WithColumnarStorage(opts ...ColumnOption) CollectionOption {
	return func(def *collectionDef) {
		var cfg columnarConfig
		for _, opt := range opts {
			opt(&cfg)
		}
		def.newEngine = newColumnarEngineFactory(cfg)
	}
}

// serializedEngineFactory is the engineFactory for the default Serialized engine.
// The Serialized engine is always faithful, so it ignores schemaRefBreaking.
func serializedEngineFactory(collection string, factory func() any, _ bool) storageEngine {
	return newSerializedEngine(collection, factory)
}

// WithCollection registers a collection backed by the concrete record type T.
//
// If newRecord is nil, a zero value (new(T)) is used to materialize each record
// read by a query. Provide a factory to populate default field values instead.
//
// Trailing CollectionOption arguments select a per-collection storage engine;
// with none, the collection uses the default Serialized engine.
func WithCollection[T any](name string, newRecord func() *T, opts ...CollectionOption) collectionDef {
	factory := func() any {
		if newRecord != nil {
			return newRecord()
		}
		return new(T)
	}
	def := collectionDef{name: name, newRecord: factory}
	for _, opt := range opts {
		opt(&def)
	}
	return def
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
		engines := make(map[string]engineFactory, len(collections))
		for _, c := range collections {
			factories[c.name] = c.newRecord
			if c.newEngine != nil {
				engines[c.name] = c.newEngine
			}
		}
		db.schema = &memorySchema{
			collections:    factories,
			engines:        engines,
			allowUndefined: allowUndefinedCollections,
		}
	}
}

// memorySchema holds the registered record factories and per-collection engine
// choices for the in-memory database.
type memorySchema struct {
	collections    map[string]func() any
	engines        map[string]engineFactory
	allowUndefined bool
}

// WithoutSchemaRefBreaking disables columnar ref-breaking schema-wide: columnar
// collections store reference-bearing column values without the serialization
// round-trip unless a collection re-enables it (see WithColumnarRefBreaking).
// It has no effect on the always-faithful Serialized engine. The default is
// faithful (ref-breaking on).
func WithoutSchemaRefBreaking() Option {
	return func(db *database) {
		db.schemaRefBreaking = false
	}
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

// engine resolves the storage engine for a collection, constructing it lazily
// on first access and registering it. The engine choice comes from the
// collection's registered CollectionOption when present; any collection without
// a recorded choice (unregistered, or registered without an engine option)
// resolves to the default Serialized engine. The record-type factory (for
// unknown-field validation) is resolved alongside; callers that need the guard
// error should call recordFactory/guardCollection first.
func (db *database) engine(collection string) storageEngine {
	if eng, ok := db.collections[collection]; ok {
		return eng
	}
	factory, _ := db.recordFactory(collection)
	newEngine := serializedEngineFactory
	if db.schema != nil {
		if chosen, ok := db.schema.engines[collection]; ok {
			newEngine = chosen
		}
	}
	eng := newEngine(collection, factory, db.schemaRefBreaking)
	db.collections[collection] = eng
	return eng
}

// checkUnknownFields validates that the marshaled record data contains no fields
// that are undefined in the collection's schema type. Callers pass the factory
// already resolved via recordFactory, and only call this when factory is not nil.
func checkUnknownFields(collection string, factory func() any, marshaled []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(marshaled))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(factory()); err != nil {
		return fmt.Errorf("record for collection %q does not conform to the schema: %w", collection, err)
	}
	return nil
}
