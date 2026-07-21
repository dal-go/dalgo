package dalgo2memory

import (
	"context"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
	"github.com/stretchr/testify/require"
)

// countingEngine is a test-only second storage engine: it wraps a Serialized
// engine and records how many writes it received, proving the registry routes
// per collection through a distinct, non-Serialized engine instance.
type countingEngine struct {
	inner  *serializedEngine
	writes int
}

func (e *countingEngine) exists(id string) bool { return e.inner.exists(id) }

func (e *countingEngine) store(id string, record record.Record, overwrite bool) error {
	e.writes++
	return e.inner.store(id, record, overwrite)
}

func (e *countingEngine) load(id string, record record.Record) error { return e.inner.load(id, record) }

func (e *countingEngine) delete(id string) { e.inner.delete(id) }

func (e *countingEngine) update(id string, updates []update.Update) error {
	return e.inner.update(id, updates)
}

func (e *countingEngine) rows() ([]engineRow, error) { return e.inner.rows() }

// withCountingStorage is a test-only CollectionOption selecting countingEngine.
func withCountingStorage() CollectionOption {
	return func(def *collectionDef) {
		def.newEngine = func(collection string, factory func() any, _ bool) storageEngine {
			return &countingEngine{inner: newSerializedEngine(collection, factory)}
		}
	}
}

// TestSerializedStorageMatchesDefault verifies AC:default-is-serialized: a
// collection registered with WithSerializedStorage() behaves identically to an
// option-less one across Set/Get/equality-filtered query.
func TestSerializedStorageMatchesDefault(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	run := func(opts ...CollectionOption) *user {
		db := NewDB(WithSchema(false,
			WithCollection[user]("users", nil, opts...),
		)).(*database)
		require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("users", "u1"), &user{Name: "Alice", Role: "admin"})))
		require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("users", "u2"), &user{Name: "Bob", Role: "member"})))

		q := dal.From(dal.NewRootCollectionRef("users", "")).NewQuery().WhereField("Role", dal.Equal, "admin").SelectKeysOnly(reflect.String)
		reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
		require.NoError(t, err)
		records := readAll(t, reader)
		require.Len(t, records, 1)
		got, ok := records[0].Data().(*user)
		require.True(t, ok)
		return got
	}

	defaultResult := run()
	explicitResult := run(WithSerializedStorage())
	require.Equal(t, defaultResult, explicitResult)
	require.Equal(t, "Alice", defaultResult.Name)
}

// TestMixedEnginesInOneDB verifies AC:mixed-engines-in-one-db: one database can
// host different engines per collection, each routing through its own instance
// with no cross-collection interference.
func TestMixedEnginesInOneDB(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB(WithSchema(false,
		WithCollection[user]("a", nil, WithSerializedStorage()),
		WithCollection[user]("b", nil, withCountingStorage()),
	)).(*database)

	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("a", "a1"), &user{Name: "Alice"})))
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("b", "b1"), &user{Name: "Bob"})))
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("b", "b2"), &user{Name: "Carol"})))

	// Collection a uses the Serialized engine.
	_, isSerialized := db.collections["a"].(*serializedEngine)
	require.True(t, isSerialized)

	// Collection b uses the counting engine, which observed only b's writes.
	counting, isCounting := db.collections["b"].(*countingEngine)
	require.True(t, isCounting)
	require.Equal(t, 2, counting.writes)

	// Each retrieves its own records; operations on one do not affect the other.
	var a1 user
	require.NoError(t, db.Get(ctx, record.NewRecordWithData(record.NewKeyWithID("a", "a1"), &a1)))
	require.Equal(t, "Alice", a1.Name)
	var b1 user
	require.NoError(t, db.Get(ctx, record.NewRecordWithData(record.NewKeyWithID("b", "b1"), &b1)))
	require.Equal(t, "Bob", b1.Name)

	existsA, err := db.Exists(ctx, record.NewKeyWithID("a", "b1"))
	require.NoError(t, err)
	require.False(t, existsA)
}

// TestUnregisteredCollectionUsesSerialized verifies
// AC:unregistered-collection-is-serialized: a collection never registered
// through a schema resolves to the Serialized engine.
func TestUnregisteredCollectionUsesSerialized(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB().(*database)
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("ad-hoc", "x1"), &user{Name: "Alice"})))
	_, isSerialized := db.collections["ad-hoc"].(*serializedEngine)
	require.True(t, isSerialized)

	var got user
	require.NoError(t, db.Get(ctx, record.NewRecordWithData(record.NewKeyWithID("ad-hoc", "x1"), &got)))
	require.Equal(t, "Alice", got.Name)
}

// TestEngineCachesInstance verifies the registry returns the same engine
// instance on repeated access for a collection.
func TestEngineCachesInstance(t *testing.T) {
	t.Parallel()
	db := NewDB().(*database)
	first := db.engine("things")
	second := db.engine("things")
	require.Same(t, first, second)
}
