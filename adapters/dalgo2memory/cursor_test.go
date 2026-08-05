package dalgo2memory

import (
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
)

func TestSingleSourceDocumentIDCursorsAreOrderedAndExclusive(t *testing.T) {
	db, ctx := seedThings(t)
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("things", "3"), &orderThing{Name: "c"})))
	query := func(descending bool, from, after dal.Cursor, offset, limit int) dal.Query {
		order := dal.Ascending(dal.DocumentID())
		if descending {
			order = dal.Descending(dal.DocumentID())
		}
		builder := dal.From(dal.NewRootCollectionRef("things", "")).NewQuery().OrderBy(order).Offset(offset).Limit(limit)
		if from != "" {
			builder = builder.StartFrom(from)
		}
		if after != "" {
			builder = builder.StartAfter(after)
		}
		return builder.SelectIntoRecord(func() record.Record {
			return record.NewRecordWithIncompleteKey("things", reflect.String, &orderThing{})
		})
	}
	require.Equal(t, []string{"a", "c"}, runSingleSource(t, db, ctx, query(false, "2", "", 0, 0)))
	require.Equal(t, []string{"c"}, runSingleSource(t, db, ctx, query(false, "", "2", 0, 0)))
	require.Equal(t, []string{"b"}, runSingleSource(t, db, ctx, query(true, "", "2", 0, 0)))
	require.Equal(t, []string{"a"}, runSingleSource(t, db, ctx, query(false, "", "", 1, 1)))
}

func TestSingleSourceCursorRejectsUntypedDocumentNameField(t *testing.T) {
	db, ctx := seedThings(t)
	q := dal.From(dal.NewRootCollectionRef("things", "")).NewQuery().
		OrderBy(dal.Ascending(dal.Field("__name__"))).StartAfter("1").
		SelectIntoRecord(func() record.Record { return record.NewRecordWithIncompleteKey("things", reflect.String, &orderThing{}) })
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.Nil(t, reader)
	require.ErrorIs(t, err, dal.ErrNotSupported)
}
