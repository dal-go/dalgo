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

type orderThing struct {
	Name string `json:"Name"`
}

func seedThings(t *testing.T) (*database, context.Context) {
	t.Helper()
	db := NewDB().(*database)
	ctx := context.Background()
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("things", "1"), &orderThing{Name: "b"})))
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("things", "2"), &orderThing{Name: "a"})))
	return db, ctx
}

func thingsQuery(alias string, order ...dal.OrderExpression) dal.Query {
	return dal.From(dal.NewRootCollectionRef("things", alias)).NewQuery().OrderBy(order...).
		SelectIntoRecord(func() record.Record {
			return record.NewRecordWithIncompleteKey("things", reflect.String, &orderThing{})
		})
}

func runSingleSource(t *testing.T, db *database, ctx context.Context, q dal.Query) []string {
	t.Helper()
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	var names []string
	for {
		rec, err := reader.Next()
		if errors.Is(err, dal.ErrNoMoreRecords) {
			break
		}
		require.NoError(t, err)
		names = append(names, rec.Data().(*orderThing).Name)
	}
	return names
}

// Task 3: single-source ordering is unchanged through the shared comparator.
func TestSingleSource_OrderBy(t *testing.T) {
	t.Run("unqualified ascending then descending", func(t *testing.T) {
		db, ctx := seedThings(t)
		require.Equal(t, []string{"a", "b"}, runSingleSource(t, db, ctx, thingsQuery("", dal.AscendingField("Name"))))
		require.Equal(t, []string{"b", "a"}, runSingleSource(t, db, ctx, thingsQuery("", dal.DescendingField("Name"))))
	})

	t.Run("no ORDER BY returns rows in base-id order", func(t *testing.T) {
		db, ctx := seedThings(t)
		// ids "1" (Name b), "2" (Name a) -> base-id order is b, a.
		require.Equal(t, []string{"b", "a"}, runSingleSource(t, db, ctx, thingsQuery("")))
	})

	t.Run("alias-qualified ORDER BY resolves to the base", func(t *testing.T) {
		db, ctx := seedThings(t)
		got := runSingleSource(t, db, ctx, thingsQuery("t", dal.Ascending(dal.NewFieldRef("t", "Name"))))
		require.Equal(t, []string{"a", "b"}, got)
	})

	t.Run("unknown ORDER BY source errors", func(t *testing.T) {
		db, ctx := seedThings(t)
		reader, err := db.ExecuteQueryToRecordsReader(ctx, thingsQuery("", dal.Ascending(dal.NewFieldRef("x", "Name"))))
		require.Nil(t, reader)
		require.ErrorContains(t, err, "unknown source")
	})
}
