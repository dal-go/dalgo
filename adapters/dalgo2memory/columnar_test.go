package dalgo2memory

import (
	"context"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/update"
	"github.com/stretchr/testify/require"
)

// typed record types used across the columnar tests.

// item is a simple typed record with int/string/bool scalar columns and an
// interface{} column (which must become an []any fallback column).
type item struct {
	Name   string
	Count  int
	Active bool
	Extra  any
}

// doc is a record with a scalar column and reference-bearing columns (a slice
// and a nested struct), used to prove ref-breaking and the fidelity opt-out.
type doc struct {
	Title string
	Tags  []string
	Meta  address
}

func columnarItemDB(t *testing.T, colOpts ...ColumnOption) *database {
	t.Helper()
	db := NewDB(WithSchema(false,
		WithCollection[item]("items", nil, WithColumnarStorage(colOpts...)),
	)).(*database)
	return db
}

// TestColumnar_RequiresTypedCollection verifies
// columnar-storage#ac:columnar-requires-typed-collection.
func TestColumnar_RequiresTypedCollection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Typed collection uses the columnar engine successfully.
	typedDB := columnarItemDB(t)
	require.NoError(t, typedDB.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", "i1"), &item{Name: "a"})))
	eng, ok := typedDB.collections["items"].(*columnarEngine)
	require.True(t, ok)
	require.Nil(t, eng.initErr)

	// Selecting columnar storage for a non-struct, non-map collection (here a
	// scalar element type) fails with the typed-collection error on use.
	scalarDB := NewDB(WithSchema(false,
		WithCollection[int]("nums", nil, WithColumnarStorage()),
	)).(*database)
	scalarErr := scalarDB.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("nums", "n1"), 1))
	require.Error(t, scalarErr)
	require.ErrorContains(t, scalarErr, "nums")
	require.ErrorContains(t, scalarErr, "typed collection")

	// Selecting columnar storage for a schemaless (map[string]any) collection
	// with no declared column fails with a descriptive error on use.
	schemalessDB := NewDB(WithSchema(false,
		WithCollection[map[string]any]("blobs", nil, WithColumnarStorage()),
	)).(*database)
	err := schemalessDB.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("blobs", "b1"), map[string]any{"x": 1}))
	require.Error(t, err)
	require.ErrorContains(t, err, "blobs")
	require.ErrorContains(t, err, "declared column")

	// Every operation reports the init error.
	badEng := schemalessDB.engine("blobs").(*columnarEngine)
	require.False(t, badEng.exists("b1"))
	require.Error(t, badEng.load("b1", dal.NewRecordWithData(dal.NewKeyWithID("blobs", "b1"), &map[string]any{})))
	require.Error(t, badEng.update("b1", []update.Update{update.ByFieldName("x", 1)}))
	_, rowsErr := badEng.rows()
	require.Error(t, rowsErr)
	_, _, candErr := badEng.candidateRows(nil)
	require.Error(t, candErr)
	badEng.delete("b1") // no-op, must not panic
}

// TestColumnar_TypedSlicesAndAnyFallback verifies
// columnar-storage#ac:columns-are-typed-slices.
func TestColumnar_TypedSlicesAndAnyFallback(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := columnarItemDB(t)
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", "i1"),
		&item{Name: "a", Count: 7, Active: true, Extra: "x"})))

	eng := db.collections["items"].(*columnarEngine)
	require.Equal(t, reflect.TypeOf([]string{}), eng.byName["Name"].values.Type())
	require.Equal(t, reflect.TypeOf([]int{}), eng.byName["Count"].values.Type())
	require.Equal(t, reflect.TypeOf([]bool{}), eng.byName["Active"].values.Type())
	require.Equal(t, reflect.TypeOf([]any{}), eng.byName["Extra"].values.Type())
}

// TestColumnar_SlotStableAcrossColumns verifies
// columnar-storage#ac:slot-stable-across-columns.
func TestColumnar_SlotStableAcrossColumns(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := columnarItemDB(t)
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", "a"), &item{Name: "a", Count: 1})))
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", "b"), &item{Name: "b", Count: 2})))

	eng := db.collections["items"].(*columnarEngine)
	slotA := eng.idToSlot["a"]
	slotB := eng.idToSlot["b"]
	require.NotEqual(t, slotA, slotB)

	// a's values are all drawn from its slot.
	require.Equal(t, "a", eng.byName["Name"].values.Index(slotA).Interface())
	require.Equal(t, 1, eng.byName["Count"].values.Index(slotA).Interface())

	// Writing more records does not move a's slot.
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", "c"), &item{Name: "c", Count: 3})))
	require.Equal(t, slotA, eng.idToSlot["a"])
}

// TestColumnar_WriteBreaksRefsByDefault verifies
// columnar-storage#ac:write-breaks-refs-by-default.
func TestColumnar_WriteBreaksRefsByDefault(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB(WithSchema(false, WithCollection[doc]("docs", nil, WithColumnarStorage()))).(*database)
	key := dal.NewKeyWithID("docs", "d1")
	written := &doc{Title: "t", Tags: []string{"a", "b"}, Meta: address{City: "Paris"}}
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key, written)))

	written.Tags[0] = "MUTATED"
	written.Meta.City = "MUTATED"

	var got doc
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &got)))
	require.Equal(t, []string{"a", "b"}, got.Tags)
	require.Equal(t, "Paris", got.Meta.City)
}

// TestColumnar_FidelityOptOutMatrix verifies
// columnar-storage#ac:fidelity-opt-out-toggles-ref-breaking: a default
// collection (a), a per-collection opt-out (b), and a schema-wide opt-out with
// one collection re-enabling fidelity (c).
func TestColumnar_FidelityOptOutMatrix(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mutateThenRead := func(db *database, collection string) []string {
		key := dal.NewKeyWithID(collection, "d1")
		written := &doc{Title: "t", Tags: []string{"a", "b"}}
		require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key, written)))
		written.Tags[0] = "MUTATED"
		var got doc
		require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &got)))
		return got.Tags
	}

	// (a) default faithful collection: mutation does not show through.
	faithfulDB := NewDB(WithSchema(false,
		WithCollection[doc]("a", nil, WithColumnarStorage()),
	)).(*database)
	require.Equal(t, []string{"a", "b"}, mutateThenRead(faithfulDB, "a"))

	// (b) per-collection opt-out: mutation shows through.
	optOutDB := NewDB(WithSchema(false,
		WithCollection[doc]("b", nil, WithColumnarStorage(WithColumnarRefBreaking(false))),
	)).(*database)
	require.Equal(t, []string{"MUTATED", "b"}, mutateThenRead(optOutDB, "b"))

	// (c) schema-wide opt-out, but one collection re-enables fidelity. The
	// re-enabled collection is faithful; the inheriting collection reflects the
	// mutation, proving per-collection overrides schema-wide.
	schemaWideDB := NewDB(
		WithoutSchemaRefBreaking(),
		WithSchema(false,
			WithCollection[doc]("reenabled", nil, WithColumnarStorage(WithColumnarRefBreaking(true))),
			WithCollection[doc]("inherits", nil, WithColumnarStorage()),
		),
	).(*database)
	require.Equal(t, []string{"a", "b"}, mutateThenRead(schemaWideDB, "reenabled"))
	require.Equal(t, []string{"MUTATED", "b"}, mutateThenRead(schemaWideDB, "inherits"))
}

// TestColumnar_ParityWithSerializedOps verifies
// columnar-storage#ac:parity-with-serialized-ops: Set overwrites,
// Insert-duplicate errors, Get/Update-absent are not-found, Update of an
// undefined field is rejected — identical outcomes to the Serialized engine.
func TestColumnar_ParityWithSerializedOps(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	run := func(t *testing.T, opts ...CollectionOption) {
		db := NewDB(WithSchema(false, WithCollection[user]("users", nil, opts...))).(*database)
		key := dal.NewKeyWithID("users", "u1")

		// Set overwrites.
		require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key, &user{Name: "Alice", Role: "admin"})))
		require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key, &user{Name: "Alice2", Role: "admin"})))
		var got user
		require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &got)))
		require.Equal(t, "Alice2", got.Name)

		// Insert on existing id errors and does not overwrite.
		err := db.Insert(ctx, dal.NewRecordWithData(key, &user{Name: "Bob"}))
		require.Error(t, err)
		require.ErrorContains(t, err, "already exists")
		require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &got)))
		require.Equal(t, "Alice2", got.Name)

		// Get/Update on an absent id are not-found.
		absent := dal.NewKeyWithID("users", "absent")
		getErr := db.Get(ctx, dal.NewRecordWithData(absent, &user{}))
		require.True(t, dal.IsNotFound(getErr))
		updErr := db.Update(ctx, absent, []update.Update{update.ByFieldName("Role", "x")})
		require.True(t, dal.IsNotFound(updErr))

		// Update of an undefined field is rejected.
		badUpd := db.Update(ctx, key, []update.Update{update.ByFieldName("Undefined", 1)})
		require.Error(t, badUpd)
		require.ErrorContains(t, badUpd, "users")

		// A defined-field update applies.
		require.NoError(t, db.Update(ctx, key, []update.Update{update.ByFieldName("Role", "member")}))
		require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &got)))
		require.Equal(t, "member", got.Role)
	}

	t.Run("columnar", func(t *testing.T) { t.Parallel(); run(t, WithColumnarStorage()) })
	t.Run("serialized", func(t *testing.T) { t.Parallel(); run(t, WithSerializedStorage()) })
}

// TestColumnar_InsertRejectsUnknownField proves a store carrying an undefined
// field is rejected (parity with the Serialized engine's DisallowUnknownFields).
func TestColumnar_InsertRejectsUnknownField(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := columnarItemDB(t)
	err := db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", "i1"),
		map[string]any{"Name": "a", "Bogus": 1}))
	require.Error(t, err)
	require.ErrorContains(t, err, "Bogus")
}

// TestColumnar_DeleteTombstonesAndReuses verifies
// columnar-storage#ac:delete-tombstones-and-hides.
func TestColumnar_DeleteTombstonesAndReuses(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := columnarItemDB(t)
	for _, id := range []string{"a", "b", "c"} {
		require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", id), &item{Name: id})))
	}
	eng := db.collections["items"].(*columnarEngine)
	slotA, slotB, slotC := eng.idToSlot["a"], eng.idToSlot["b"], eng.idToSlot["c"]

	require.NoError(t, db.Delete(ctx, dal.NewKeyWithID("items", "b")))

	// b is hidden from Exists/Get/queries; a and c keep their slots.
	existsB, err := db.Exists(ctx, dal.NewKeyWithID("items", "b"))
	require.NoError(t, err)
	require.False(t, existsB)
	require.True(t, dal.IsNotFound(db.Get(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", "b"), &item{}))))
	require.Equal(t, slotA, eng.idToSlot["a"])
	require.Equal(t, slotC, eng.idToSlot["c"])

	rows, err := eng.rows()
	require.NoError(t, err)
	require.Len(t, rows, 2)

	// A later insert reuses b's freed slot.
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", "d"), &item{Name: "d"})))
	require.Equal(t, slotB, eng.idToSlot["d"])
}

// TestColumnar_CompactionPreservesLiveRecords verifies
// columnar-storage#ac:compaction-preserves-live-records.
func TestColumnar_CompactionPreservesLiveRecords(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := columnarItemDB(t)
	for i, id := range []string{"a", "b", "c", "d"} {
		require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", id), &item{Name: id, Count: i})))
	}
	eng := db.collections["items"].(*columnarEngine)

	// Delete b and c so the dead fraction (2/4 = 0.5) crosses the threshold and
	// compaction runs on the second delete.
	require.NoError(t, db.Delete(ctx, dal.NewKeyWithID("items", "b")))
	require.NoError(t, db.Delete(ctx, dal.NewKeyWithID("items", "c")))

	require.Equal(t, 0, eng.deadCount, "dead slots reclaimed")
	require.Len(t, eng.live, 2)
	require.Empty(t, eng.freeList)

	// Every live record is still readable with correct values.
	var a, d item
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", "a"), &a)))
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", "d"), &d)))
	require.Equal(t, item{Name: "a", Count: 0}, a)
	require.Equal(t, item{Name: "d", Count: 3}, d)

	// Deleted records are gone.
	require.True(t, dal.IsNotFound(db.Get(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", "b"), &item{}))))
}

// TestColumnar_GetAndQueryReassemble verifies
// columnar-storage#ac:get-and-query-reassemble: reassembled rows equal stored
// data and share no references.
func TestColumnar_GetAndQueryReassemble(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB(WithSchema(false, WithCollection[doc]("docs", nil, WithColumnarStorage()))).(*database)
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("docs", "d1"),
		&doc{Title: "shared", Tags: []string{"x"}, Meta: address{City: "Paris"}})))
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("docs", "d2"),
		&doc{Title: "shared", Tags: []string{"y"}, Meta: address{City: "Lyon"}})))

	// Get into a typed target equals stored data.
	var got doc
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(dal.NewKeyWithID("docs", "d1"), &got)))
	require.Equal(t, doc{Title: "shared", Tags: []string{"x"}, Meta: address{City: "Paris"}}, got)

	// Query materializes typed rows that share no references.
	q := dal.From(dal.NewRootCollectionRef("docs", "")).NewQuery().
		WhereField("Title", dal.Equal, "shared").
		SelectIntoRecord(func() dal.Record {
			return dal.NewRecordWithIncompleteKey("docs", reflect.String, &doc{})
		})
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	records := readAll(t, reader)
	require.Len(t, records, 2)
	first := records[0].Data().(*doc)
	second := records[1].Data().(*doc)
	require.NotSame(t, first, second)
	first.Tags[0] = "MUTATED"
	require.NotEqual(t, "MUTATED", second.Tags[0])
}

// queryRow is a small typed record for the columnar-vs-serialized query
// equality test (comparable string column for the WHERE predicate).
type queryRow struct {
	Group string
	Name  string
	Rank  int
}

func seedQueryRows(t *testing.T, db *database, collection string) {
	t.Helper()
	ctx := context.Background()
	rows := []struct {
		id  string
		rec queryRow
	}{
		{"r1", queryRow{Group: "a", Name: "alice", Rank: 3}},
		{"r2", queryRow{Group: "b", Name: "bob", Rank: 1}},
		{"r3", queryRow{Group: "a", Name: "carol", Rank: 2}},
		{"r4", queryRow{Group: "a", Name: "dave", Rank: 5}},
	}
	for _, r := range rows {
		require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID(collection, r.id), &r.rec)))
	}
}

// TestColumnar_QueryMatchesSerialized verifies
// columnar-storage#ac:columnar-query-matches-serialized: the supported single
// equality WHERE plus projection and ORDER BY/LIMIT return identical content
// and order through both engines.
func TestColumnar_QueryMatchesSerialized(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	colDB := NewDB(WithSchema(false, WithCollection[queryRow]("q", nil, WithColumnarStorage()))).(*database)
	serDB := NewDB(WithSchema(false, WithCollection[queryRow]("q", nil, WithSerializedStorage()))).(*database)
	seedQueryRows(t, colDB, "q")
	seedQueryRows(t, serDB, "q")

	build := func() dal.Query {
		return dal.From(dal.NewRootCollectionRef("q", "")).NewQuery().
			WhereField("Group", dal.Equal, "a").
			OrderBy(dal.DescendingField("Rank")).
			Limit(2).
			SelectColumns(
				dal.Column{Expression: dal.NewFieldRef("", "Name")},
				dal.Column{Expression: dal.NewFieldRef("", "Rank")},
			)
	}

	colReader, err := colDB.ExecuteQueryToRecordsReader(ctx, build())
	require.NoError(t, err)
	serReader, err := serDB.ExecuteQueryToRecordsReader(ctx, build())
	require.NoError(t, err)

	colRecords := readAll(t, colReader)
	serRecords := readAll(t, serReader)
	require.Len(t, colRecords, 2)
	require.Equal(t, len(serRecords), len(colRecords))
	for i := range colRecords {
		require.Equal(t, serRecords[i].Key().ID, colRecords[i].Key().ID)
		require.Equal(t, serRecords[i].Data(), colRecords[i].Data())
	}
}

// TestColumnar_DefaultStrategyScansColumn verifies
// columnar-storage#ac:default-strategy-scans-column: the default strategy on a
// comparable string column returns exactly the matching live slots and does not
// return "no opinion".
func TestColumnar_DefaultStrategyScansColumn(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB(WithSchema(false, WithCollection[queryRow]("q", nil, WithColumnarStorage()))).(*database)
	seedQueryRows(t, db, "q")
	eng := db.collections["q"].(*columnarEngine)

	slots, ok := eng.byName["Group"].strategy.EqualSlots("a")
	require.True(t, ok, "comparable column has an opinion")
	require.Len(t, slots, 3)
	want := map[string]struct{}{"r1": {}, "r3": {}, "r4": {}}
	got := map[string]struct{}{}
	for slot := range slots {
		got[eng.slotToID[slot]] = struct{}{}
	}
	require.Equal(t, want, got)

	// A deleted record's slot is excluded from the strategy's answer.
	require.NoError(t, db.Delete(ctx, dal.NewKeyWithID("q", "r3")))
	slots, ok = eng.byName["Group"].strategy.EqualSlots("a")
	require.True(t, ok)
	require.Len(t, slots, 2)
}

// TestColumnar_DefaultStrategyNoOpinionOnAny verifies the default strategy
// returns "no opinion" for a non-comparable []any column.
func TestColumnar_DefaultStrategyNoOpinionOnAny(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := columnarItemDB(t)
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", "i1"),
		&item{Name: "a", Extra: []any{1, 2}})))
	eng := db.collections["items"].(*columnarEngine)

	_, ok := eng.byName["Extra"].strategy.EqualSlots([]any{1, 2})
	require.False(t, ok, "non-comparable []any column returns no opinion")
}

// recordingStrategy wraps the live column slice (via a back-reference resolved
// at SetValue time) and records the values written and the EqualSlots calls,
// so a test can assert the engine consulted the strategy. When opinion is
// false it always returns "no opinion" to exercise the scan fall-back.
type recordingStrategy struct {
	values  map[int]any
	queried []any
	opinion bool
	live    func(int) bool
}

func newRecordingStrategy(opinion bool, live func(int) bool) *recordingStrategy {
	return &recordingStrategy{values: map[int]any{}, opinion: opinion, live: live}
}

func (s *recordingStrategy) SetValue(slot int, value any) { s.values[slot] = value }

func (s *recordingStrategy) ClearValue(slot int) { delete(s.values, slot) }

func (s *recordingStrategy) EqualSlots(value any) (SlotSet, bool) {
	s.queried = append(s.queried, value)
	if !s.opinion {
		return nil, false
	}
	slots := make(SlotSet)
	for slot, v := range s.values {
		if s.live(slot) && v == value {
			slots[slot] = struct{}{}
		}
	}
	return slots, true
}

// TestColumnar_WhereUsesStrategy verifies
// columnar-storage#ac:where-uses-strategy: the query consults the column's
// strategy and uses its slot set, and the result matches the Serialized engine.
func TestColumnar_WhereUsesStrategy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	colDB := NewDB(WithSchema(false,
		WithCollection[queryRow]("q", nil, WithColumnarStorage()),
	)).(*database)
	// Install a recording strategy whose live-check is bound to the engine, so
	// the engine's write side populates it as rows are seeded.
	eng := colDB.engine("q").(*columnarEngine)
	stgy := newRecordingStrategy(true, func(slot int) bool { return eng.live[slot] })
	eng.byName["Group"].strategy = stgy
	eng.byName["Group"].defaultStgy = nil

	serDB := NewDB(WithSchema(false, WithCollection[queryRow]("q", nil, WithSerializedStorage()))).(*database)
	seedQueryRows(t, colDB, "q")
	seedQueryRows(t, serDB, "q")

	build := func() dal.Query {
		return dal.From(dal.NewRootCollectionRef("q", "")).NewQuery().
			WhereField("Group", dal.Equal, "a").
			SelectKeysOnly(reflect.String)
	}
	colReader, err := colDB.ExecuteQueryToRecordsReader(ctx, build())
	require.NoError(t, err)
	serReader, err := serDB.ExecuteQueryToRecordsReader(ctx, build())
	require.NoError(t, err)

	require.NotEmpty(t, stgy.queried, "the engine consulted the strategy's read side")
	require.Equal(t, []any{"a"}, stgy.queried)

	colIDs := recordIDs(readAll(t, colReader))
	serIDs := recordIDs(readAll(t, serReader))
	require.Equal(t, serIDs, colIDs)
	require.Equal(t, []string{"r1", "r3", "r4"}, colIDs)
}

// TestColumnar_WhereFallsBackOnNoOpinion verifies
// columnar-storage#ac:where-falls-back-on-no-opinion: when the strategy returns
// "no opinion", the engine scans and still returns the correct result.
func TestColumnar_WhereFallsBackOnNoOpinion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	colDB := NewDB(WithSchema(false, WithCollection[queryRow]("q", nil, WithColumnarStorage()))).(*database)
	eng := colDB.engine("q").(*columnarEngine)
	stgy := newRecordingStrategy(false, func(slot int) bool { return eng.live[slot] })
	eng.byName["Group"].strategy = stgy

	serDB := NewDB(WithSchema(false, WithCollection[queryRow]("q", nil, WithSerializedStorage()))).(*database)
	seedQueryRows(t, colDB, "q")
	seedQueryRows(t, serDB, "q")

	build := func() dal.Query {
		return dal.From(dal.NewRootCollectionRef("q", "")).NewQuery().
			WhereField("Group", dal.Equal, "a").
			SelectKeysOnly(reflect.String)
	}
	colReader, err := colDB.ExecuteQueryToRecordsReader(ctx, build())
	require.NoError(t, err)
	serReader, err := serDB.ExecuteQueryToRecordsReader(ctx, build())
	require.NoError(t, err)

	require.NotEmpty(t, stgy.queried, "the strategy was consulted before falling back")
	colIDs := recordIDs(readAll(t, colReader))
	serIDs := recordIDs(readAll(t, serReader))
	require.Equal(t, serIDs, colIDs)
	require.Equal(t, []string{"r1", "r3", "r4"}, colIDs)
}

func recordIDs(records []dal.Record) []string {
	ids := make([]string, len(records))
	for i, r := range records {
		ids[i] = r.Key().ID.(string)
	}
	return ids
}

// TestColumnar_RaceInterleavesOperations verifies
// columnar-storage#ac:columnar-passes-race: under the single global write lock,
// concurrent reads interleaved with writes, deletes (which trigger compaction),
// and queries report no data race and stay correct. Run with -race.
func TestColumnar_RaceInterleavesOperations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := columnarItemDB(t)

	const n = 40
	ids := make([]string, n)
	for i := range ids {
		ids[i] = "k" + strconv.Itoa(i)
		require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", ids[i]), &item{Name: ids[i], Count: i, Active: i%2 == 0})))
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
					var got item
					_ = db.Get(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", id), &got))
				case 1:
					_ = db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", id), &item{Name: id, Count: round}))
				case 2:
					_ = db.Delete(ctx, dal.NewKeyWithID("items", id))
				case 3:
					q := dal.From(dal.NewRootCollectionRef("items", "")).NewQuery().
						WhereField("Active", dal.Equal, true).
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

	// The engine remains internally consistent: every id->slot maps to a live
	// slot, and a query still returns correct results.
	eng := db.collections["items"].(*columnarEngine)
	for id, slot := range eng.idToSlot {
		require.True(t, eng.live[slot], "id %s maps to a live slot", id)
	}
	q := dal.From(dal.NewRootCollectionRef("items", "")).NewQuery().SelectKeysOnly(reflect.String)
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	require.Len(t, readAll(t, reader), len(eng.idToSlot))
}
