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

// seedPeople loads rows carrying id, name and status to exercise subset
// projection (selecting fewer columns than the row has).
func seedPeople(t *testing.T) (*database, context.Context) {
	t.Helper()
	db := NewDB().(*database)
	ctx := context.Background()
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("people", "1"), &map[string]any{"id": 1, "name": "alice", "status": "active"})))
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("people", "2"), &map[string]any{"id": 2, "name": "bob", "status": "active"})))
	return db, ctx
}

func runProjection(t *testing.T, db *database, ctx context.Context, q dal.Query) []map[string]any {
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

// AC single-source-projection: selecting id and name aliased n yields a data
// map of exactly {id, n}, with name and status absent.
func TestSingleSource_Projection(t *testing.T) {
	db, ctx := seedPeople(t)
	q := dal.From(dal.NewRootCollectionRef("people", "")).NewQuery().SelectColumns(
		dal.Column{Expression: dal.NewFieldRef("", "id")},
		dal.Column{Alias: "n", Expression: dal.NewFieldRef("", "name")},
	)
	got := runProjection(t, db, ctx, q)
	require.Len(t, got, 2)
	for _, r := range got {
		require.Len(t, r, 2)
		require.Contains(t, r, "id")
		require.Contains(t, r, "n")
		require.NotContains(t, r, "name")
		require.NotContains(t, r, "status")
	}
}

// AC join-projection-qualified: selecting u.id and o.status yields exactly
// {id, status} with status taking the order's value (not the user's).
func TestJoin_ProjectionQualified(t *testing.T) {
	db, ctx := seedUsersOrders(t)
	join := dal.NewJoinedSource(ordersAlias(), dal.JoinInner, onUserEqOrder())
	q := dal.From(usersAlias()).Join(join).NewQuery().SelectColumns(
		dal.Column{Expression: dal.NewFieldRef("u", "id")},
		dal.Column{Expression: dal.NewFieldRef("o", "status")},
	)
	got := runJoinQuery(t, db, ctx, q)
	require.Len(t, got, 2)
	for _, r := range got {
		require.Len(t, r, 2)
		require.Contains(t, r, "id")
		require.EqualValues(t, 1, r["id"])
		require.Equal(t, "shipped", r["status"], "o.status must take the order's value, not the user's 'active'")
	}
}

// AC non-field-column-errors: a selected column whose expression is not a
// FieldRef produces a descriptive error and no rows.
func TestSingleSource_NonFieldColumnErrors(t *testing.T) {
	db, ctx := seedPeople(t)
	q := dal.From(dal.NewRootCollectionRef("people", "")).NewQuery().SelectColumns(
		dal.Column{Expression: dal.Constant{Value: 1}},
	)
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.Nil(t, reader)
	require.ErrorContains(t, err, "not a field reference")
}

// AC unknown-column-source-errors: a selected column qualified with a source
// naming no recordset produces a descriptive error and no rows.
func TestJoin_UnknownColumnSourceErrors(t *testing.T) {
	db, ctx := seedUsersOrders(t)
	join := dal.NewJoinedSource(ordersAlias(), dal.JoinInner, onUserEqOrder())
	q := dal.From(usersAlias()).Join(join).NewQuery().SelectColumns(
		dal.Column{Expression: dal.NewFieldRef("x", "id")},
	)
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.Nil(t, reader)
	require.ErrorContains(t, err, "unknown source")
}

// AC empty-columns-unchanged: an empty Columns() leaves the full-record path
// untouched.
func TestSingleSource_EmptyColumnsUnchanged(t *testing.T) {
	db, ctx := seedPeople(t)
	q := dal.From(dal.NewRootCollectionRef("people", "")).NewQuery().SelectIntoRecord(func() record.Record {
		return record.NewRecordWithIncompleteKey("people", reflect.String, &map[string]any{})
	})
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	var n int
	for {
		rec, err := reader.Next()
		if errors.Is(err, dal.ErrNoMoreRecords) {
			break
		}
		require.NoError(t, err)
		r := *rec.Data().(*map[string]any)
		require.Contains(t, r, "id")
		require.Contains(t, r, "name")
		require.Contains(t, r, "status")
		n++
	}
	require.Equal(t, 2, n)
}
