package dalgo2memory

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
)

type hashOnlyBackend struct{ dal.Backend }

func (hashOnlyBackend) QueryCapabilities() dal.QueryCapabilities { return dal.QueryCapabilities{} }

func TestDALgoGenericOrderedAggregation(t *testing.T) {
	backend, ctx := seedSales(t)
	db := dal.NewDB(backend)
	count := dal.Count()
	count.Alias = "orders"
	q := salesQuery().
		GroupBy(dal.Field("category")).
		Having(dal.NewComparison(dal.Field("orders"), dal.GreaterOrEqual, dal.NewConstant(1))).
		SelectColumns(
			dal.Column{Expression: dal.Field("category")},
			count,
			dal.CountDistinctAs(dal.Field("amount"), "distinct_amounts"),
			dal.SumDistinctAs(dal.Field("amount"), "sum_distinct"),
			dal.AverageDistinctAs(dal.Field("amount"), "avg_distinct"),
			dal.MinAs(dal.Field("amount"), "minimum"),
			dal.MaxAs(dal.Field("amount"), "maximum"),
		)
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	rows := readResultMaps(t, reader)
	require.Len(t, rows, 2)
	groups := byCategory(t, rows)
	require.EqualValues(t, 3, groups["A"]["orders"])
	require.EqualValues(t, 2, groups["A"]["distinct_amounts"])
	require.EqualValues(t, 30, groups["A"]["sum_distinct"])
	require.EqualValues(t, 15, groups["A"]["avg_distinct"])
	require.EqualValues(t, 10, groups["A"]["minimum"])
	require.EqualValues(t, 20, groups["A"]["maximum"])
}

func TestDALgoGenericUngroupedEmptyInput(t *testing.T) {
	db := dal.NewDB(newDatabase())
	count := dal.Count()
	count.Alias = "rows"
	q := dal.From(dal.NewRootCollectionRef("empty", "")).NewQuery().
		SelectColumns(count, dal.SumAs(dal.Field("amount"), "sum"))
	reader, err := db.ExecuteQueryToRecordsReader(context.Background(), q)
	require.NoError(t, err)
	rows := readResultMaps(t, reader)
	require.Equal(t, []map[string]any{{"rows": int64(0), "sum": nil}}, rows)
}

func readResultMaps(t *testing.T, reader dal.RecordsReader) []map[string]any {
	t.Helper()
	defer reader.Close()
	var rows []map[string]any
	for {
		rec, err := reader.Next()
		if err == dal.ErrNoMoreRecords {
			break
		}
		require.NoError(t, err)
		data, ok := rec.Data().(map[string]any)
		if !ok {
			pointer, pointerOK := rec.Data().(*map[string]any)
			require.True(t, pointerOK)
			data = *pointer
		}
		rows = append(rows, data)
	}
	return rows
}

func TestDALgoGenericFirstLastIncludeNull(t *testing.T) {
	backend := newDatabase()
	ctx := context.Background()
	require.NoError(t, backend.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("events", "1"), &map[string]any{"kind": "A", "status": nil})))
	require.NoError(t, backend.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("events", "2"), &map[string]any{"kind": "A", "status": "done"})))
	db := dal.NewDB(backend)
	q := dal.From(dal.NewRootCollectionRef("events", "")).NewQuery().
		GroupBy(dal.Field("kind")).
		SelectColumns(dal.FirstAs(dal.Field("status"), "first"), dal.LastAs(dal.Field("status"), "last"))
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	rows := readResultMaps(t, reader)
	require.Equal(t, []map[string]any{{"first": nil, "last": "done"}}, rows)
}

func TestDALgoGenericStreamingHashParityAndResultOrder(t *testing.T) {
	backend, ctx := seedSales(t)
	count := dal.Count()
	count.Alias = "orders"
	q := salesQuery().
		GroupBy(dal.Field("category")).
		OrderBy(dal.Descending(dal.Field("orders"))).
		Limit(1).
		SelectColumns(dal.Column{Expression: dal.Field("category")}, count)

	streamReader, err := dal.NewDB(backend).ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	streamed := readResultMaps(t, streamReader)
	hashReader, err := dal.NewDB(hashOnlyBackend{Backend: backend}).ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	hashed := readResultMaps(t, hashReader)
	require.Equal(t, streamed, hashed)
	require.Equal(t, "A", streamed[0]["category"])
	require.EqualValues(t, 3, streamed[0]["orders"])
}

func TestDALgoGenericAggregationRecordset(t *testing.T) {
	backend, ctx := seedSales(t)
	count := dal.Count()
	count.Alias = "orders"
	q := salesQuery().GroupBy(dal.Field("category")).SelectColumns(
		dal.Column{Expression: dal.Field("category")}, count,
	)
	reader, err := dal.NewDB(backend).ExecuteQueryToRecordsetReader(ctx, q)
	require.NoError(t, err)
	defer reader.Close()
	require.Equal(t, 2, reader.Recordset().RowsCount())
	row, rs, err := reader.Next()
	require.NoError(t, err)
	category, err := row.GetValueByName("category", rs)
	require.NoError(t, err)
	require.Equal(t, "A", category)
	orders, err := row.GetValueByName("orders", rs)
	require.NoError(t, err)
	require.EqualValues(t, 3, orders)
}

func TestDALgoGenericAggregationInsideTransactions(t *testing.T) {
	backend, ctx := seedSales(t)
	require.NoError(t, backend.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("sales", "5"), &map[string]any{"category": "A", "amount": 10})))
	db := dal.NewDB(backend)
	q := salesQuery().GroupBy(dal.Field("category")).SelectColumns(
		dal.Column{Expression: dal.Field("category")},
		dal.CountDistinctAs(dal.Field("amount"), "distinct_amounts"),
	)

	assertQuery := func(executor dal.QueryExecutor) {
		reader, err := executor.ExecuteQueryToRecordsReader(ctx, q)
		require.NoError(t, err)
		rows := byCategory(t, readResultMaps(t, reader))
		require.EqualValues(t, 2, rows["A"]["distinct_amounts"])
		require.EqualValues(t, 1, rows["B"]["distinct_amounts"])
	}

	require.NoError(t, db.RunReadonlyTransaction(ctx, func(_ context.Context, tx dal.ReadTransaction) error {
		assertQuery(tx)
		return nil
	}))
	require.NoError(t, db.RunReadwriteTransaction(ctx, func(_ context.Context, tx dal.ReadwriteTransaction) error {
		assertQuery(tx)
		return nil
	}))
}
