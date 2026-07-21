package dalgo2memory

import (
	"context"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
	"github.com/stretchr/testify/require"
)

// mixedDB builds a map[string]any-backed columnar collection "m" with the given
// column options (typically one or more WithDeclaredColumn plus an optional
// strategy/ref-breaking override).
func mixedDB(t *testing.T, opts ...ColumnOption) *database {
	t.Helper()
	db := NewDB(WithSchema(false,
		WithCollection[map[string]any]("m", nil, WithColumnarStorage(opts...)),
	)).(*database)
	return db
}

// serMapDB builds a Serialized map[string]any-backed collection "m" for parity
// comparisons.
func serMapDB(t *testing.T) *database {
	t.Helper()
	db := NewDB(WithSchema(false,
		WithCollection[map[string]any]("m", nil, WithSerializedStorage()),
	)).(*database)
	return db
}

func setMap(t *testing.T, db *database, id string, data map[string]any) {
	t.Helper()
	require.NoError(t, db.Set(context.Background(),
		record.NewRecordWithData(record.NewKeyWithID("m", id), data)))
}

func getMap(t *testing.T, db *database, id string) map[string]any {
	t.Helper()
	got := map[string]any{}
	require.NoError(t, db.Get(context.Background(),
		record.NewRecordWithData(record.NewKeyWithID("m", id), &got)))
	return got
}

// AC: mixed-mode-requires-declared-column.
func TestMixed_RequiresDeclaredColumn(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// With a declared column the mixed-mode engine builds and works.
	okDB := mixedDB(t, WithDeclaredColumn[int]("age"))
	eng := okDB.engine("m").(*columnarEngine)
	require.Nil(t, eng.initErr)
	require.True(t, eng.mapBacked)
	require.Contains(t, eng.byName, "age")
	setMap(t, okDB, "u1", map[string]any{"age": 30, "name": "Alice"})
	require.Equal(t, map[string]any{"age": float64(30), "name": "Alice"}, getMap(t, okDB, "u1"))

	// With no declared column the selection fails with a descriptive error.
	noColDB := mixedDB(t)
	err := noColDB.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("m", "x"), map[string]any{"a": 1}))
	require.Error(t, err)
	require.ErrorContains(t, err, "m")
	require.ErrorContains(t, err, "declared column")

	badEng := noColDB.engine("m").(*columnarEngine)
	require.False(t, badEng.mapBacked)
	require.NotNil(t, badEng.initErr)
}

// AC: declared-value-wrong-type-errors.
func TestMixed_DeclaredValueWrongTypeErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := mixedDB(t, WithDeclaredColumn[int]("age"))

	err := db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("m", "u1"),
		map[string]any{"age": "old", "name": "Alice"}))
	require.Error(t, err)
	require.ErrorContains(t, err, "age")

	// Nothing was stored for that id: neither dropped nor coerced.
	eng := db.engine("m").(*columnarEngine)
	require.NotContains(t, eng.idToSlot, "u1")
	exists, existsErr := db.Exists(ctx, record.NewKeyWithID("m", "u1"))
	require.NoError(t, existsErr)
	require.False(t, exists)
}

// AC: declared-field-removed-from-leftover.
func TestMixed_DeclaredFieldRemovedFromLeftover(t *testing.T) {
	t.Parallel()
	db := mixedDB(t, WithDeclaredColumn[int]("age"))
	setMap(t, db, "u1", map[string]any{"age": 30, "name": "Alice", "email": "a@x.io"})

	eng := db.engine("m").(*columnarEngine)
	slot := eng.idToSlot["u1"]

	// age lives in a typed []int column at the slot.
	require.Equal(t, reflect.TypeOf([]int{}), eng.byName["age"].values.Type())
	require.Equal(t, 30, eng.byName["age"].values.Index(slot).Interface())

	// age is absent from leftover; name and email remain.
	leftover := eng.leftover[slot]
	require.NotContains(t, leftover, "age")
	require.Equal(t, "Alice", leftover["name"])
	require.Equal(t, "a@x.io", leftover["email"])
}

// AC: leftover-holds-undeclared-fields.
func TestMixed_LeftoverHoldsUndeclaredFields(t *testing.T) {
	t.Parallel()
	db := mixedDB(t, WithDeclaredColumn[int]("age"))
	setMap(t, db, "withName", map[string]any{"age": 1, "name": "Alice"})
	setMap(t, db, "ageOnly", map[string]any{"age": 2})
	setMap(t, db, "twoExtra", map[string]any{"age": 3, "city": "Paris", "note": "vip"})

	eng := db.engine("m").(*columnarEngine)

	require.Equal(t, map[string]any{"name": "Alice"}, eng.leftover[eng.idToSlot["withName"]])
	require.Nil(t, eng.leftover[eng.idToSlot["ageOnly"]])
	require.Equal(t, map[string]any{"city": "Paris", "note": "vip"}, eng.leftover[eng.idToSlot["twoExtra"]])
}

// AC: slot-stays-synced-through-delete-and-compaction.
func TestMixed_SlotSyncedThroughDeleteAndCompaction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := mixedDB(t, WithDeclaredColumn[int]("age"))
	records := map[string]map[string]any{
		"a": {"age": 10, "name": "Anna"},
		"b": {"age": 20, "city": "Berlin"},
		"c": {"age": 30, "name": "Carol", "role": "admin"},
		"d": {"age": 40},
	}
	for id, rec := range records {
		setMap(t, db, id, rec)
	}
	eng := db.engine("m").(*columnarEngine)

	// Delete b: its slot is freed for reuse.
	freed := eng.idToSlot["b"]
	require.NoError(t, db.Delete(ctx, record.NewKeyWithID("m", "b")))
	require.NotContains(t, eng.idToSlot, "b")

	// Reuse the freed slot with a new record; its declared value and leftover
	// land at the same slot.
	setMap(t, db, "e", map[string]any{"age": 50, "name": "Eve"})
	require.Equal(t, freed, eng.idToSlot["e"])
	require.Equal(t, 50, eng.byName["age"].values.Index(freed).Interface())
	require.Equal(t, map[string]any{"name": "Eve"}, eng.leftover[freed])

	// Delete two more to cross the compaction threshold (2 dead of 4 live+dead).
	require.NoError(t, db.Delete(ctx, record.NewKeyWithID("m", "c")))
	require.NoError(t, db.Delete(ctx, record.NewKeyWithID("m", "d")))
	require.Equal(t, 0, eng.deadCount, "compaction reclaimed dead slots")
	require.Len(t, eng.leftover, len(eng.live), "leftover stays the same length as the slot vector")

	// Surviving records read back with declared + undeclared fields intact, and
	// at each surviving slot the declared value and leftover stay paired.
	require.Equal(t, map[string]any{"age": float64(10), "name": "Anna"}, getMap(t, db, "a"))
	require.Equal(t, map[string]any{"age": float64(50), "name": "Eve"}, getMap(t, db, "e"))
	slotA := eng.idToSlot["a"]
	require.True(t, eng.live[slotA])
	require.Equal(t, 10, eng.byName["age"].values.Index(slotA).Interface())
	require.Equal(t, map[string]any{"name": "Anna"}, eng.leftover[slotA])
}

// AC: read-reconstructs-full-record.
func TestMixed_ReadReconstructsFullRecord(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := mixedDB(t, WithDeclaredColumn[int]("age"))
	written := map[string]any{"age": 42, "name": "Alice", "email": "a@x.io"}
	setMap(t, db, "u1", written)

	want := map[string]any{"age": float64(42), "name": "Alice", "email": "a@x.io"}

	// Get reassembles the full record.
	require.Equal(t, want, getMap(t, db, "u1"))

	// A query reassembles the same full record (materialized into a map target).
	q := dal.From(dal.NewRootCollectionRef("m", "")).NewQuery().
		WhereField("name", dal.Equal, "Alice").
		SelectIntoRecord(func() record.Record {
			return record.NewRecordWithIncompleteKey("m", reflect.String, &map[string]any{})
		})
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	records := readAll(t, reader)
	require.Len(t, records, 1)
	got := records[0].Data().(*map[string]any)
	require.Equal(t, want, *got)

	// Each key appears exactly once (merge never duplicates a key).
	require.Len(t, *got, 3)
}

// recordingDeclaredStrategy records EqualSlots invocations on a declared column
// while delegating matching to the engine's live column slice via a back
// reference resolved at SetValue time.
type recordingDeclaredStrategy struct {
	values  map[int]any
	queried []any
	live    func(int) bool
}

func newRecordingDeclaredStrategy(live func(int) bool) *recordingDeclaredStrategy {
	return &recordingDeclaredStrategy{values: map[int]any{}, live: live}
}

func (s *recordingDeclaredStrategy) SetValue(slot int, value any) { s.values[slot] = value }

func (s *recordingDeclaredStrategy) ClearValue(slot int) { delete(s.values, slot) }

func (s *recordingDeclaredStrategy) EqualSlots(value any) (SlotSet, bool) {
	s.queried = append(s.queried, value)
	slots := make(SlotSet)
	for slot, v := range s.values {
		if s.live(slot) && v == value {
			slots[slot] = struct{}{}
		}
	}
	return slots, true
}

// seedMixedQueryData seeds identical map data into both engines for parity. The
// declared columns are age (int) and tier (string); city/name are undeclared
// leftover fields.
func seedMixedQueryData(t *testing.T, db *database) {
	t.Helper()
	setMap(t, db, "r1", map[string]any{"age": 30, "tier": "gold", "city": "Paris", "name": "alice"})
	setMap(t, db, "r2", map[string]any{"age": 25, "tier": "silver", "city": "Lyon", "name": "bob"})
	setMap(t, db, "r3", map[string]any{"age": 30, "tier": "gold", "city": "Paris", "name": "carol"})
	setMap(t, db, "r4", map[string]any{"age": 40, "tier": "bronze", "city": "Nice", "name": "dave"})
}

// AC: where-on-declared-column-accelerated. A custom strategy on the declared
// string column "tier" records its invocation, and the accelerated result equals
// the Serialized engine's.
func TestMixed_WhereOnDeclaredColumnAccelerated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	colDB := mixedDB(t, WithDeclaredColumn[int]("age"), WithDeclaredColumn[string]("tier"))
	eng := colDB.engine("m").(*columnarEngine)
	stgy := newRecordingDeclaredStrategy(func(slot int) bool { return eng.live[slot] })
	eng.byName["tier"].strategy = stgy
	eng.byName["tier"].defaultStgy = nil

	serDB := serMapDB(t)
	seedMixedQueryData(t, colDB)
	seedMixedQueryData(t, serDB)

	build := func() dal.Query {
		return dal.From(dal.NewRootCollectionRef("m", "")).NewQuery().
			WhereField("tier", dal.Equal, "gold").
			SelectKeysOnly(reflect.String)
	}
	colReader, err := colDB.ExecuteQueryToRecordsReader(ctx, build())
	require.NoError(t, err)
	serReader, err := serDB.ExecuteQueryToRecordsReader(ctx, build())
	require.NoError(t, err)

	require.Equal(t, []any{"gold"}, stgy.queried, "the declared column's strategy was consulted")
	colIDs := recordIDs(readAll(t, colReader))
	serIDs := recordIDs(readAll(t, serReader))
	require.Equal(t, serIDs, colIDs)
	require.Equal(t, []string{"r1", "r3"}, colIDs)
}

// AC: where-on-leftover-field-scans.
func TestMixed_WhereOnLeftoverFieldScans(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	colDB := mixedDB(t, WithDeclaredColumn[int]("age"))
	serDB := serMapDB(t)
	seedMixedQueryData(t, colDB)
	seedMixedQueryData(t, serDB)

	// city is undeclared (a leftover field); candidateSlots has no opinion.
	eng := colDB.engine("m").(*columnarEngine)
	_, ok := eng.candidateSlots(dal.Comparison{Operator: dal.Equal,
		Left: dal.NewFieldRef("", "city"), Right: dal.NewConstant("Paris")})
	require.False(t, ok, "a leftover field is not a column, so no opinion")

	build := func() dal.Query {
		return dal.From(dal.NewRootCollectionRef("m", "")).NewQuery().
			WhereField("city", dal.Equal, "Paris").
			SelectKeysOnly(reflect.String)
	}
	colReader, err := colDB.ExecuteQueryToRecordsReader(ctx, build())
	require.NoError(t, err)
	serReader, err := serDB.ExecuteQueryToRecordsReader(ctx, build())
	require.NoError(t, err)

	colIDs := recordIDs(readAll(t, colReader))
	serIDs := recordIDs(readAll(t, serReader))
	require.Equal(t, serIDs, colIDs)
	require.Equal(t, []string{"r1", "r3"}, colIDs)
}

// AC: mixed-mode-parity-and-fidelity.
func TestMixed_ParityAndFidelity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Faithful mixed-mode collection declaring a reference-bearing column
	// (tags) alongside an undeclared reference-bearing field (meta).
	db := mixedDB(t, WithDeclaredColumn[[]string]("tags"))
	key := record.NewKeyWithID("m", "u1")
	tags := []string{"a", "b"}
	meta := map[string]any{"city": "Paris"}
	written := map[string]any{"tags": tags, "meta": meta}
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(key, written)))

	// Mutate both the declared and the undeclared reference after the write.
	tags[0] = "MUTATED"
	meta["city"] = "MUTATED"

	got := getMap(t, db, "u1")
	require.Equal(t, []any{"a", "b"}, got["tags"], "declared ref-bearing column is isolated")
	require.Equal(t, map[string]any{"city": "Paris"}, got["meta"], "leftover ref-bearing field is isolated")

	// Behavioral parity with Serialized: Set overwrites, Insert-duplicate errors,
	// Get-absent is not-found.
	serDB := serMapDB(t)
	for _, target := range []*database{db, serDB} {
		k := record.NewKeyWithID("m", "p1")
		require.NoError(t, target.Set(ctx, record.NewRecordWithData(k, map[string]any{"tags": []string{"x"}})))
		require.NoError(t, target.Set(ctx, record.NewRecordWithData(k, map[string]any{"tags": []string{"y"}})))
		dup := target.Insert(ctx, record.NewRecordWithData(k, map[string]any{"tags": []string{"z"}}))
		require.Error(t, dup)
		require.ErrorContains(t, dup, "already exists")
		absent := target.Get(ctx, record.NewRecordWithData(record.NewKeyWithID("m", "absent"), &map[string]any{}))
		require.True(t, record.IsNotFound(absent))
	}
}

// TestMixed_OptOutSharesLeftoverlessRefs covers the ref-breaking opt-out for a
// map-backed collection: with WithColumnarRefBreaking(false) a mutation of the
// undeclared field shows through (leftover values are not deep-copied).
func TestMixed_UpdateAddsAndChangesFields(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := mixedDB(t, WithDeclaredColumn[int]("age"))
	key := record.NewKeyWithID("m", "u1")
	setMap(t, db, "u1", map[string]any{"age": 1, "name": "Alice"})

	// Update a declared column and add a new leftover field (map-backed accepts
	// undeclared fields rather than rejecting them).
	require.NoError(t, db.Update(ctx, key, []update.Update{
		update.ByFieldName("age", 2),
		update.ByFieldName("city", "Paris"),
	}))
	require.Equal(t, map[string]any{"age": float64(2), "name": "Alice", "city": "Paris"}, getMap(t, db, "u1"))
}

// TestMixed_LastDeclarationWins covers the duplicate-declaration policy: the last
// WithDeclaredColumn for a name wins (its element type), and the column keeps its
// position (no duplicate column).
func TestMixed_LastDeclarationWins(t *testing.T) {
	t.Parallel()
	db := mixedDB(t, WithDeclaredColumn[string]("v"), WithDeclaredColumn[int]("v"))
	eng := db.engine("m").(*columnarEngine)
	require.Len(t, eng.columns, 1)
	require.Equal(t, reflect.TypeOf([]int{}), eng.byName["v"].values.Type())
	setMap(t, db, "u1", map[string]any{"v": 7, "x": "leftover"})
	require.Equal(t, map[string]any{"v": float64(7), "x": "leftover"}, getMap(t, db, "u1"))
}

// TestMixed_DeclaredOnStructIsRedundant covers that declared options on a struct
// collection are accepted but the struct path still reflects over T.
func TestMixed_DeclaredOnStructIsRedundant(t *testing.T) {
	t.Parallel()
	db := NewDB(WithSchema(false,
		WithCollection[item]("items", nil, WithColumnarStorage(WithDeclaredColumn[int]("Count"))),
	)).(*database)
	eng := db.engine("items").(*columnarEngine)
	require.False(t, eng.mapBacked)
	// Struct reflection still produced all columns (not just the declared one).
	require.Contains(t, eng.byName, "Name")
	require.Contains(t, eng.byName, "Count")
	require.Contains(t, eng.byName, "Active")
}

// TestMixed_InterfaceDeclaredColumn covers a declared interface column becoming
// an []any fallback slice.
func TestMixed_InterfaceDeclaredColumn(t *testing.T) {
	t.Parallel()
	db := mixedDB(t, WithDeclaredColumn[any]("payload"))
	eng := db.engine("m").(*columnarEngine)
	require.Equal(t, reflect.TypeOf([]any{}), eng.byName["payload"].values.Type())
	setMap(t, db, "u1", map[string]any{"payload": []any{1, 2}, "tag": "x"})
	require.Equal(t, map[string]any{"payload": []any{float64(1), float64(2)}, "tag": "x"}, getMap(t, db, "u1"))
}

// TestMixed_IsMapBackedFactory covers isMapBackedFactory's nil-factory and
// non-map branches directly.
func TestMixed_IsMapBackedFactory(t *testing.T) {
	t.Parallel()
	require.False(t, isMapBackedFactory(nil))
	require.False(t, isMapBackedFactory(func() any { return new(int) }))
	require.False(t, isMapBackedFactory(func() any { return &item{} }))
	require.True(t, isMapBackedFactory(func() any { m := map[string]any{}; return &m }))
}

// TestMixed_ExplicitStrategyOnDeclaredColumn covers the WithColumnStrategy path
// in buildDeclaredColumns: an explicit strategy is installed for a declared
// map-backed column (and the default is not).
func TestMixed_ExplicitStrategyOnDeclaredColumn(t *testing.T) {
	t.Parallel()
	stgy := newRecordingDeclaredStrategy(func(int) bool { return true })
	db := mixedDB(t, WithDeclaredColumn[string]("tier"), WithColumnStrategy("tier", stgy))
	eng := db.engine("m").(*columnarEngine)
	require.Same(t, stgy, eng.byName["tier"].strategy)
	require.Nil(t, eng.byName["tier"].defaultStgy)
}

// TestMixed_DecodeMapFieldsErrors covers decodeMapFields' marshal-error branch (a
// non-serializable value) and its unmarshal-to-map error branch (data that is a
// JSON array, not an object).
func TestMixed_DecodeMapFieldsErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := mixedDB(t, WithDeclaredColumn[int]("age"))

	// Non-serializable value -> json.Marshal error.
	marshalErr := db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("m", "u1"),
		map[string]any{"age": 1, "bad": make(chan int)}))
	require.Error(t, marshalErr)

	// Data that marshals to a JSON array, not an object -> unmarshal-to-map error.
	eng := db.engine("m").(*columnarEngine)
	_, _, arrErr := eng.decodeMapFields([]int{1, 2, 3})
	require.Error(t, arrErr)
}

// TestMixed_RaceInterleavesOperations exercises the mixed-mode engine under the
// global write lock with concurrent reads, writes, deletes (triggering
// compaction), and queries. Run with -race.
func TestMixed_RaceInterleavesOperations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := mixedDB(t, WithDeclaredColumn[int]("age"))

	const n = 40
	ids := make([]string, n)
	for i := range ids {
		ids[i] = "k" + strconv.Itoa(i)
		setMap(t, db, ids[i], map[string]any{"age": i, "name": ids[i], "even": i%2 == 0})
	}

	var wg sync.WaitGroup
	const workers = 8
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for round := 0; round < 50; round++ {
				id := ids[(seed+round)%n]
				switch round % 4 {
				case 0:
					_ = db.Get(ctx, record.NewRecordWithData(record.NewKeyWithID("m", id), &map[string]any{}))
				case 1:
					_ = db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("m", id),
						map[string]any{"age": round, "name": id}))
				case 2:
					_ = db.Delete(ctx, record.NewKeyWithID("m", id))
				case 3:
					q := dal.From(dal.NewRootCollectionRef("m", "")).NewQuery().
						WhereField("age", dal.Equal, 0).
						SelectKeysOnly(reflect.String)
					reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
					if err == nil {
						_ = readAll(t, reader)
					}
				}
			}
		}(w)
	}
	wg.Wait()

	eng := db.engine("m").(*columnarEngine)
	for id, slot := range eng.idToSlot {
		require.True(t, eng.live[slot], "id %s maps to a live slot", id)
	}
	require.Len(t, eng.leftover, len(eng.live))
}
