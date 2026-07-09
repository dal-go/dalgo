package dalgo2memory

import (
	"context"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/update"
	"github.com/stretchr/testify/require"
)

// tagged exercises jsonFieldName's tag handling: a renamed field, an
// option-only tag (",omitempty" -> field name), and a skipped field
// (json:"-"). It also carries reference-bearing types for isRefBearing.
type tagged struct {
	Renamed    string         `json:"renamed"`
	OnlyOpts   int            `json:",omitempty"`
	Skipped    string         `json:"-"`
	unexported int            //nolint:unused // proves unexported fields get no column
	Mapping    map[string]int `json:"mapping"`
}

// TestColumnar_BuildColumnsTagsAndRefTypes covers jsonFieldName (tag variants,
// unexported skip) and the reference-bearing detection over a map field.
func TestColumnar_BuildColumnsTagsAndRefTypes(t *testing.T) {
	t.Parallel()
	db := NewDB(WithSchema(false, WithCollection[tagged]("t", nil, WithColumnarStorage()))).(*database)
	eng := db.engine("t").(*columnarEngine)

	require.Contains(t, eng.byName, "renamed")
	require.Contains(t, eng.byName, "OnlyOpts")
	require.Contains(t, eng.byName, "mapping")
	require.NotContains(t, eng.byName, "Skipped")
	require.NotContains(t, eng.byName, "unexported")
	require.True(t, eng.byName["mapping"].refBearing)
	require.False(t, eng.byName["renamed"].refBearing)
}

// refShapes exercises isRefBearing's array, channel, and nested-struct cases,
// and the "struct with no ref-bearing field" false branch.
type refShapes struct {
	ArrayOfSlices [2][]int
	Plain         scalarsOnly
}

type scalarsOnly struct {
	A int
	B string
}

func TestColumnar_IsRefBearingShapes(t *testing.T) {
	t.Parallel()
	require.True(t, isRefBearing(reflect.TypeOf([2][]int{})))     // array of slices
	require.True(t, isRefBearing(reflect.TypeOf(make(chan int)))) // channel
	require.False(t, isRefBearing(reflect.TypeOf(scalarsOnly{}))) // struct, no refs
	require.True(t, isRefBearing(reflect.TypeOf(refShapes{})))    // struct containing a ref
	require.False(t, isRefBearing(reflect.TypeOf([3]int{})))      // array of scalars
}

// TestColumnar_StructTypeOfErrors covers structTypeOf's nil-factory and
// non-struct branches.
func TestColumnar_StructTypeOfErrors(t *testing.T) {
	t.Parallel()
	_, err := structTypeOf(nil)
	require.Error(t, err)

	_, err = structTypeOf(func() any { return new(int) })
	require.Error(t, err)

	typ, err := structTypeOf(func() any { return &user{} })
	require.NoError(t, err)
	require.Equal(t, reflect.TypeOf(user{}), typ)
}

// TestColumnar_ExplicitStrategyOption covers the WithColumnStrategy path in
// buildColumns (an explicit strategy is installed and the default is not).
func TestColumnar_ExplicitStrategyOption(t *testing.T) {
	t.Parallel()
	stgy := newRecordingStrategy(true, func(int) bool { return true })
	db := NewDB(WithSchema(false,
		WithCollection[user]("users", nil, WithColumnarStorage(WithColumnStrategy("Role", stgy))),
	)).(*database)
	eng := db.engine("users").(*columnarEngine)
	require.Same(t, stgy, eng.byName["Role"].strategy)
	require.Nil(t, eng.byName["Role"].defaultStgy)
	require.NotNil(t, eng.byName["Name"].defaultStgy)
}

// TestColumnar_CandidateSlotsNonMatchingShapes covers candidateSlots' early
// returns for condition shapes the strategy path does not handle.
func TestColumnar_CandidateSlotsNonMatchingShapes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := columnarItemDB(t)
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", "i1"), &item{Name: "a"})))
	eng := db.collections["items"].(*columnarEngine)

	// nil condition -> not a Comparison.
	_, ok := eng.candidateSlots(nil)
	require.False(t, ok)

	// Non-equal operator.
	_, ok = eng.candidateSlots(dal.Comparison{Operator: dal.GreaterThen,
		Left: dal.NewFieldRef("", "Count"), Right: dal.NewConstant(1)})
	require.False(t, ok)

	// Left is not a FieldRef.
	_, ok = eng.candidateSlots(dal.Comparison{Operator: dal.Equal,
		Left: dal.NewConstant(1), Right: dal.NewConstant(1)})
	require.False(t, ok)

	// Right is not a Constant.
	_, ok = eng.candidateSlots(dal.Comparison{Operator: dal.Equal,
		Left: dal.NewFieldRef("", "Count"), Right: dal.NewFieldRef("", "Count")})
	require.False(t, ok)

	// Field references an unknown column.
	_, ok = eng.candidateSlots(dal.Comparison{Operator: dal.Equal,
		Left: dal.NewFieldRef("", "Missing"), Right: dal.NewConstant(1)})
	require.False(t, ok)
}

// TestColumnar_ValuesEqualNonComparableQueried covers
// valuesEqualForStrategy's guard against a non-comparable queried value.
func TestColumnar_ValuesEqualNonComparableQueried(t *testing.T) {
	t.Parallel()
	require.False(t, valuesEqualForStrategy("x", []int{1}))
	require.True(t, valuesEqualForStrategy("x", "x"))
	require.False(t, valuesEqualForStrategy("x", nil))
}

// TestColumnar_MaybeCompactNoTrigger covers maybeCompact's early returns: no
// slots, no dead slots, and a dead fraction below the threshold.
func TestColumnar_MaybeCompactNoTrigger(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := columnarItemDB(t)
	eng := db.engine("items").(*columnarEngine)

	// No slots at all.
	eng.maybeCompact()
	require.Empty(t, eng.live)

	// Three live, delete one: dead fraction 1/3 < 0.5, so no compaction.
	for _, id := range []string{"a", "b", "c"} {
		require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", id), &item{Name: id})))
	}
	require.NoError(t, db.Delete(ctx, dal.NewKeyWithID("items", "a")))
	require.Equal(t, 1, eng.deadCount)
	require.Len(t, eng.live, 3, "no compaction below threshold")

	// deadCount == 0 early return.
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", "d"), &item{Name: "d"})))
	require.Equal(t, 0, eng.deadCount)
	eng.maybeCompact()
	require.Len(t, eng.live, 3)
}

// TestColumnar_CompactRebuildsExternalStrategy covers compact()/rebuildStrategies
// for an explicit (non-default) strategy: after compaction the strategy is
// re-synced from the compacted slices.
func TestColumnar_CompactRebuildsExternalStrategy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB(WithSchema(false, WithCollection[queryRow]("q", nil, WithColumnarStorage()))).(*database)
	eng := db.engine("q").(*columnarEngine)
	stgy := newRecordingStrategy(true, func(slot int) bool { return slot < len(eng.live) && eng.live[slot] })
	eng.byName["Group"].strategy = stgy
	eng.byName["Group"].defaultStgy = nil
	seedQueryRows(t, db, "q")

	// Delete two of four (>= threshold) to force compaction.
	require.NoError(t, db.Delete(ctx, dal.NewKeyWithID("q", "r2")))
	require.NoError(t, db.Delete(ctx, dal.NewKeyWithID("q", "r4")))
	require.Equal(t, 0, eng.deadCount, "compaction reclaimed dead slots")

	// The external strategy, rebuilt from compacted slices, still answers
	// correctly for the surviving rows.
	slots, ok := stgy.EqualSlots("a")
	require.True(t, ok)
	require.Len(t, slots, 2) // r1 and r3 survive, both Group "a"
}

// TestColumnar_UpdateOnDefaultStrategyColumn covers update()'s read-modify-write
// over an existing record, and the non-error decode path.
func TestColumnar_UpdateRoundTrips(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := columnarItemDB(t)
	key := dal.NewKeyWithID("items", "i1")
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key, &item{Name: "a", Count: 1, Active: true})))
	require.NoError(t, db.Update(ctx, key, []update.Update{update.ByFieldName("Count", 9)}))
	var got item
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &got)))
	require.Equal(t, item{Name: "a", Count: 9, Active: true}, got)
}

// TestColumnar_NonSerializableWriteErrors covers decodeFields' marshal-error
// branch (parity with the Serialized engine): a channel value is rejected.
func TestColumnar_NonSerializableWriteErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := columnarItemDB(t)
	err := db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", "i1"),
		map[string]any{"Extra": make(chan int)}))
	require.Error(t, err)
	require.ErrorContains(t, err, "json")
}

// TestColumnar_RowMarshalErrorPaths covers the marshal/unmarshal error branches
// of rowData, materializeSlot, and buildRows by poking a non-serializable value
// into an []any column cell at a live slot.
func TestColumnar_RowMarshalErrorPaths(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := columnarItemDB(t)
	key := dal.NewKeyWithID("items", "i1")
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key, &item{Name: "a"})))
	eng := db.collections["items"].(*columnarEngine)
	slot := eng.idToSlot["i1"]

	// Force a non-serializable value into the []any "Extra" column cell.
	eng.byName["Extra"].values.Index(slot).Set(reflect.ValueOf(any(make(chan int))))

	_, err := eng.rowData(slot)
	require.Error(t, err)

	require.Error(t, eng.materializeSlot(slot, &item{}))

	_, err = eng.rows()
	require.Error(t, err)

	// candidateRows surfaces the same reassembly error when it has an opinion.
	_, _, err = eng.candidateRows(dal.Comparison{Operator: dal.Equal,
		Left: dal.NewFieldRef("", "Name"), Right: dal.NewConstant("a")})
	require.Error(t, err)

	// load surfaces the reassembly error too.
	require.Error(t, db.Get(ctx, dal.NewRecordWithData(key, &item{})))
}

// TestColumnar_MaterializeUnmarshalError covers materializeSlot's final
// json.Unmarshal error branch: a stored string cell cannot decode into an int
// target field.
func TestColumnar_MaterializeUnmarshalError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := columnarItemDB(t)
	key := dal.NewKeyWithID("items", "i1")
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key, &item{Name: "a", Count: 3})))
	eng := db.collections["items"].(*columnarEngine)
	slot := eng.idToSlot["i1"]

	// Put a string into the []any Extra column, then materialize into a target
	// whose Extra is typed int — the unmarshal fails.
	eng.byName["Extra"].values.Index(slot).Set(reflect.ValueOf(any("not-an-int")))
	type intExtra struct {
		Name  string
		Count int
		Extra int
	}
	require.Error(t, eng.materializeSlot(slot, &intExtra{}))
}

// TestColumnar_DecodeFieldsAssignError covers assignInto's json.Unmarshal error
// path via update: assigning a string into an int column is rejected.
func TestColumnar_DecodeFieldsAssignError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := columnarItemDB(t)
	key := dal.NewKeyWithID("items", "i1")
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key, &item{Name: "a", Count: 1})))
	err := db.Update(ctx, key, []update.Update{update.ByFieldName("Count", "not-an-int")})
	require.Error(t, err)
}

// TestColumnar_UpdateOnBrokenEngine covers update()'s initErr branch.
func TestColumnar_UpdateOnBrokenEngine(t *testing.T) {
	t.Parallel()
	db := NewDB(WithSchema(false,
		WithCollection[map[string]any]("blobs", nil, WithColumnarStorage()),
	)).(*database)
	eng := db.engine("blobs").(*columnarEngine)
	require.Error(t, eng.update("x", []update.Update{update.ByFieldName("a", 1)}))
}

// TestColumnar_DecodeFieldsNonObjectErrors covers decodeFields' unmarshal-to-map
// error branch: a record whose data marshals to a JSON array, not an object.
func TestColumnar_DecodeFieldsNonObjectErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := columnarItemDB(t)
	err := db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", "i1"), []int{1, 2, 3}))
	require.Error(t, err)
}

// TestColumnar_UpdateRowDataError covers update()'s rowData error branch: a
// non-serializable existing cell makes the read-modify-write read fail.
func TestColumnar_UpdateRowDataError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := columnarItemDB(t)
	key := dal.NewKeyWithID("items", "i1")
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key, &item{Name: "a"})))
	eng := db.collections["items"].(*columnarEngine)
	slot := eng.idToSlot["i1"]
	eng.byName["Extra"].values.Index(slot).Set(reflect.ValueOf(any(make(chan int))))

	err := db.Update(ctx, key, []update.Update{update.ByFieldName("Name", "b")})
	require.Error(t, err)
}

// TestColumnar_QueryReassemblyErrorPropagates covers loadCandidateRows' error
// return: a non-serializable cell makes candidateRows fail during a query.
func TestColumnar_QueryReassemblyErrorPropagates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := columnarItemDB(t)
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("items", "i1"), &item{Name: "a"})))
	eng := db.collections["items"].(*columnarEngine)
	slot := eng.idToSlot["i1"]
	eng.byName["Extra"].values.Index(slot).Set(reflect.ValueOf(any(make(chan int))))

	q := dal.From(dal.NewRootCollectionRef("items", "")).NewQuery().
		WhereField("Name", dal.Equal, "a").
		SelectKeysOnly(reflect.String)
	_, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.Error(t, err)
}

// TestColumnar_InterfaceGuards documents the compile-time interface guards.
func TestColumnar_InterfaceGuards(t *testing.T) {
	t.Parallel()
	var _ storageEngine = (*columnarEngine)(nil)
	var _ ColumnStrategy = (*typedSliceStrategy)(nil)
}
