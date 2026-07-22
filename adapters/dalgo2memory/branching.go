package dalgo2memory

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/dal-go/dalgo/branching"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

const (
	branchingProviderName    = "dalgo2memory"
	branchingProviderVersion = "1"
	branchingProviderMode    = "serialized"
)

var checkpointSequence atomic.Uint64

var (
	serializedEngineFactoryPC = reflect.ValueOf(serializedEngineFactory).Pointer()
)

// NewBranchingProvider returns the optional database branching capability for
// dalgo2memory's default serialized storage engine.
//
// Columnar and custom storage engines are intentionally outside this
// capability. Capture reports them as branching.UnsupportedError instead of
// publishing an incomplete checkpoint.
func NewBranchingProvider() branching.Provider {
	return serializedBranchingProvider{}
}

type serializedBranchingProvider struct{}

var _ branching.Provider = serializedBranchingProvider{}

func (serializedBranchingProvider) Capability() branching.Capability {
	return branching.Capability{
		Provider: branchingProviderName,
		Version:  branchingProviderVersion,
		Mode:     branchingProviderMode,
	}
}

func (serializedBranchingProvider) Capture(ctx context.Context, source dal.DB) (branching.Checkpoint, error) {
	if source == nil {
		return nil, branching.ErrNilSourceDB
	}
	db, ok := source.(*database)
	if !ok {
		return nil, &branching.UnsupportedError{
			Provider: branchingProviderName,
			Mode:     fmt.Sprintf("source:%T", source),
			Reason:   "source is not a dalgo2memory database",
		}
	}
	if db == nil {
		return nil, branching.ErrNilSourceDB
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Every data operation acquires db.mu before reaching an engine. Taking the
	// write lock makes record bytes stable, while enginesMu also excludes lazy
	// collection creation by concurrent readers.
	db.mu.Lock()
	defer db.mu.Unlock()
	db.enginesMu.Lock()
	defer db.enginesMu.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateSerializedConfiguration(db); err != nil {
		return nil, err
	}

	snapshot := snapshotDatabase(db)
	generation := fmt.Sprintf("%s-%d", branchingProviderName, checkpointSequence.Add(1))
	return &serializedCheckpoint{generation: generation, snapshot: &snapshot}, nil
}

// validateSerializedConfiguration checks both engines already initialized in
// the database and engines configured in the schema but not initialized yet.
// Sorted collection names make an unsupported error deterministic when several
// collections use deferred engines.
func validateSerializedConfiguration(db *database) error {
	for _, collection := range sortedEngineNames(db.collections) {
		if _, ok := db.collections[collection].(*serializedEngine); !ok {
			return unsupportedEngine(collection, db.collections[collection])
		}
	}
	if db.schema == nil {
		return nil
	}
	for _, collection := range sortedFactoryNames(db.schema.engines) {
		factory := db.schema.engines[collection]
		factoryPC := reflect.ValueOf(factory).Pointer()
		if factoryPC == serializedEngineFactoryPC {
			continue
		}
		// A non-serialized factory is user-supplied configuration. Do not call it
		// merely to classify the unsupported mode: constructors may have side
		// effects or panic. An initialized engine was classified above.
		return unsupportedEngineMode(collection, "non-serialized")
	}
	return nil
}

func sortedEngineNames(engines map[string]storageEngine) []string {
	names := make([]string, 0, len(engines))
	for name := range engines {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedFactoryNames(factories map[string]engineFactory) []string {
	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func unsupportedEngine(collection string, engine storageEngine) error {
	mode := "custom"
	if _, ok := engine.(*columnarEngine); ok {
		mode = "columnar"
	}
	return unsupportedEngineMode(collection, mode)
}

func unsupportedEngineMode(collection, mode string) error {
	return &branching.UnsupportedError{
		Provider: branchingProviderName,
		Mode:     mode,
		Reason:   fmt.Sprintf("collection %q does not use serialized storage", collection),
	}
}

type databaseSnapshot struct {
	collections                     map[string]serializedEngineSnapshot
	schema                          *memorySchema
	noReadsAfterWritesInTransaction bool
	schemaRefBreaking               bool
}

type serializedEngineSnapshot struct {
	collection string
	factory    func() any
	records    map[string][]byte
	keys       map[string]*record.Key
}

func snapshotDatabase(db *database) databaseSnapshot {
	collections := make(map[string]serializedEngineSnapshot, len(db.collections))
	for name, storage := range db.collections {
		engine := storage.(*serializedEngine) // validated while the database is locked
		collections[name] = snapshotSerializedEngine(engine)
	}
	return databaseSnapshot{
		collections:                     collections,
		schema:                          cloneMemorySchema(db.schema),
		noReadsAfterWritesInTransaction: db.noReadsAfterWritesInTransaction,
		schemaRefBreaking:               db.schemaRefBreaking,
	}
}

func snapshotSerializedEngine(engine *serializedEngine) serializedEngineSnapshot {
	records := make(map[string][]byte, len(engine.records))
	for id, data := range engine.records {
		records[id] = append([]byte(nil), data...)
	}
	keys := make(map[string]*record.Key, len(engine.keys))
	for id, key := range engine.keys {
		keys[id] = cloneRecordKey(key)
	}
	return serializedEngineSnapshot{
		collection: engine.collection,
		factory:    engine.factory,
		records:    records,
		keys:       keys,
	}
}

func cloneMemorySchema(schema *memorySchema) *memorySchema {
	if schema == nil {
		return nil
	}
	collections := make(map[string]func() any, len(schema.collections))
	for name, factory := range schema.collections {
		collections[name] = factory
	}
	engines := make(map[string]engineFactory, len(schema.engines))
	for name, factory := range schema.engines {
		engines[name] = factory
	}
	return &memorySchema{
		collections:    collections,
		engines:        engines,
		allowUndefined: schema.allowUndefined,
	}
}

func cloneRecordKey(key *record.Key) *record.Key {
	if key == nil {
		return nil
	}
	parent := cloneRecordKey(key.Parent())
	if key.ID == nil && key.IDKind != 0 {
		return record.NewIncompleteKey(key.Collection(), key.IDKind, parent)
	}
	id := cloneRecordKeyID(key.ID)
	if parent == nil {
		return record.NewKeyWithID[any](key.Collection(), id)
	}
	return record.NewKeyWithParentAndID[any](parent, key.Collection(), id)
}

// Composite key slices are the only mutable key-ID shape defined by record.
// Their field values are identifiers and therefore treated as immutable, while
// the containing slice is copied so callers cannot replace components after a
// checkpoint.
func cloneRecordKeyID(id any) any {
	fields, ok := id.([]record.FieldVal)
	if !ok {
		return id
	}
	return append([]record.FieldVal(nil), fields...)
}

type serializedCheckpoint struct {
	mu         sync.Mutex
	generation string
	snapshot   *databaseSnapshot
}

var _ branching.Checkpoint = (*serializedCheckpoint)(nil)

func (c *serializedCheckpoint) Generation() string {
	return c.generation
}

func (c *serializedCheckpoint) Branch(ctx context.Context) (branching.Branch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.snapshot == nil {
		return nil, branching.ErrReleased
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &serializedBranch{db: c.snapshot.newDatabase()}, nil
}

func (c *serializedCheckpoint) Release(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.snapshot = nil
	return nil
}

func (s databaseSnapshot) newDatabase() dal.DB {
	db := &database{
		collections:                     make(map[string]storageEngine, len(s.collections)),
		schema:                          cloneMemorySchema(s.schema),
		noReadsAfterWritesInTransaction: s.noReadsAfterWritesInTransaction,
		schemaRefBreaking:               s.schemaRefBreaking,
	}
	for name, snapshot := range s.collections {
		db.collections[name] = snapshot.newEngine()
	}
	return db
}

func (s serializedEngineSnapshot) newEngine() *serializedEngine {
	records := make(map[string][]byte, len(s.records))
	for id, data := range s.records {
		records[id] = append([]byte(nil), data...)
	}
	keys := make(map[string]*record.Key, len(s.keys))
	for id, key := range s.keys {
		keys[id] = cloneRecordKey(key)
	}
	return &serializedEngine{
		collection: s.collection,
		factory:    s.factory,
		records:    records,
		keys:       keys,
	}
}

type serializedBranch struct {
	db     dal.DB
	closed atomic.Bool
}

var _ branching.Branch = (*serializedBranch)(nil)

func (b *serializedBranch) DB() dal.DB {
	return b.db
}

func (b *serializedBranch) Close(context.Context) error {
	b.closed.Store(true)
	return nil
}
