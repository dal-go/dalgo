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
		{"serialized schemaless", func() *database { return newDatabase(firestoreTimes()) }},
		{"serialized typed", func() *database {
			return newDatabase(firestoreTimes(), WithSchema(false, WithCollection[stampedRecord]("stamped", nil, WithSerializedStorage())))
		}},
		{"columnar", func() *database {
			return newDatabase(firestoreTimes(), WithSchema(false, WithCollection[stampedRecord]("stamped", nil, WithColumnarStorage())))
		}},
		{"columnar sharing references", func() *database {
			return newDatabase(firestoreTimes(), WithSchema(false, WithCollection[stampedRecord]("stamped", nil,
				WithColumnarStorage(WithColumnarRefBreaking(false)))))
		}},
		{"optimistic", func() *database { return newDatabase(firestoreTimes(), WithOptimisticConcurrency()) }},
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
		{"serialized", func() *database { return newDatabase(firestoreTimes()) }},
		{"columnar mixed", func() *database {
			return newDatabase(firestoreTimes(), WithSchema(false, WithCollection[map[string]any]("m", nil,
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
		{"serialized", func() *database { return newDatabase(firestoreTimes(), WithInterleavedReadsAndWritesInTransaction()) }},
		{"columnar", func() *database {
			return newDatabase(firestoreTimes(), WithInterleavedReadsAndWritesInTransaction(),
				WithSchema(false, WithCollection[stampedRecord]("stamped", nil, WithColumnarStorage())))
		}},
		{"optimistic", func() *database {
			return newDatabase(firestoreTimes(), WithOptimisticConcurrency(), WithInterleavedReadsAndWritesInTransaction())
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
	db := newDatabase(firestoreTimes(), WithOptimisticConcurrency(), WithInterleavedReadsAndWritesInTransaction())
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
		{"serialized", func() *database { return newDatabase(firestoreTimes()) }},
		{"columnar", func() *database {
			return newDatabase(firestoreTimes(), WithSchema(false, WithCollection[timedThing]("timed", nil, WithColumnarStorage())))
		}},
		{"columnar sharing references", func() *database {
			return newDatabase(firestoreTimes(), WithSchema(false, WithCollection[timedThing]("timed", nil,
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
	require.Nil(t, truncateConditionTimes(nil, firestoreTimePrecision))
	isNull := dal.NewIsNullCondition(dal.Field("At"))
	require.Equal(t, dal.Condition(isNull), truncateConditionTimes(isNull, firestoreTimePrecision))

	got := truncateConditionTimes(dal.NewGroupCondition(dal.Or,
		dal.NewComparison(dal.Field("At"), dal.Equal, dal.NewConstant(nanoTime)),
		dal.NewComparison(dal.NewConstant(nanoTime), dal.In, dal.Field("List")),
		dal.NewComparison(dal.Field("At"), dal.In, dal.Array{Value: []any{nanoTime, "x"}}),
	), firestoreTimePrecision).(dal.GroupCondition)
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
	db := newDatabase(firestoreTimes())

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
	got := truncateTimes([]any{withHiddenEmbedded{hiddenInner: hiddenInner{At: nanoTime}}}, firestoreTimePrecision).([]any)
	require.Equal(t, microTime, got[0].(withHiddenEmbedded).At)
	// Unchanged unexported embedded values are returned as they are.
	unchanged := &withHiddenEmbeddedPtr{hiddenInner: &hiddenInner{At: microTime}}
	require.Same(t, unchanged, truncateTimes(unchanged, firestoreTimePrecision))
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
		done <- newDatabase(firestoreTimes()).Set(context.Background(), record.NewRecordWithData(record.NewKeyWithID("fork", "f"), n))
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
	require.Same(t, rec, truncateTimes(rec, firestoreTimePrecision))
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
		require.Nil(t, truncateTimes(nil, firestoreTimePrecision))
	})
	t.Run("unchanged values are returned as they are", func(t *testing.T) {
		rec := &stampedRecord{At: microTime, Times: []time.Time{microTime}, ByName: map[string]time.Time{"a": microTime}}
		require.Same(t, rec, truncateTimes(rec, firestoreTimePrecision))
		m := map[string]any{"a": microTime, "b": nil, "c": []byte("x"), "d": map[string]int{"x": 1}, "e": (*time.Time)(nil)}
		require.Equal(t, reflect.ValueOf(m).Pointer(), reflect.ValueOf(truncateTimes(m, firestoreTimePrecision)).Pointer())
		s := []any{nil, []string(nil), map[string]any(nil), [1]int{1}}
		require.Equal(t, reflect.ValueOf(s).Pointer(), reflect.ValueOf(truncateTimes(s, firestoreTimePrecision)).Pointer())
		require.Equal(t, 5, truncateTimes(5, firestoreTimePrecision))
	})
	t.Run("embedded struct", func(t *testing.T) {
		got := truncateTimes(struct{ StampedEmbedded }{StampedEmbedded{EmbeddedAt: nanoTime}}, firestoreTimePrecision).(struct{ StampedEmbedded })
		require.Equal(t, microTime, got.EmbeddedAt)
	})
	t.Run("time and pointer to time", func(t *testing.T) {
		require.Equal(t, microTime, truncateTimes(nanoTime, firestoreTimePrecision))
		at := nanoTime
		got := truncateTimes(&at, firestoreTimePrecision).(*time.Time)
		require.Equal(t, microTime, *got)
		require.Equal(t, nanoTime, at, "input not mutated")
	})
	t.Run("unexported fields are kept but not truncated", func(t *testing.T) {
		got := truncateTimes(withHidden{At: nanoTime, hidden: nanoTime}, firestoreTimePrecision).(withHidden)
		require.Equal(t, microTime, got.At)
		require.Equal(t, nanoTime, got.hidden)
	})
	t.Run("a value shared twice is truncated once and stays shared", func(t *testing.T) {
		shared := &stampedInner{At: nanoTime}
		got := truncateTimes([]any{shared, shared, []any{}}, firestoreTimePrecision).([]any)
		require.Same(t, got[0], got[1])
		require.Equal(t, microTime, got[0].(*stampedInner).At)
		require.Equal(t, nanoTime, shared.At)
	})
	t.Run("only changed container entries are replaced", func(t *testing.T) {
		shared := &stampedInner{At: microTime}
		in := []any{shared, nanoTime}
		got := truncateTimes(in, firestoreTimePrecision).([]any)
		require.Same(t, shared, got[0])
		require.Equal(t, microTime, got[1])
		require.Equal(t, nanoTime, in[1])
	})
	t.Run("self-referencing value terminates", func(t *testing.T) {
		n := &selfRef{At: nanoTime}
		n.Next = n
		got := truncateTimes(n, firestoreTimePrecision).(*selfRef)
		require.Equal(t, microTime, got.At)
		require.Equal(t, nanoTime, n.At)
	})
}

// firestoreTimes is the option these tests store times through: the Firestore
// profile, which keeps microseconds. Without a profile that names a backend,
// times are kept exactly (TestTimePrecision_OnlyANamedBackendTruncates).
func firestoreTimes() Option { return Option(FirestoreProfile()) }

// dalgo is backend-neutral: a memory database truncates times only when its
// profile names a backend that does (Firestore: microseconds).
func TestTimePrecision_OnlyANamedBackendTruncates(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		db   dal.DB
		want time.Time
	}{
		{"no profile", dal.NewDB(newDatabase()), nanoTime},
		{"single writer", New(SingleWriterProfile()), nanoTime},
		{"Firestore profile", New(FirestoreProfile()), microTime},
		{"Firestore option", dal.NewDB(newDatabase(WithFirestoreProfile())), microTime},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := record.NewKeyWithID("stamped", "s1")
			require.NoError(t, tc.db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
				return tx.Set(ctx, record.NewRecordWithData(key, &stampedRecord{At: nanoTime}))
			}))
			got := &stampedRecord{}
			require.NoError(t, tc.db.Get(ctx, record.NewRecordWithData(key, got)))
			require.True(t, got.At.Equal(tc.want), "stored %v, want %v", got.At, tc.want)
		})
	}
}

// Every engine stores at its database's precision: exactly without a named
// backend, to the microsecond with Firestore; a query by the time as given
// finds it either way, also when the bound carries a monotonic clock reading
// (time.Now()), which no storage keeps.
func TestTimePrecision_EnginesFollowTheProfile(t *testing.T) {
	// time.Now() carries a monotonic reading (UTC, In and Local strip it,
	// so such a bound is always in the Local zone).
	monotonic := time.Now()
	require.False(t, monotonic == monotonic.Round(0), "time.Now() carries a monotonic reading") //nolint:staticcheck // == on purpose: Equal ignores the monotonic reading
	for _, profile := range []struct {
		name   string
		option []Option
		stored time.Time
	}{
		{"no profile", nil, nanoTime},
		{"Firestore", []Option{firestoreTimes()}, microTime},
	} {
		for _, engine := range []struct {
			name   string
			option CollectionOption
		}{
			{"serialized", WithSerializedStorage()},
			{"columnar", WithColumnarStorage()},
		} {
			t.Run(profile.name+"/"+engine.name, func(t *testing.T) {
				ctx := context.Background()
				options := append(append([]Option{}, profile.option...), WithSchema(false, WithCollection[timedThing]("timed", nil, engine.option)))
				db := newDatabase(options...)
				require.NoError(t, db.Insert(ctx, record.NewRecordWithData(record.NewKeyWithID("timed", "nano"), &timedThing{DtNext: nanoTime})))
				// The columnar engine matches equality only for UTC times (a
				// separate, pre-existing limitation), so the Local-zone
				// monotonic bound is checked on the Serialized engine.
				bounds := map[string]time.Time{"nano": nanoTime}
				if engine.name == "serialized" {
					require.NoError(t, db.Insert(ctx, record.NewRecordWithData(record.NewKeyWithID("timed", "now"), &timedThing{DtNext: monotonic})))
					bounds["now"] = monotonic
				}
				got := &timedThing{}
				require.NoError(t, db.Get(ctx, record.NewRecordWithData(record.NewKeyWithID("timed", "nano"), got)))
				require.True(t, got.DtNext.Equal(profile.stored), "stored %v, want %v", got.DtNext, profile.stored)
				for id, bound := range bounds {
					ids := queryTimedThings(t, db, func(q *dal.QueryBuilder) dal.IQueryBuilder { return q.WhereField("DtNext", dal.Equal, bound) })
					require.Equal(t, []string{id}, ids, "query by %v", bound)
				}
				if profile.option == nil {
					// Exact: the microsecond value is another time.
					ids := queryTimedThings(t, db, func(q *dal.QueryBuilder) dal.IQueryBuilder { return q.WhereField("DtNext", dal.Equal, microTime) })
					require.Empty(t, ids)
				}
			})
		}
	}
}

// A branch keeps its source's precision, for collections it copied and for
// new ones.
func TestTimePrecision_BranchKeepsThePrecision(t *testing.T) {
	ctx := context.Background()
	source := New(FirestoreProfile())
	require.NoError(t, source.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("timed", "before"), &timedThing{DtNext: nanoTime}))
	}))
	checkpoint, err := NewBranchingProvider().Capture(ctx, source)
	require.NoError(t, err)
	branch, err := checkpoint.Branch(ctx)
	require.NoError(t, err)
	defer func() { _ = branch.Close(ctx) }()
	db := dal.BackendOf(branch.DB()).(*database)
	require.NoError(t, db.Insert(ctx, record.NewRecordWithData(record.NewKeyWithID("timed", "after"), &timedThing{DtNext: nanoTime})))
	for _, id := range []string{"before", "after"} {
		got := &timedThing{}
		require.NoError(t, db.Get(ctx, record.NewRecordWithData(record.NewKeyWithID("timed", id), got)))
		require.True(t, got.DtNext.Equal(microTime), "%s stored %v", id, got.DtNext)
	}
	require.Equal(t, []string{"after", "before"},
		queryTimedThings(t, db, func(q *dal.QueryBuilder) dal.IQueryBuilder { return q.WhereField("DtNext", dal.Equal, nanoTime) }))
}
