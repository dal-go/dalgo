package dalgo2memory

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
)

// seedUsersOrders loads two users and orders matching only user 1.
// users and orders both carry a "status" field to exercise qualified
// resolution under a name collision.
func seedUsersOrders(t *testing.T) (*database, context.Context) {
	t.Helper()
	db := newDatabase()
	ctx := context.Background()
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("users", "1"), &map[string]any{"id": 1, "status": "active"})))
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("users", "2"), &map[string]any{"id": 2, "status": "active"})))
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("orders", "a"), &map[string]any{"userId": 1, "status": "shipped"})))
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("orders", "b"), &map[string]any{"userId": 1, "status": "shipped"})))
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("orders", "c"), &map[string]any{"userId": 9, "status": "shipped"})))
	return db, ctx
}

func intoMapRecord() func() record.Record {
	return func() record.Record {
		return record.NewRecordWithIncompleteKey("users", reflect.String, &map[string]any{})
	}
}

func runJoinQuery(t *testing.T, db *database, ctx context.Context, q dal.Query) []map[string]any {
	t.Helper()
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	var got []map[string]any
	for {
		rec, err := reader.Next()
		if errors.Is(err, dal.ErrNoMoreRecords) {
			break
		}
		require.NoError(t, err)
		got = append(got, rec.Data().(map[string]any))
	}
	return got
}

func usersAlias() dal.RecordsetSource  { return dal.NewRootCollectionRef("users", "u") }
func ordersAlias() dal.RecordsetSource { return dal.NewRootCollectionRef("orders", "o") }

func onUserEqOrder() dal.Condition {
	return dal.NewComparison(dal.NewFieldRef("u", "id"), dal.Equal, dal.NewFieldRef("o", "userId"))
}

// Task 4: INNER returns only matched pairs.
func TestExecuteJoin_InnerMatchesOnly(t *testing.T) {
	db, ctx := seedUsersOrders(t)
	join := dal.NewJoinedSource(ordersAlias(), dal.JoinInner, onUserEqOrder())
	q := dal.From(usersAlias()).Join(join).NewQuery().SelectIntoRecord(intoMapRecord())

	got := runJoinQuery(t, db, ctx, q)

	require.Len(t, got, 2, "INNER must yield only the two id==1/userId==1 pairings")
	for _, r := range got {
		require.EqualValues(t, 1, r["id"])
		require.EqualValues(t, 1, r["userId"])
	}
}

// Task 5: LEFT keeps the unmatched left row with absent right fields.
func TestExecuteJoin_LeftKeepsUnmatched(t *testing.T) {
	db, ctx := seedUsersOrders(t)
	join := dal.NewJoinedSource(ordersAlias(), dal.JoinLeft, onUserEqOrder())
	q := dal.From(usersAlias()).Join(join).NewQuery().SelectIntoRecord(intoMapRecord())

	got := runJoinQuery(t, db, ctx, q)

	require.Len(t, got, 3, "LEFT must yield the two matches plus the unmatched user 2")
	var unmatched int
	for _, r := range got {
		if r["id"] == float64(2) {
			unmatched++
			_, present := r["userId"]
			require.False(t, present, "unmatched LEFT row must have absent right fields")
		}
	}
	require.Equal(t, 1, unmatched)
}

// Task 4/5: a qualified WHERE predicate reads from its own source — o.status
// resolves to the order's value (not the user's field of the same name) and is
// absent for an unmatched LEFT row.
func TestExecuteJoin_QualifiedResolutionInWhere(t *testing.T) {
	db, ctx := seedUsersOrders(t)
	join := dal.NewJoinedSource(ordersAlias(), dal.JoinLeft, onUserEqOrder())
	where := dal.NewComparison(dal.NewFieldRef("o", "status"), dal.Equal, dal.Constant{Value: "shipped"})
	q := dal.From(usersAlias()).Join(join).NewQuery().Where(where).SelectIntoRecord(intoMapRecord())

	got := runJoinQuery(t, db, ctx, q)

	require.Len(t, got, 2, "only rows whose order status is 'shipped'; unmatched user 2 is excluded")
	for _, r := range got {
		require.EqualValues(t, 1, r["id"])
		require.Equal(t, "shipped", r["status"], "o.status must resolve to the order's value, not the user's 'active'")
	}
}

// Task 6: unsupported join type and chained joins error, no rows.
func TestExecuteJoin_UnsupportedJoinErrors(t *testing.T) {
	db, ctx := seedUsersOrders(t)

	t.Run("reserved RIGHT type", func(t *testing.T) {
		right := dal.NewJoinedSource(ordersAlias(), dal.JoinRight, onUserEqOrder())
		q := dal.From(usersAlias()).Join(right).NewQuery().SelectIntoRecord(intoMapRecord())
		reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
		require.Nil(t, reader)
		require.ErrorContains(t, err, "unsupported join type")
	})

	t.Run("chained second join", func(t *testing.T) {
		j1 := dal.NewJoinedSource(ordersAlias(), dal.JoinInner, onUserEqOrder())
		j2 := dal.NewJoinedSource(ordersAlias(), dal.JoinInner, onUserEqOrder())
		q := dal.From(usersAlias()).Join(j1).Join(j2).NewQuery().SelectIntoRecord(intoMapRecord())
		reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
		require.Nil(t, reader)
		require.ErrorContains(t, err, "single join")
	})
}

// Task 6: a field qualified with a source naming no recordset errors, no rows —
// on either side of a WHERE comparison or inside an ON condition.
func TestExecuteJoin_UnresolvableSourceErrors(t *testing.T) {
	db, ctx := seedUsersOrders(t)

	t.Run("unknown source on WHERE left", func(t *testing.T) {
		join := dal.NewJoinedSource(ordersAlias(), dal.JoinLeft, onUserEqOrder())
		where := dal.NewComparison(dal.NewFieldRef("x", "foo"), dal.Equal, dal.Constant{Value: 1})
		q := dal.From(usersAlias()).Join(join).NewQuery().Where(where).SelectIntoRecord(intoMapRecord())
		reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
		require.Nil(t, reader)
		require.ErrorContains(t, err, "unknown source")
	})

	t.Run("unknown source on WHERE right", func(t *testing.T) {
		join := dal.NewJoinedSource(ordersAlias(), dal.JoinLeft, onUserEqOrder())
		where := dal.NewComparison(dal.NewFieldRef("o", "status"), dal.Equal, dal.NewFieldRef("zzz", "x"))
		q := dal.From(usersAlias()).Join(join).NewQuery().Where(where).SelectIntoRecord(intoMapRecord())
		reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
		require.Nil(t, reader)
		require.ErrorContains(t, err, "unknown source")
	})

	t.Run("unknown source in ON", func(t *testing.T) {
		on := dal.NewComparison(dal.NewFieldRef("u", "id"), dal.Equal, dal.NewFieldRef("x", "y"))
		join := dal.NewJoinedSource(ordersAlias(), dal.JoinInner, on)
		q := dal.From(usersAlias()).Join(join).NewQuery().SelectIntoRecord(intoMapRecord())
		reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
		require.Nil(t, reader)
		require.ErrorContains(t, err, "unknown source")
	})
}

type weirdExpr struct{}

func (weirdExpr) String() string { return "weird" }

// Edge cases for full branch coverage of the join executor.
func TestExecuteJoin_EdgeCases(t *testing.T) {
	t.Run("ordered WHERE operators are evaluated, not ignored", func(t *testing.T) {
		db, ctx := seedUsersOrders(t)
		join := dal.NewJoinedSource(ordersAlias(), dal.JoinInner, onUserEqOrder())
		run := func(op dal.Operator, value int) int {
			where := dal.NewComparison(dal.NewFieldRef("o", "userId"), op, dal.Constant{Value: value})
			q := dal.From(usersAlias()).Join(join).NewQuery().Where(where).SelectIntoRecord(intoMapRecord())
			return len(runJoinQuery(t, db, ctx, q))
		}
		require.Equal(t, 2, run(dal.GreaterThen, 0))
		require.Equal(t, 2, run(dal.GreaterOrEqual, 1))
		require.Equal(t, 0, run(dal.GreaterThen, 1))
		require.Equal(t, 2, run(dal.LessThen, 2))
		require.Equal(t, 0, run(dal.LessOrEqual, 0))
	})

	t.Run("an operator the join evaluator lacks fails the query", func(t *testing.T) {
		db, ctx := seedUsersOrders(t)
		join := dal.NewJoinedSource(ordersAlias(), dal.JoinInner, onUserEqOrder())
		for _, op := range []dal.Operator{dal.In, dal.NotIn, dal.Operator("!=")} {
			where := dal.NewComparison(dal.NewFieldRef("o", "userId"), op, dal.Constant{Value: 1})
			q := dal.From(usersAlias()).Join(join).NewQuery().Where(where).SelectIntoRecord(intoMapRecord())
			reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
			require.Nil(t, reader)
			require.ErrorContains(t, err, "unsupported operator")
		}
	})

	t.Run("a condition the join evaluator lacks fails the query", func(t *testing.T) {
		db, ctx := seedUsersOrders(t)
		join := dal.NewJoinedSource(ordersAlias(), dal.JoinInner, onUserEqOrder())
		where := dal.NewGroupCondition(dal.Or, notComparison{}, dal.NewComparison(dal.NewFieldRef("o", "userId"), dal.Equal, dal.Constant{Value: 1}))
		q := dal.From(usersAlias()).Join(join).NewQuery().Where(where).SelectIntoRecord(intoMapRecord())
		reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
		require.Nil(t, reader)
		require.ErrorContains(t, err, "unsupported condition")
	})

	t.Run("an ON predicate that is not a comparison fails the query", func(t *testing.T) {
		db, ctx := seedUsersOrders(t)
		join := dal.NewJoinedSource(ordersAlias(), dal.JoinInner, dal.NewGroupCondition(dal.And, onUserEqOrder()))
		q := dal.From(usersAlias()).Join(join).NewQuery().SelectIntoRecord(intoMapRecord())
		reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
		require.Nil(t, reader)
		require.ErrorContains(t, err, "ON predicate must be a comparison")
	})

	t.Run("unsupported expression in WHERE errors", func(t *testing.T) {
		db, ctx := seedUsersOrders(t)
		join := dal.NewJoinedSource(ordersAlias(), dal.JoinLeft, onUserEqOrder())
		where := dal.NewComparison(weirdExpr{}, dal.Equal, dal.Constant{Value: 1})
		q := dal.From(usersAlias()).Join(join).NewQuery().Where(where).SelectIntoRecord(intoMapRecord())
		reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
		require.Nil(t, reader)
		require.ErrorContains(t, err, "unsupported expression")
	})

	t.Run("limit truncates", func(t *testing.T) {
		db, ctx := seedUsersOrders(t)
		join := dal.NewJoinedSource(ordersAlias(), dal.JoinInner, onUserEqOrder())
		q := dal.From(usersAlias()).Join(join).NewQuery().Limit(1).SelectIntoRecord(intoMapRecord())
		require.Len(t, runJoinQuery(t, db, ctx, q), 1)
	})

	t.Run("keys-only join returns keys without data", func(t *testing.T) {
		db, ctx := seedUsersOrders(t)
		join := dal.NewJoinedSource(ordersAlias(), dal.JoinInner, onUserEqOrder())
		q := dal.From(usersAlias()).Join(join).NewQuery().SelectKeysOnly(reflect.String)
		reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
		require.NoError(t, err)
		var n int
		for {
			rec, err := reader.Next()
			if errors.Is(err, dal.ErrNoMoreRecords) {
				break
			}
			require.NoError(t, err)
			require.Equal(t, "users", rec.Key().Collection())
			n++
		}
		require.Equal(t, 2, n)
	})

	t.Run("source resolves by name when alias is empty", func(t *testing.T) {
		db, ctx := seedUsersOrders(t)
		ordersNoAlias := dal.NewRootCollectionRef("orders", "")
		on := dal.NewComparison(dal.NewFieldRef("u", "id"), dal.Equal, dal.NewFieldRef("orders", "userId"))
		join := dal.NewJoinedSource(ordersNoAlias, dal.JoinInner, on)
		q := dal.From(usersAlias()).Join(join).NewQuery().SelectIntoRecord(intoMapRecord())
		require.Len(t, runJoinQuery(t, db, ctx, q), 2)
	})

	t.Run("malformed base row errors", func(t *testing.T) {
		db := newDatabase()
		ctx := context.Background()
		db.collections["users"] = &serializedEngine{records: map[string][]byte{"1": []byte("{")}}
		require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("orders", "a"), &map[string]any{"userId": 1})))
		join := dal.NewJoinedSource(ordersAlias(), dal.JoinInner, onUserEqOrder())
		q := dal.From(usersAlias()).Join(join).NewQuery().SelectIntoRecord(intoMapRecord())
		reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
		require.Nil(t, reader)
		require.Error(t, err)
	})

	t.Run("malformed join row errors", func(t *testing.T) {
		db := newDatabase()
		ctx := context.Background()
		require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("users", "1"), &map[string]any{"id": 1})))
		db.collections["orders"] = &serializedEngine{records: map[string][]byte{"a": []byte("{")}}
		join := dal.NewJoinedSource(ordersAlias(), dal.JoinInner, onUserEqOrder())
		q := dal.From(usersAlias()).Join(join).NewQuery().SelectIntoRecord(intoMapRecord())
		reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
		require.Nil(t, reader)
		require.Error(t, err)
	})
}

// seedForOrdering loads three matched rows with distinct amounts/status; users
// carry a colliding "status" field ("zuser") to exercise qualified resolution.
func seedForOrdering(t *testing.T) (*database, context.Context) {
	t.Helper()
	db := newDatabase()
	ctx := context.Background()
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("users", "1"), &map[string]any{"id": 1, "status": "zuser"})))
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("users", "2"), &map[string]any{"id": 2, "status": "zuser"})))
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("orders", "oa"), &map[string]any{"userId": 1, "amount": 30, "status": "c"})))
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("orders", "ob"), &map[string]any{"userId": 1, "amount": 10, "status": "a"})))
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("orders", "oc"), &map[string]any{"userId": 2, "amount": 20, "status": "b"})))
	return db, ctx
}

func amountSeq(rows []map[string]any) []float64 {
	out := make([]float64, len(rows))
	for i, r := range rows {
		out[i] = r["amount"].(float64)
	}
	return out
}

func idSeq(rows []map[string]any) []float64 {
	out := make([]float64, len(rows))
	for i, r := range rows {
		out[i] = r["id"].(float64)
	}
	return out
}

func innerJoinQuery(order ...dal.OrderExpression) dal.Query {
	join := dal.NewJoinedSource(ordersAlias(), dal.JoinInner, onUserEqOrder())
	return dal.From(usersAlias()).Join(join).NewQuery().OrderBy(order...).SelectIntoRecord(intoMapRecord())
}

// Task 1: source-qualified, multi-key ordering with base-id tiebreak.
func TestExecuteJoin_OrderBy(t *testing.T) {
	t.Run("by qualified numeric key ascending", func(t *testing.T) {
		db, ctx := seedForOrdering(t)
		got := runJoinQuery(t, db, ctx, innerJoinQuery(dal.Ascending(dal.NewFieldRef("o", "amount"))))
		require.Equal(t, []float64{10, 20, 30}, amountSeq(got))
	})

	t.Run("by qualified colliding key uses the order's source", func(t *testing.T) {
		db, ctx := seedForOrdering(t)
		got := runJoinQuery(t, db, ctx, innerJoinQuery(dal.Ascending(dal.NewFieldRef("o", "status"))))
		// o.status a,b,c -> amounts 10,20,30; status values are the order's, not "zuser".
		require.Equal(t, []float64{10, 20, 30}, amountSeq(got))
		for _, r := range got {
			require.NotEqual(t, "zuser", r["status"])
		}
	})

	t.Run("multiple keys: u.id asc then o.amount desc", func(t *testing.T) {
		db, ctx := seedForOrdering(t)
		got := runJoinQuery(t, db, ctx, innerJoinQuery(
			dal.Ascending(dal.NewFieldRef("u", "id")),
			dal.Descending(dal.NewFieldRef("o", "amount")),
		))
		require.Equal(t, []float64{1, 1, 2}, idSeq(got))
		require.Equal(t, []float64{30, 10, 20}, amountSeq(got))
	})

	t.Run("no ORDER BY falls back to base-id order", func(t *testing.T) {
		db, ctx := seedForOrdering(t)
		got := runJoinQuery(t, db, ctx, innerJoinQuery())
		require.Equal(t, []float64{1, 1, 2}, idSeq(got))
	})

	t.Run("all keys equal falls back to base-id order", func(t *testing.T) {
		db, ctx := seedForOrdering(t)
		// u.status is "zuser" for every row -> all equal -> base-id tiebreak.
		got := runJoinQuery(t, db, ctx, innerJoinQuery(dal.Ascending(dal.NewFieldRef("u", "status"))))
		require.Equal(t, []float64{1, 1, 2}, idSeq(got))
	})
}

// Task 2: ORDER BY edge handling.
func TestExecuteJoin_OrderByEdges(t *testing.T) {
	t.Run("unknown ORDER BY source errors", func(t *testing.T) {
		db, ctx := seedForOrdering(t)
		q := innerJoinQuery(dal.Ascending(dal.NewFieldRef("z", "foo")))
		reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
		require.Nil(t, reader)
		require.ErrorContains(t, err, "unknown source")
	})

	t.Run("non-field ORDER BY key is skipped, rows returned in base-id order", func(t *testing.T) {
		db, ctx := seedForOrdering(t)
		got := runJoinQuery(t, db, ctx, innerJoinQuery(dal.Ascending(dal.Constant{Value: 1})))
		require.Equal(t, []float64{1, 1, 2}, idSeq(got))
	})
}

type joinTimedUser struct {
	ID int `json:"id"`
}

type joinTimedOrder struct {
	UserID int       `json:"userId"`
	At     time.Time `json:"at"`
}

func TestExecuteJoin_WhereTimeBounds(t *testing.T) {
	ctx := context.Background()
	earlier := nanoTime.Add(-time.Second)
	later := nanoTime.Add(time.Second)
	plusTwo := time.FixedZone("UTC+2", 2*3600)

	runOnDB := func(t *testing.T, db dal.QueryExecutor, withFirestore bool) {
		t.Helper()
		join := dal.NewJoinedSource(ordersAlias(), dal.JoinInner, onUserEqOrder())
		runWhere := func(where dal.Condition) []record.Record {
			q := dal.From(usersAlias()).Join(join).NewQuery().Where(where).SelectIntoRecord(intoMapRecord())
			reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
			require.NoError(t, err)
			recs, err := dal.ReadAllToRecords(ctx, reader)
			require.NoError(t, err)
			return recs
		}

		// Matching bounds against nanoTime (in UTC and in a non-UTC zone).
		for _, bound := range []time.Time{nanoTime, nanoTime.In(plusTwo)} {
			for _, op := range []dal.Operator{dal.Equal, dal.GreaterOrEqual, dal.LessOrEqual} {
				recs := runWhere(dal.NewComparison(dal.NewFieldRef("o", "at"), op, dal.NewConstant(bound)))
				require.Len(t, recs, 1, "o.at %s %v should match", op, bound)
			}
			// Constant on the left side as well.
			recs := runWhere(dal.NewComparison(dal.NewConstant(bound), dal.Equal, dal.NewFieldRef("o", "at")))
			require.Len(t, recs, 1, "%v == o.at should match", bound)
		}

		// Strict inequalities: earlier < stored < later.
		require.Len(t, runWhere(dal.NewComparison(dal.NewFieldRef("o", "at"), dal.GreaterThen, dal.NewConstant(earlier))), 1)
		require.Empty(t, runWhere(dal.NewComparison(dal.NewFieldRef("o", "at"), dal.GreaterThen, dal.NewConstant(later))))
		require.Len(t, runWhere(dal.NewComparison(dal.NewFieldRef("o", "at"), dal.LessThen, dal.NewConstant(later))), 1)
		require.Empty(t, runWhere(dal.NewComparison(dal.NewFieldRef("o", "at"), dal.LessThen, dal.NewConstant(earlier))))

		// Microsecond bound matches under FirestoreProfile (truncated), does not
		// match under exact (non-Firestore) profile.
		microRecs := runWhere(dal.NewComparison(dal.NewFieldRef("o", "at"), dal.Equal, dal.NewConstant(microTime)))
		if withFirestore {
			require.Len(t, microRecs, 1, "FirestoreProfile truncates stored time to microseconds")
		} else {
			require.Empty(t, microRecs, "non-Firestore profile keeps nanoseconds, so microTime != nanoTime")
		}
	}

	t.Run("direct backend with FirestoreProfile", func(t *testing.T) {
		db := newDatabase(firestoreTimes())
		require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("users", "1"), &joinTimedUser{ID: 1})))
		require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("orders", "a"), &joinTimedOrder{UserID: 1, At: nanoTime})))
		runOnDB(t, db, true)
	})

	t.Run("direct backend without FirestoreProfile", func(t *testing.T) {
		db := newDatabase()
		require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("users", "1"), &joinTimedUser{ID: 1})))
		require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("orders", "a"), &joinTimedOrder{UserID: 1, At: nanoTime})))
		runOnDB(t, db, false)
	})

	for _, tc := range []struct {
		name          string
		profile       Profile
		withFirestore bool
	}{
		{"dal.DB with FirestoreProfile", FirestoreProfile(), true},
		{"dal.DB with SingleWriterProfile", SingleWriterProfile(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := New(tc.profile, WithSchema(false,
				WithCollection[joinTimedUser]("users", nil),
				WithCollection[joinTimedOrder]("orders", nil),
			))
			require.NoError(t, db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
				if err := tx.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("users", "1"), &joinTimedUser{ID: 1})); err != nil {
					return err
				}
				return tx.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("orders", "a"), &joinTimedOrder{UserID: 1, At: nanoTime}))
			}))
			runOnDB(t, db, tc.withFirestore)
		})
	}

	t.Run("non-time stored values do not match time bound", func(t *testing.T) {
		db := newDatabase()
		require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("users", "1"), &map[string]any{"id": 1})))
		require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("orders", "a"), &map[string]any{"userId": 1, "at": 123})))
		require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("orders", "b"), &map[string]any{"userId": 1, "at": "not-a-time"})))
		join := dal.NewJoinedSource(ordersAlias(), dal.JoinInner, onUserEqOrder())
		where := dal.NewComparison(dal.NewFieldRef("o", "at"), dal.Equal, dal.NewConstant(nanoTime))
		q := dal.From(usersAlias()).Join(join).NewQuery().Where(where).SelectIntoRecord(intoMapRecord())
		require.Empty(t, runJoinQuery(t, db, ctx, q))
	})
}

func TestCanExecuteJoin(t *testing.T) {
	ctx := context.Background()
	db := newDatabase()
	inner := dal.NewJoinedSource(ordersAlias(), dal.JoinInner, onUserEqOrder())
	left := dal.NewJoinedSource(ordersAlias(), dal.JoinLeft, onUserEqOrder())

	// Accepted native join shapes.
	accepted := []dal.StructuredQuery{
		dal.From(usersAlias()).Join(inner).NewQuery().SelectIntoRecord(intoMapRecord()),
		dal.From(usersAlias()).Join(left).NewQuery().
			Where(dal.NewGroupCondition(dal.And,
				dal.NewIsNullCondition(dal.NewFieldRef("o", "status")),
				dal.NewGroupCondition(dal.Or,
					dal.NewComparison(dal.NewFieldRef("o", "userId"), dal.GreaterOrEqual, dal.NewConstant(1)),
				),
			)).
			OrderBy(dal.Ascending(dal.NewFieldRef("u", "id"))).
			SelectColumns(dal.Column{Expression: dal.NewFieldRef("u", "id")}),
	}
	for i, q := range accepted {
		require.NoError(t, db.CanExecuteJoin(ctx, q), "accepted[%d]", i)
	}

	// Declined shapes fall back to dal's generic join executor.
	parentKey := record.NewKeyWithID("orgs", "o1")
	parentRef := dal.NewCollectionRef("users", "u", parentKey)
	nestedJoin := dal.NewJoinedFrom(dal.From(ordersAlias()).Join(inner), dal.JoinInner, onUserEqOrder())
	declined := []struct {
		name string
		q    dal.StructuredQuery
	}{
		{"chained joins", dal.From(usersAlias()).Join(inner).Join(inner).NewQuery().SelectIntoRecord(intoMapRecord())},
		{"offset", dal.From(usersAlias()).Join(inner).NewQuery().Offset(1).SelectIntoRecord(intoMapRecord())},
		{"cursor", dal.From(usersAlias()).Join(inner).NewQuery().StartAfter("1").SelectIntoRecord(intoMapRecord())},
		{"group by", dal.From(usersAlias()).Join(inner).NewQuery().GroupBy(dal.NewFieldRef("u", "id")).SelectIntoRecord(intoMapRecord())},
		{"having", dal.From(usersAlias()).Join(inner).NewQuery().Having(dal.NewComparison(dal.NewFieldRef("u", "id"), dal.Equal, dal.NewConstant(1))).SelectIntoRecord(intoMapRecord())},
		{"nested join from", dal.From(usersAlias()).Join(nestedJoin).NewQuery().SelectIntoRecord(intoMapRecord())},
		{"right join", dal.From(usersAlias()).Join(dal.NewJoinedSource(ordersAlias(), dal.JoinRight, onUserEqOrder())).NewQuery().SelectIntoRecord(intoMapRecord())},
		{"parent-scoped collection", dal.From(parentRef).Join(inner).NewQuery().SelectIntoRecord(intoMapRecord())},
		{"non-comparison ON", dal.From(usersAlias()).Join(dal.NewJoinedSource(ordersAlias(), dal.JoinInner, dal.NewIsNullCondition(dal.NewFieldRef("o", "userId")))).NewQuery().SelectIntoRecord(intoMapRecord())},
		{"unsupported ON operator", dal.From(usersAlias()).Join(dal.NewJoinedSource(ordersAlias(), dal.JoinInner, dal.NewComparison(dal.NewFieldRef("u", "id"), dal.In, dal.NewConstant(1)))).NewQuery().SelectIntoRecord(intoMapRecord())},
		{"unsupported ON expression", dal.From(usersAlias()).Join(dal.NewJoinedSource(ordersAlias(), dal.JoinInner, dal.NewComparison(weirdExpr{}, dal.Equal, dal.NewConstant(1)))).NewQuery().SelectIntoRecord(intoMapRecord())},
		{"unsupported WHERE condition", dal.From(usersAlias()).Join(inner).NewQuery().Where(notComparison{}).SelectIntoRecord(intoMapRecord())},
		{"unsupported WHERE group op", dal.From(usersAlias()).Join(inner).NewQuery().Where(dal.NewGroupCondition(dal.Operator("XOR"), onUserEqOrder())).SelectIntoRecord(intoMapRecord())},
		{"unsupported WHERE child in group", dal.From(usersAlias()).Join(inner).NewQuery().Where(dal.NewGroupCondition(dal.And, notComparison{})).SelectIntoRecord(intoMapRecord())},
		{"wildcard column", dal.From(usersAlias()).Join(inner).NewQuery().SelectColumns(dal.Column{Expression: dal.NewFieldRef("u", "*")})},
		{"non-field column", dal.From(usersAlias()).Join(inner).NewQuery().SelectColumns(dal.Column{Expression: dal.NewConstant(1)})},
		{"non-field order by", dal.From(usersAlias()).Join(inner).NewQuery().OrderBy(dal.Ascending(dal.NewConstant(1))).SelectIntoRecord(intoMapRecord())},
	}
	for _, tc := range declined {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorIs(t, db.CanExecuteJoin(ctx, tc.q), dal.ErrNotSupported)
		})
	}
}
