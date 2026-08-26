package dalgo2memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

// Option configures an in-memory database created by NewDB.
type Option func(*database)

// WithNoReadsAfterWritesInTransaction enables Firestore-compatible transaction
// ordering for this in-memory database. In a read-write transaction, every
// read after the first successful write returns ErrReadAfterWriteInTransaction.
//
// Deprecated: this is now NewDB's default behavior, so calling this option is
// redundant on a plain NewDB(). It still works — it re-asserts strict
// ordering, which only matters when combined with an earlier
// WithInterleavedReadsAndWritesInTransaction() in the same option list, since
// options apply in order and the last one wins. New code should omit it and
// rely on the default; existing callers (sneat-co/chessraiders among them)
// are unaffected and may drop the call at their own pace.
func WithNoReadsAfterWritesInTransaction() Option {
	return func(db *database) {
		db.noReadsAfterWritesInTransaction = true
	}
}

// WithInterleavedReadsAndWritesInTransaction opts a database out of the
// default Firestore-compatible transaction-ordering check: with it, a
// read-write transaction may freely read a key after the transaction has
// written (to that key or any other), the same way a SQL database's
// session-local transaction behaves.
//
// Use this when the in-memory database stands in for a backend whose
// transactions genuinely permit interleaving reads and writes — a SQL-style
// adapter such as dalgo2mysql, dalgo2postgres, or dalgo2sqlite, or a test
// double standing in for one of them. Do NOT reach for this just to make a
// failing test pass: if the code under test also runs against Firestore
// (dalgo2firestore) or another backend with the same read-after-write
// restriction, a test failing with ErrReadAfterWriteInTransaction is
// reporting a real ordering bug in the code under test — the same class of
// bug that shipped a production 500 in sneat-core-modules's
// set_user_country, undetected because its unit tests ran against the old
// permissive default. Silencing that signal with this option would hide the
// bug again, not fix it.
func WithInterleavedReadsAndWritesInTransaction() Option {
	return func(db *database) {
		db.noReadsAfterWritesInTransaction = false
	}
}

// WithOptimisticConcurrency selects optimistic-concurrency read-write
// transactions for this in-memory database instead of the default: without
// it, RunReadwriteTransaction takes a whole-database lock for the callback's
// entire duration, so read-write transactions are fully serialized and can
// never actually contend with one another.
//
// That default is exactly right for most tests, but it makes one specific
// class of test lie: a test that claims to prove a real concurrency
// guarantee — "two concurrent claims of a unique slug, exactly one wins", or
// "two concurrent bookings for the last remaining place, one is refused" —
// passes trivially against the default lock, because the two transactions it
// spawns can never actually run at the same time. It proves nothing about a
// database, like Firestore, whose transactions really do contend. Production
// code in this ecosystem relies on exactly this guarantee (see
// sneat-co/bookius's facade4bookius/booking.go, which does a Get-then-Insert
// inside RunReadwriteTransaction for slug uniqueness and for capacity), so a
// test standing in for Firestore needs a mode where the contention is real.
//
// With this option, transactions may run concurrently: each buffers the keys
// it reads and the writes it makes locally, touching no shared storage until
// it commits (when its callback returns nil). At that point it fails with
// ErrTransactionConflict (test with IsTransactionConflict) if another
// transaction has committed a write to any key this one read or wrote since
// it first touched that key — whether this one only read the key or wrote it
// too. Buffering writes rather than applying them immediately is what makes
// the LOSING side of a race get the conflict error specifically, rather than
// some other error that happens to depend on write ordering: see
// optimisticState's doc comment in optimistic.go for the full reasoning.
//
// A query (ExecuteQueryToRecordsReader / ExecuteQueryToRecordsetReader)
// inside such a transaction is supported and participates in the
// transaction's snapshot and conflict detection at COLLECTION granularity:
// the query registers its collection, aborts with ErrTransactionConflict if
// the collection was committed to after this transaction's snapshot, and the
// commit revalidates it — which is what makes phantom inserts conflict
// instead of slipping past the per-key read set (see
// optimisticState.observeCollectionAtSnapshot). Joins are the one refused
// shape (Firestore has none to stay faithful to). Point reads and writes by
// key (Get, Exists, Set, Insert, Update, Delete and their -Multi forms) are
// fully supported.
//
// This is deliberately adapter-local rather than part of dalgotest's shared
// conformance suite: the suite proves record-validation invariants every
// dal.DB adapter can be held to uniformly, but optimistic-concurrency
// contention is not such a capability — a real backend like Firestore or a
// SQL database already has its own genuine transactional contention, and
// forcing every adapter to grow an equivalent option and test hook just to
// stay in the suite would be scope the other adapters never asked for.
//
// The default remains the whole-database lock, so transactions still cannot
// contend unless this option is passed. What the default no longer differs on
// is ATOMICITY: both modes buffer a transaction's writes and apply them only
// once its callback returns nil, so a failed transaction discards its writes
// exactly as Firestore does (see runLockedReadwriteTransaction). Contention is
// therefore the one remaining transactional difference between the two modes,
// alongside queries — which the default mode supports inside a read-write
// transaction and this mode does not.
func WithOptimisticConcurrency() Option {
	return func(db *database) {
		db.optimisticConcurrency = true
	}
}

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
	db.enginesMu.Lock()
	defer db.enginesMu.Unlock()

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
