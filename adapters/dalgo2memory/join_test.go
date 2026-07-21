package dalgo2memory

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
)

// seedUsersOrders loads two users and orders matching only user 1.
// users and orders both carry a "status" field to exercise qualified
// resolution under a name collision.
func seedUsersOrders(t *testing.T) (*database, context.Context) {
	t.Helper()
	db := NewDB().(*database)
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
	t.Run("non-equality WHERE matches nothing", func(t *testing.T) {
		db, ctx := seedUsersOrders(t)
		join := dal.NewJoinedSource(ordersAlias(), dal.JoinInner, onUserEqOrder())
		where := dal.NewComparison(dal.NewFieldRef("o", "userId"), dal.GreaterThen, dal.Constant{Value: 0})
		q := dal.From(usersAlias()).Join(join).NewQuery().Where(where).SelectIntoRecord(intoMapRecord())
		require.Empty(t, runJoinQuery(t, db, ctx, q))
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
		db := NewDB().(*database)
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
		db := NewDB().(*database)
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
	db := NewDB().(*database)
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
