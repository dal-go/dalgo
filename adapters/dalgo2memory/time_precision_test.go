package dalgo2memory

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
	"github.com/stretchr/testify/require"
)

// nanoTime has a sub-microsecond part that real storage drops.
var nanoTime = time.Date(2026, 11, 4, 10, 11, 12, 123456789, time.UTC)

// microTime is nanoTime as real storage returns it.
var microTime = dal.NormalizeTime(nanoTime)

type stampedInner struct {
	At    time.Time
	AtPtr *time.Time
}

type StampedEmbedded struct {
	EmbeddedAt time.Time
}

type stampedRecord struct {
	Name      string
	At        time.Time
	AtPtr     *time.Time
	Nested    stampedInner
	NestedPtr *stampedInner
	Times     []time.Time
	Arr       [2]time.Time
	ByName    map[string]time.Time
	Extra     map[string]any
	Any       any
}

func newStampedRecord() *stampedRecord {
	at := nanoTime
	innerAt := nanoTime
	return &stampedRecord{
		Name:      "n",
		At:        nanoTime,
		AtPtr:     &at,
		Nested:    stampedInner{At: nanoTime},
		NestedPtr: &stampedInner{At: nanoTime, AtPtr: &innerAt},
		Times:     []time.Time{nanoTime, microTime},
		Arr:       [2]time.Time{microTime, nanoTime},
		ByName:    map[string]time.Time{"a": nanoTime},
		Extra:     map[string]any{"at": nanoTime, "deep": map[string]any{"list": []any{nanoTime}}},
		Any:       nanoTime,
	}
}

// requireStampedTruncated asserts every time in a read-back record is microTime.
func requireStampedTruncated(t *testing.T, got *stampedRecord) {
	t.Helper()
	micro := microTime.Format(time.RFC3339Nano)
	require.True(t, got.At.Equal(microTime), "At %v", got.At)
	require.True(t, got.AtPtr.Equal(microTime), "AtPtr %v", got.AtPtr)
	require.True(t, got.Nested.At.Equal(microTime), "Nested.At %v", got.Nested.At)
	require.True(t, got.NestedPtr.At.Equal(microTime), "NestedPtr.At %v", got.NestedPtr.At)
	require.True(t, got.NestedPtr.AtPtr.Equal(microTime), "NestedPtr.AtPtr %v", got.NestedPtr.AtPtr)
	require.True(t, got.Times[0].Equal(microTime) && got.Times[1].Equal(microTime), "Times %v", got.Times)
	require.True(t, got.Arr[0].Equal(microTime) && got.Arr[1].Equal(microTime), "Arr %v", got.Arr)
	require.True(t, got.ByName["a"].Equal(microTime), "ByName %v", got.ByName)
	require.Equal(t, micro, got.Extra["at"])
	require.Equal(t, micro, got.Extra["deep"].(map[string]any)["list"].([]any)[0])
	require.Equal(t, micro, got.Any)

	// An un-normalised copy no longer matches what storage returned; a
	// normalised one does.
	require.False(t, got.At.Equal(nanoTime), "exact comparison with the nanosecond original must fail")
	require.NotEqual(t, nanoTime, got.At)
	require.Equal(t, dal.NormalizeTime(nanoTime), got.At)
}

func TestTimePrecision_StructRecordRoundTrip(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		db   func() *database
	}{
		{"serialized schemaless", func() *database { return newDatabase() }},
		{"serialized typed", func() *database {
			return newDatabase(WithSchema(false, WithCollection[stampedRecord]("stamped", nil, WithSerializedStorage())))
		}},
		{"columnar", func() *database {
			return newDatabase(WithSchema(false, WithCollection[stampedRecord]("stamped", nil, WithColumnarStorage())))
		}},
		{"columnar sharing references", func() *database {
			return newDatabase(WithSchema(false, WithCollection[stampedRecord]("stamped", nil,
				WithColumnarStorage(WithColumnarRefBreaking(false)))))
		}},
		{"optimistic", func() *database { return newDatabase(WithOptimisticConcurrency()) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := tc.db()
			key := record.NewKeyWithID("stamped", "r1")
			written := newStampedRecord()
			require.NoError(t, db.Set(ctx, record.NewRecordWithData(key, written)))

			// The caller's own value is never mutated.
			require.Equal(t, newStampedRecord(), written)

			got := new(stampedRecord)
			require.NoError(t, db.Get(ctx, record.NewRecordWithData(key, got)))
			requireStampedTruncated(t, got)

			// Queries materialize the same truncated values.
			reader, err := db.ExecuteQueryToRecordsReader(ctx, dal.From(dal.NewRootCollectionRef("stamped", "")).
				NewQuery().SelectIntoRecord(func() record.Record {
				return record.NewRecordWithIncompleteKey("stamped", reflect.String, new(stampedRecord))
			}))
			require.NoError(t, err)
			rec, err := reader.Next()
			require.NoError(t, err)
			requireStampedTruncated(t, rec.Data().(*stampedRecord))
		})
	}
}

func TestTimePrecision_MapRecordRoundTrip(t *testing.T) {
	ctx := context.Background()
	micro := microTime.Format(time.RFC3339Nano)
	for _, tc := range []struct {
		name string
		db   func() *database
	}{
		{"serialized", func() *database { return newDatabase() }},
		{"columnar mixed", func() *database {
			return newDatabase(WithSchema(false, WithCollection[map[string]any]("m", nil,
				WithColumnarStorage(WithDeclaredColumn[time.Time]("declared")))))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := tc.db()
			key := record.NewKeyWithID("m", "r1")
			written := map[string]any{
				"declared": nanoTime,
				"at":       nanoTime,
				"nested":   map[string]any{"at": nanoTime, "list": []any{nanoTime, "text"}},
			}
			require.NoError(t, db.Set(ctx, record.NewRecordWithData(key, written)))
			require.Equal(t, nanoTime, written["at"], "the caller's map is not mutated")

			got := map[string]any{}
			require.NoError(t, db.Get(ctx, record.NewRecordWithData(key, &got)))
			require.Equal(t, micro, got["declared"])
			require.Equal(t, micro, got["at"])
			nested := got["nested"].(map[string]any)
			require.Equal(t, micro, nested["at"])
			require.Equal(t, []any{micro, "text"}, nested["list"])
			require.NotEqual(t, nanoTime.Format(time.RFC3339Nano), got["at"])
		})
	}
}

func TestTimePrecision_UpdateTruncates(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		db   func() *database
	}{
		{"serialized", func() *database { return newDatabase(WithInterleavedReadsAndWritesInTransaction()) }},
		{"columnar", func() *database {
			return newDatabase(WithInterleavedReadsAndWritesInTransaction(),
				WithSchema(false, WithCollection[stampedRecord]("stamped", nil, WithColumnarStorage())))
		}},
		{"optimistic", func() *database {
			return newDatabase(WithOptimisticConcurrency(), WithInterleavedReadsAndWritesInTransaction())
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := tc.db()
			key := record.NewKeyWithID("stamped", "r1")
			require.NoError(t, db.Set(ctx, record.NewRecordWithData(key, &stampedRecord{Name: "n"})))
			require.NoError(t, db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
				if err := tx.Update(ctx, key, []update.Update{
					update.ByFieldName("At", nanoTime),
					update.ByFieldPath([]string{"Nested", "At"}, nanoTime),
				}); err != nil {
					return err
				}
				// Reads inside the transaction already see the stored precision.
				got := new(stampedRecord)
				if err := tx.Get(ctx, record.NewRecordWithData(key, got)); err != nil {
					return err
				}
				require.Equal(t, microTime, got.At)
				return nil
			}))
			got := new(stampedRecord)
			require.NoError(t, db.Get(ctx, record.NewRecordWithData(key, got)))
			require.Equal(t, microTime, got.At)
			require.Equal(t, microTime, got.Nested.At)
		})
	}
}

func TestTimePrecision_OptimisticInsertReadsTruncated(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency(), WithInterleavedReadsAndWritesInTransaction())
	key := record.NewKeyWithID("stamped", "r1")
	require.NoError(t, db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		if err := tx.Insert(ctx, record.NewRecordWithData(key, newStampedRecord())); err != nil {
			return err
		}
		got := new(stampedRecord)
		if err := tx.Get(ctx, record.NewRecordWithData(key, got)); err != nil {
			return err
		}
		requireStampedTruncated(t, got)
		return nil
	}))
}

func TestTimePrecision_QueryBoundIsTruncatedLikeStorage(t *testing.T) {
	for _, tc := range []struct {
		name string
		db   func() *database
	}{
		{"serialized", func() *database { return newDatabase() }},
		{"columnar", func() *database {
			return newDatabase(WithSchema(false, WithCollection[timedThing]("timed", nil, WithColumnarStorage())))
		}},
		{"columnar sharing references", func() *database {
			return newDatabase(WithSchema(false, WithCollection[timedThing]("timed", nil,
				WithColumnarStorage(WithColumnarRefBreaking(false)))))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := tc.db()
			require.NoError(t, db.Insert(context.Background(),
				record.NewRecordWithData(record.NewKeyWithID("timed", "r1"), &timedThing{DtNext: nanoTime})))
			for _, op := range []dal.Operator{dal.Equal, dal.GreaterOrEqual, dal.LessOrEqual} {
				ids := queryTimedThings(t, db, func(q *dal.QueryBuilder) dal.IQueryBuilder { return q.WhereField("DtNext", op, nanoTime) })
				require.Equal(t, []string{"r1"}, ids, "operator %s", op)
			}
			// Inside an AND group as well.
			ids := queryTimedThings(t, db, func(q *dal.QueryBuilder) dal.IQueryBuilder {
				return q.Where(dal.NewGroupCondition(dal.And,
					dal.NewComparison(dal.Field("DtNext"), dal.Equal, dal.NewConstant(nanoTime)),
					dal.NewComparison(dal.Field("DtNext"), dal.LessOrEqual, dal.NewConstant(nanoTime)),
				))
			})
			require.Equal(t, []string{"r1"}, ids)
		})
	}
}

func TestTruncateConditionTimes(t *testing.T) {
	require.Nil(t, truncateConditionTimes(nil))
	isNull := dal.NewIsNullCondition(dal.Field("At"))
	require.Equal(t, dal.Condition(isNull), truncateConditionTimes(isNull))

	got := truncateConditionTimes(dal.NewGroupCondition(dal.Or,
		dal.NewComparison(dal.Field("At"), dal.Equal, dal.NewConstant(nanoTime)),
		dal.NewComparison(dal.NewConstant(nanoTime), dal.In, dal.Field("List")),
		dal.NewComparison(dal.Field("At"), dal.In, dal.Array{Value: []any{nanoTime, "x"}}),
	)).(dal.GroupCondition)
	require.EqualValues(t, dal.Or, got.Operator())
	children := got.Conditions()
	require.Equal(t, dal.Field("At"), children[0].(dal.Comparison).Left)
	require.Equal(t, microTime, children[0].(dal.Comparison).Right.(dal.Constant).Value)
	require.Equal(t, microTime, children[1].(dal.Comparison).Left.(dal.Constant).Value)
	require.Equal(t, []any{microTime, "x"}, children[2].(dal.Comparison).Right.(dal.Array).Value)
}

type hiddenInner struct {
	At time.Time
}

type withHiddenEmbedded struct {
	hiddenInner
	Name string
}

type withHiddenEmbeddedPtr struct {
	*hiddenInner
	Name string
}

func TestTimePrecision_UnexportedEmbeddedStructs(t *testing.T) {
	ctx := context.Background()
	db := newDatabase()

	byValue := withHiddenEmbedded{hiddenInner: hiddenInner{At: nanoTime}, Name: "v"}
	key := record.NewKeyWithID("hidden", "v")
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(key, &byValue)))
	require.Equal(t, nanoTime, byValue.At, "input not mutated")
	var gotValue withHiddenEmbedded
	require.NoError(t, db.Get(ctx, record.NewRecordWithData(key, &gotValue)))
	require.Equal(t, microTime, gotValue.At)

	byPtr := withHiddenEmbeddedPtr{hiddenInner: &hiddenInner{At: nanoTime}, Name: "p"}
	key = record.NewKeyWithID("hidden", "p")
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(key, &byPtr)))
	require.Equal(t, nanoTime, byPtr.At, "input not mutated")
	gotPtr := withHiddenEmbeddedPtr{hiddenInner: &hiddenInner{}}
	require.NoError(t, db.Get(ctx, record.NewRecordWithData(key, &gotPtr)))
	require.Equal(t, microTime, gotPtr.At)

	// A non-addressable value (held by an interface) is walked through a copy.
	got := truncateTimes([]any{withHiddenEmbedded{hiddenInner: hiddenInner{At: nanoTime}}}).([]any)
	require.Equal(t, microTime, got[0].(withHiddenEmbedded).At)
	// Unchanged unexported embedded values are returned as they are.
	unchanged := &withHiddenEmbeddedPtr{hiddenInner: &hiddenInner{At: microTime}}
	require.Same(t, unchanged, truncateTimes(unchanged))
}

type fork struct {
	A, B *fork
	At   time.Time
}

func TestTimePrecision_CyclicValueFailsFast(t *testing.T) {
	n := &fork{At: nanoTime}
	n.A, n.B = n, n
	done := make(chan error, 1)
	go func() {
		done <- newDatabase().Set(context.Background(), record.NewRecordWithData(record.NewKeyWithID("fork", "f"), n))
	}()
	select {
	case err := <-done:
		require.ErrorContains(t, err, "cycle")
	case <-time.After(5 * time.Second):
		t.Fatal("Set of a cyclic value did not return")
	}
	require.Equal(t, nanoTime, n.At, "input not mutated")
}

type noTimes struct {
	Name  string
	Items []struct{ N int }
	Next  *noTimes
	priv  time.Time
}

func TestMayHoldTime(t *testing.T) {
	require.True(t, mayHoldTime(timeType))
	require.True(t, mayHoldTime(reflect.TypeOf([]any{})))
	require.True(t, mayHoldTime(reflect.TypeOf(map[string]*stampedInner{})))
	require.True(t, mayHoldTime(reflect.TypeOf(withHiddenEmbeddedPtr{})))
	require.False(t, mayHoldTime(reflect.TypeOf(noTimes{})))
	require.False(t, mayHoldTime(reflect.TypeOf(noTimes{})), "cached answer")
	require.False(t, mayHoldTime(reflect.TypeOf([]byte{})))
	require.False(t, mayHoldTime(reflect.TypeOf(struct{ hidden int }{})))
	require.False(t, mayHoldTime(reflect.TypeOf(struct{ int }{})))
	rec := &noTimes{Name: "n", priv: nanoTime}
	require.Same(t, rec, truncateTimes(rec))
}

type selfRef struct {
	Next *selfRef
	At   time.Time
}

type withHidden struct {
	At     time.Time
	hidden time.Time
}

func TestTruncateTimes(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		require.Nil(t, truncateTimes(nil))
	})
	t.Run("unchanged values are returned as they are", func(t *testing.T) {
		rec := &stampedRecord{At: microTime, Times: []time.Time{microTime}, ByName: map[string]time.Time{"a": microTime}}
		require.Same(t, rec, truncateTimes(rec))
		m := map[string]any{"a": microTime, "b": nil, "c": []byte("x"), "d": map[string]int{"x": 1}, "e": (*time.Time)(nil)}
		require.Equal(t, reflect.ValueOf(m).Pointer(), reflect.ValueOf(truncateTimes(m)).Pointer())
		s := []any{nil, []string(nil), map[string]any(nil), [1]int{1}}
		require.Equal(t, reflect.ValueOf(s).Pointer(), reflect.ValueOf(truncateTimes(s)).Pointer())
		require.Equal(t, 5, truncateTimes(5))
	})
	t.Run("embedded struct", func(t *testing.T) {
		got := truncateTimes(struct{ StampedEmbedded }{StampedEmbedded{EmbeddedAt: nanoTime}}).(struct{ StampedEmbedded })
		require.Equal(t, microTime, got.EmbeddedAt)
	})
	t.Run("time and pointer to time", func(t *testing.T) {
		require.Equal(t, microTime, truncateTimes(nanoTime))
		at := nanoTime
		got := truncateTimes(&at).(*time.Time)
		require.Equal(t, microTime, *got)
		require.Equal(t, nanoTime, at, "input not mutated")
	})
	t.Run("unexported fields are kept but not truncated", func(t *testing.T) {
		got := truncateTimes(withHidden{At: nanoTime, hidden: nanoTime}).(withHidden)
		require.Equal(t, microTime, got.At)
		require.Equal(t, nanoTime, got.hidden)
	})
	t.Run("a value shared twice is truncated once and stays shared", func(t *testing.T) {
		shared := &stampedInner{At: nanoTime}
		got := truncateTimes([]any{shared, shared, []any{}}).([]any)
		require.Same(t, got[0], got[1])
		require.Equal(t, microTime, got[0].(*stampedInner).At)
		require.Equal(t, nanoTime, shared.At)
	})
	t.Run("only changed container entries are replaced", func(t *testing.T) {
		shared := &stampedInner{At: microTime}
		in := []any{shared, nanoTime}
		got := truncateTimes(in).([]any)
		require.Same(t, shared, got[0])
		require.Equal(t, microTime, got[1])
		require.Equal(t, nanoTime, in[1])
	})
	t.Run("self-referencing value terminates", func(t *testing.T) {
		n := &selfRef{At: nanoTime}
		n.Next = n
		got := truncateTimes(n).(*selfRef)
		require.Equal(t, microTime, got.At)
		require.Equal(t, nanoTime, n.At)
	})
}
