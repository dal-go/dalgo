package dalgo2memory

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/end2end"
	"github.com/dal-go/dalgo/update"
	"github.com/stretchr/testify/require"
)

func TestEndToEnd(t *testing.T) {
	end2end.TestDalgoDB(t, NewDB(), nil, false)
}

func TestDatabaseMetadata(t *testing.T) {
	db := NewDB().(*database)
	require.Equal(t, "dalgo2memory", db.ID())
	require.Equal(t, "memory", db.Adapter().Name())
	require.True(t, db.SupportsConcurrentConnections())
	require.Nil(t, db.Schema())
}

func TestUnsupportedRecordsetReader(t *testing.T) {
	reader, err := NewDB().ExecuteQueryToRecordsetReader(context.Background(), nil)
	require.Nil(t, reader)
	require.ErrorIs(t, err, dal.ErrNotSupported)

	err = NewDB().RunReadonlyTransaction(context.Background(), func(ctx context.Context, tx dal.ReadTransaction) error {
		reader, err := tx.ExecuteQueryToRecordsetReader(ctx, nil)
		require.Nil(t, reader)
		require.ErrorIs(t, err, dal.ErrNotSupported)
		return nil
	})
	require.NoError(t, err)
}

func TestTopLevelWriteMethods(t *testing.T) {
	ctx := context.Background()
	db := NewDB().(*database)
	key := dal.NewKeyWithID("Things", "one")
	record := dal.NewRecordWithData(key, &thing{Name: "first", Count: 1})

	require.NoError(t, db.Set(ctx, record))
	require.NoError(t, db.Update(ctx, key, []update.Update{update.ByFieldName("Name", "updated")}))

	var data thing
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &data)))
	require.Equal(t, "updated", data.Name)

	require.NoError(t, db.UpdateRecord(ctx, dal.NewRecordWithData(key, &thing{}), []update.Update{update.ByFieldName("Count", 2)}))
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &data)))
	require.Equal(t, 2, data.Count)

	require.NoError(t, db.Delete(ctx, key))
	exists, err := db.Exists(ctx, key)
	require.NoError(t, err)
	require.False(t, exists)
}

func TestTopLevelMultiMethods(t *testing.T) {
	ctx := context.Background()
	db := NewDB().(*database)
	keys := []*dal.Key{
		dal.NewKeyWithID("Things", "one"),
		dal.NewKeyWithID("Things", "two"),
	}
	records := []dal.Record{
		dal.NewRecordWithData(keys[0], &thing{Name: "first", Count: 1}),
		dal.NewRecordWithData(keys[1], &thing{Name: "second", Count: 2}),
	}

	require.NoError(t, db.SetMulti(ctx, records))
	require.NoError(t, db.UpdateMulti(ctx, keys, []update.Update{update.ByFieldName("Count", 3)}))

	data := []thing{{}, {}}
	require.NoError(t, db.GetMulti(ctx, []dal.Record{
		dal.NewRecordWithData(keys[0], &data[0]),
		dal.NewRecordWithData(keys[1], &data[1]),
	}))
	require.Equal(t, 3, data[0].Count)
	require.Equal(t, 3, data[1].Count)

	require.NoError(t, db.DeleteMulti(ctx, keys))
	for _, key := range keys {
		exists, err := db.Exists(ctx, key)
		require.NoError(t, err)
		require.False(t, exists)
	}
}

func TestTransactionMetadata(t *testing.T) {
	err := NewDB().RunReadwriteTransaction(context.Background(), func(_ context.Context, tx dal.ReadwriteTransaction) error {
		require.Empty(t, tx.ID())
		require.Nil(t, tx.Options())
		return nil
	})
	require.NoError(t, err)
}

func TestInsertDuplicateAndMissingUpdate(t *testing.T) {
	ctx := context.Background()
	db := NewDB().(*database)
	key := dal.NewKeyWithID("Things", "one")
	record := dal.NewRecordWithData(key, &thing{Name: "first"})

	require.NoError(t, db.Insert(ctx, record))
	err := db.Insert(ctx, record)
	require.Error(t, err)
	require.True(t, isDuplicate(err))

	err = db.Update(ctx, dal.NewKeyWithID("Things", "missing"), []update.Update{update.ByFieldName("Name", "x")})
	require.Error(t, err)
	require.True(t, dal.IsNotFound(err))
}

func TestMultiMethodsStopOnError(t *testing.T) {
	ctx := context.Background()
	db := NewDB().(*database)
	badRecord := dal.NewRecordWithData(dal.NewKeyWithID("Bad", "json"), func() {})

	require.Error(t, db.SetMulti(ctx, []dal.Record{badRecord}))
	require.Error(t, db.UpdateMulti(ctx, []*dal.Key{dal.NewKeyWithID("Missing", "one")}, []update.Update{update.ByFieldName("Name", "x")}))
}

func TestInsertMultiStopsOnError(t *testing.T) {
	ctx := context.Background()
	db := NewDB().(*database)
	key := dal.NewKeyWithID("Things", "one")
	records := []dal.Record{
		dal.NewRecordWithData(key, &thing{Name: "first"}),
		dal.NewRecordWithData(key, &thing{Name: "duplicate"}),
	}
	err := db.InsertMulti(ctx, records)
	require.Error(t, err)
}

func TestInsertMultiSuccess(t *testing.T) {
	ctx := context.Background()
	db := NewDB().(*database)
	require.NoError(t, db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.InsertMulti(ctx, []dal.Record{
			dal.NewRecordWithData(dal.NewKeyWithID("Things", "one"), &thing{Name: "first"}),
			dal.NewRecordWithData(dal.NewKeyWithID("Things", "two"), &thing{Name: "second"}),
		})
	}))
}

func TestBadRecordDataAndUnsupportedQuery(t *testing.T) {
	ctx := context.Background()
	db := NewDB().(*database)
	err := db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("Bad", "json"), func() {}))
	require.Error(t, err)

	reader, err := db.ExecuteQueryToRecordsReader(ctx, textQuery{})
	require.Nil(t, reader)
	require.ErrorIs(t, err, dal.ErrNotSupported)
}

func TestMalformedStoredData(t *testing.T) {
	ctx := context.Background()
	db := NewDB().(*database)
	key := dal.NewKeyWithID("Bad", "json")
	db.collections[key.Collection()] = &serializedEngine{records: map[string][]byte{keyID(key): []byte("{")}}

	require.Error(t, db.Get(ctx, dal.NewRecordWithData(key, &thing{})))
	require.Error(t, db.GetMulti(ctx, []dal.Record{dal.NewRecordWithData(key, &thing{})}))
	require.Error(t, db.Update(ctx, key, []update.Update{update.ByFieldName("Name", "x")}))

	q := dal.From(dal.NewRootCollectionRef("Bad", "")).NewQuery().SelectKeysOnly(reflect.String)
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.Nil(t, reader)
	require.Error(t, err)

	goodKey := dal.NewKeyWithID("Good", "json")
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(goodKey, &thing{Name: "ok"})))
	require.Error(t, db.Update(ctx, goodKey, []update.Update{update.ByFieldName("Bad", func() {})}))

	q = dal.From(dal.NewRootCollectionRef("Good", "")).NewQuery().SelectIntoRecord(func() dal.Record {
		return dal.NewRecordWithIncompleteKey("Good", reflect.String, &badTarget{})
	})
	reader, err = db.ExecuteQueryToRecordsReader(ctx, q)
	require.Nil(t, reader)
	require.Error(t, err)
}

func TestQueryEmptyCollection(t *testing.T) {
	q := dal.From(dal.NewRootCollectionRef("Missing", "")).NewQuery().SelectKeysOnly(reflect.String)
	reader, err := NewDB().ExecuteQueryToRecordsReader(context.Background(), q)
	require.NoError(t, err)
	record, err := reader.Next()
	require.Nil(t, record)
	require.ErrorIs(t, err, dal.ErrNoMoreRecords)
}

func TestQueryHelperBranches(t *testing.T) {
	require.False(t, matchesWhere(map[string]any{}, notComparison{}))
	require.False(t, matchesWhere(map[string]any{}, dal.Comparison{Operator: dal.In}))
	require.False(t, matchesWhere(map[string]any{}, dal.Comparison{
		Operator: dal.Equal,
		Left:     dal.Constant{Value: "not field"},
	}))
	require.False(t, matchesWhere(map[string]any{}, dal.Comparison{
		Operator: dal.Equal,
		Left:     dal.Field("Name"),
		Right:    dal.Field("Other"),
	}))

	require.Equal(t, -1, compare("a", "b"))
	require.Equal(t, 1, compare("b", "a"))
	require.Equal(t, 0, compare("a", "a"))
	require.Equal(t, 1, compare(uint(2), uint(1)))
	require.Equal(t, -1, compare(-1, 1))
	require.Equal(t, 0, compare(1.2, 1.2))
	require.Equal(t, 0, compare(nil, nil))

	_, ok := number(nil)
	require.False(t, ok)
	_, ok = number("not number")
	require.False(t, ok)
	v, ok := number(float32(1.5))
	require.True(t, ok)
	require.Equal(t, 1.5, v)
}

type thing struct {
	Name  string
	Count int
}

type badTarget struct {
	Name chan int
}

type notComparison struct{}

func (notComparison) String() string { return "" }

type textQuery struct{}

func (textQuery) String() string { return "" }
func (textQuery) Offset() int    { return 0 }
func (textQuery) Limit() int     { return 0 }
func (textQuery) GetRecordsReader(context.Context, dal.QueryExecutor) (dal.RecordsReader, error) {
	return nil, errors.New("not used")
}
func (textQuery) GetRecordsetReader(context.Context, dal.QueryExecutor) (dal.RecordsetReader, error) {
	return nil, errors.New("not used")
}
