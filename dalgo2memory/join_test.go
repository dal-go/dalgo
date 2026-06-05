package dalgo2memory

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/require"
)

type joinResult struct {
	ID     *int    `json:"id"`
	UserID *int    `json:"userId"`
	Status *string `json:"status"`
}

// seedUsersOrders loads two users and one order matching user 1.
func seedUsersOrders(t *testing.T) (*database, context.Context) {
	t.Helper()
	db := NewDB().(*database)
	ctx := context.Background()
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("users", "1"), &map[string]any{"id": 1, "status": "active"})))
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("users", "2"), &map[string]any{"id": 2, "status": "active"})))
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("orders", "a"), &map[string]any{"userId": 1, "status": "shipped"})))
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("orders", "b"), &map[string]any{"userId": 1, "status": "shipped"})))
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("orders", "c"), &map[string]any{"userId": 9, "status": "shipped"})))
	return db, ctx
}

func intoJoinResult() func() dal.Record {
	return func() dal.Record {
		return dal.NewRecordWithIncompleteKey("users", reflect.String, &joinResult{})
	}
}

func runJoinQuery(t *testing.T, db *database, ctx context.Context, q dal.Query) []joinResult {
	t.Helper()
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	var got []joinResult
	for {
		rec, err := reader.Next()
		if errors.Is(err, dal.ErrNoMoreRecords) {
			break
		}
		require.NoError(t, err)
		got = append(got, *rec.Data().(*joinResult))
	}
	return got
}

func onUserEqOrder() dal.Condition {
	return dal.NewComparison(dal.NewFieldRef("u", "id"), dal.Equal, dal.NewFieldRef("o", "userId"))
}

// Task 4: INNER returns only matched pairs.
func TestExecuteJoin_InnerMatchesOnly(t *testing.T) {
	db, ctx := seedUsersOrders(t)
	users := dal.NewRootCollectionRef("users", "u")
	orders := dal.NewRootCollectionRef("orders", "o")
	join := dal.NewJoinedSource(orders, dal.JoinInner, onUserEqOrder())
	q := dal.From(users).Join(join).NewQuery().SelectIntoRecord(intoJoinResult())

	got := runJoinQuery(t, db, ctx, q)

	require.Len(t, got, 2, "INNER must yield only the two id==1/userId==1 pairings")
	for _, r := range got {
		require.NotNil(t, r.ID)
		require.NotNil(t, r.UserID)
		require.Equal(t, 1, *r.ID)
		require.Equal(t, 1, *r.UserID)
	}
}

// Task 5: LEFT keeps the unmatched left row with absent right fields.
func TestExecuteJoin_LeftKeepsUnmatched(t *testing.T) {
	db, ctx := seedUsersOrders(t)
	users := dal.NewRootCollectionRef("users", "u")
	orders := dal.NewRootCollectionRef("orders", "o")
	join := dal.NewJoinedSource(orders, dal.JoinLeft, onUserEqOrder())
	q := dal.From(users).Join(join).NewQuery().SelectIntoRecord(intoJoinResult())

	got := runJoinQuery(t, db, ctx, q)

	require.Len(t, got, 3, "LEFT must yield the two matches plus the unmatched user 2")
	var unmatched int
	for _, r := range got {
		require.NotNil(t, r.ID)
		if *r.ID == 2 {
			unmatched++
			require.Nil(t, r.UserID, "unmatched LEFT row must have absent/nil right fields")
		}
	}
	require.Equal(t, 1, unmatched, "exactly one unmatched left row (user 2)")
}

// Task 4/5: a qualified WHERE predicate reads from its own source — the
// o-qualified status resolves to the order's value (not the user's field of
// the same name), and is absent for an unmatched LEFT row.
func TestExecuteJoin_QualifiedResolutionInWhere(t *testing.T) {
	db, ctx := seedUsersOrders(t)
	users := dal.NewRootCollectionRef("users", "u")
	orders := dal.NewRootCollectionRef("orders", "o")
	join := dal.NewJoinedSource(orders, dal.JoinLeft, onUserEqOrder())
	// WHERE o.status == "shipped": users also have a "status" field ("active").
	where := dal.NewComparison(dal.NewFieldRef("o", "status"), dal.Equal, dal.Constant{Value: "shipped"})
	q := dal.From(users).Join(join).NewQuery().Where(where).SelectIntoRecord(intoJoinResult())

	got := runJoinQuery(t, db, ctx, q)

	require.Len(t, got, 2, "only the two rows whose order status is 'shipped' match; unmatched user 2 (no order) is excluded")
	for _, r := range got {
		require.NotNil(t, r.ID)
		require.Equal(t, 1, *r.ID)
		require.NotNil(t, r.Status)
		require.Equal(t, "shipped", *r.Status, "o.status must resolve to the order's value, not the user's 'active'")
	}
}
