package dalgo2memory

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
)

// drainRecordset executes a query via the recordset reader, drives the reader
// to exhaustion (covering Next/Close), and returns the resulting recordset.
func drainRecordset(t *testing.T, db *database, ctx context.Context, q dal.Query, opts ...recordset.Option) recordset.Recordset {
	t.Helper()
	reader, err := db.ExecuteQueryToRecordsetReader(ctx, q, opts...)
	require.NoError(t, err)
	rs := reader.Recordset()
	cur, err := reader.Cursor()
	require.NoError(t, err)
	require.Equal(t, "", cur)
	for {
		row, gotRS, e := reader.Next()
		if errors.Is(e, dal.ErrNoMoreRecords) {
			break
		}
		require.NoError(t, e)
		require.NotNil(t, row)
		require.Same(t, rs, gotRS)
	}
	require.NoError(t, reader.Close())
	return rs
}

func cellByName(t *testing.T, rs recordset.Recordset, rowIdx int, name string) any {
	t.Helper()
	row := rs.GetRow(rowIdx)
	require.NotNil(t, row)
	v, err := row.GetValueByName(name, rs)
	require.NoError(t, err)
	return v
}

// Column projection through the recordset reader: columns come from the
// explicit SELECT list (one aliased), in order.
func TestRecordset_ColumnProjection(t *testing.T) {
	db, ctx := seedPeople(t)
	q := dal.From(dal.NewRootCollectionRef("people", "")).NewQuery().
		SelectColumns(
			dal.Column{Alias: "n", Expression: dal.Field("name")},
			dal.Column{Expression: dal.Field("status")},
		)
	rs := drainRecordset(t, db, ctx, q)
	require.Equal(t, 2, rs.RowsCount())
	require.Equal(t, []string{"n", "status"}, columnNames(rs))
	require.Equal(t, "people", rs.Name())
}

// GROUP BY aggregation through the recordset reader.
func TestRecordset_GroupByAggregation(t *testing.T) {
	db, ctx := seedSales(t)
	q := dal.From(dal.NewRootCollectionRef("sales", "")).NewQuery().
		GroupBy(dal.Field("category")).
		SelectColumns(
			dal.Column{Expression: dal.Field("category")},
			dal.SumAs(dal.Field("amount"), "total"),
		)
	rs := drainRecordset(t, db, ctx, q)
	require.Equal(t, 2, rs.RowsCount())
	require.Equal(t, []string{"category", "total"}, columnNames(rs))
	require.Equal(t, reflect.TypeOf(""), rs.GetColumnByName("category").ValueType(), "category is a typed string column")
	require.Equal(t, reflect.TypeOf(float64(0)), rs.GetColumnByName("total").ValueType(), "SUM is a typed float64 column")
	byCat := map[string]any{}
	for i := 0; i < rs.RowsCount(); i++ {
		byCat[cellByName(t, rs, i, "category").(string)] = cellByName(t, rs, i, "total")
	}
	require.EqualValues(t, 30, byCat["A"])
	require.EqualValues(t, 5, byCat["B"])
}

// SELECT * (no explicit columns) derives columns from the sorted union of keys
// across rows; a *map[string]any record body is dereferenced.
func TestRecordset_SelectStar(t *testing.T) {
	db, ctx := seedPeople(t)
	q := dal.From(dal.NewRootCollectionRef("people", "")).NewQuery().
		SelectIntoRecord(func() record.Record {
			return record.NewRecordWithIncompleteKey("people", reflect.String, &map[string]any{})
		})
	rs := drainRecordset(t, db, ctx, q)
	require.Equal(t, 2, rs.RowsCount())
	require.Equal(t, []string{"id", "name", "status"}, columnNames(rs), "sorted union of keys")
}

// Keys-only query: each row exposes its key under an "ID" column.
func TestRecordset_KeysOnly(t *testing.T) {
	db, ctx := seedPeople(t)
	q := dal.From(dal.NewRootCollectionRef("people", "")).NewQuery().
		SelectKeysOnly(0)
	rs := drainRecordset(t, db, ctx, q)
	require.Equal(t, 2, rs.RowsCount())
	require.Equal(t, []string{"ID"}, columnNames(rs))
}

// The recordset name comes from the WithName option when provided.
func TestRecordset_NameOption(t *testing.T) {
	db, ctx := seedSales(t)
	q := dal.From(dal.NewRootCollectionRef("sales", "")).NewQuery().
		GroupBy(dal.Field("category")).
		SelectColumns(dal.Column{Expression: dal.Field("category")})
	rs := drainRecordset(t, db, ctx, q, recordset.WithName("custom"))
	require.Equal(t, "custom", rs.Name())
}

// A non-structured query is not supported by the recordset reader.
func TestRecordset_NonStructuredUnsupported(t *testing.T) {
	db := NewDB().(*database)
	_, err := db.ExecuteQueryToRecordsetReader(context.Background(), notStructuredQuery{})
	require.ErrorIs(t, err, dal.ErrNotSupported)
}

// An execution error from the underlying records pipeline surfaces.
func TestRecordset_ExecutionError(t *testing.T) {
	db, ctx := seedSales(t)
	q := dal.From(dal.NewRootCollectionRef("sales", "")).NewQuery().
		GroupBy(dal.Field("category")).
		SelectColumns(
			dal.Column{Expression: dal.Field("category")},
			dal.Column{Expression: dal.Field("amount")}, // not aggregated, not grouped
		)
	_, err := db.ExecuteQueryToRecordsetReader(ctx, q)
	require.ErrorContains(t, err, "neither an aggregate nor in GROUP BY")
}

// recordToMap covers every record-body shape.
func TestRecordToMap(t *testing.T) {
	key := record.NewKeyWithID("c", "k1")

	withData := func(data any) record.Record {
		return record.NewRecordWithData(key, data).SetError(nil)
	}

	t.Run("map", func(t *testing.T) {
		m, err := recordToMap(withData(map[string]any{"a": 1}))
		require.NoError(t, err)
		require.Equal(t, map[string]any{"a": 1}, m)
	})
	t.Run("pointer to map", func(t *testing.T) {
		m, err := recordToMap(withData(&map[string]any{"a": 2}))
		require.NoError(t, err)
		require.Equal(t, map[string]any{"a": 2}, m)
	})
	t.Run("nil pointer to map", func(t *testing.T) {
		m, err := recordToMap(withData((*map[string]any)(nil)))
		require.NoError(t, err)
		require.Empty(t, m)
	})
	t.Run("nil data exposes key as ID", func(t *testing.T) {
		m, err := recordToMap(record.NewRecord(key).SetError(nil))
		require.NoError(t, err)
		require.Equal(t, map[string]any{"ID": "k1"}, m)
	})
	t.Run("struct via json round-trip", func(t *testing.T) {
		m, err := recordToMap(withData(struct {
			Name string
			Age  int
		}{"alice", 30}))
		require.NoError(t, err)
		require.Equal(t, "alice", m["Name"])
		require.EqualValues(t, 30, m["Age"])
	})
	t.Run("nil pointer struct marshals to null", func(t *testing.T) {
		m, err := recordToMap(withData((*struct{ Name string })(nil)))
		require.NoError(t, err)
		require.Empty(t, m)
	})
	t.Run("non-object json errors", func(t *testing.T) {
		_, err := recordToMap(withData([]int{1, 2}))
		require.Error(t, err)
	})
	t.Run("unmarshalable value errors", func(t *testing.T) {
		_, err := recordToMap(withData(make(chan int)))
		require.Error(t, err)
	})
}

// inferColumn types a column from its values, falling back to any when the
// values are nullable, absent, mixed, or of an unsupported kind.
func TestInferColumn(t *testing.T) {
	rows := func(vals ...any) []map[string]any {
		out := make([]map[string]any, len(vals))
		for i, v := range vals {
			out[i] = map[string]any{"c": v}
		}
		return out
	}
	cases := []struct {
		name string
		rows []map[string]any
		want reflect.Type
	}{
		{"float64", rows(1.0, 2.0), reflect.TypeOf(float64(0))},
		{"string", rows("a", "b"), reflect.TypeOf("")},
		{"bool", rows(true, false), reflect.TypeOf(false)},
		{"int", rows(1, 2), reflect.TypeOf(int(0))},
		{"nested unsupported kind", rows([]any{1}), nil},
		{"mixed types", rows(1.0, "x"), nil},
		{"nil value present", rows(1.0, nil), nil},
		{"empty rows", []map[string]any{}, nil},
		{"absent key", []map[string]any{{"other": 1.0}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, inferColumn("c", tc.rows).ValueType())
		})
	}
}

// buildRecordsetReader surfaces a reader error from the records pipeline.
func TestBuildRecordsetReader_ReaderError(t *testing.T) {
	q := dal.From(dal.NewRootCollectionRef("people", "")).NewQuery().
		SelectColumns(dal.Column{Expression: dal.Field("name")})
	_, err := buildRecordsetReader(context.Background(), q, errRecordsReader{}, nil...)
	require.ErrorContains(t, err, "boom")
}

// buildRecordsetReader surfaces a record-to-map conversion error.
func TestBuildRecordsetReader_RecordConversionError(t *testing.T) {
	q := dal.From(dal.NewRootCollectionRef("people", "")).NewQuery().
		SelectColumns(dal.Column{Expression: dal.Field("name")})
	bad := record.NewRecordWithData(record.NewKeyWithID("people", "1"), make(chan int)).SetError(nil)
	reader := dal.NewRecordsReader([]record.Record{bad})
	_, err := buildRecordsetReader(context.Background(), q, reader)
	require.Error(t, err)
}

// errRecordsReader is a RecordsReader whose Next always fails.
type errRecordsReader struct{}

func (errRecordsReader) Next() (record.Record, error) { return nil, errors.New("boom") }
func (errRecordsReader) Cursor() (string, error)      { return "", nil }
func (errRecordsReader) Close() error                 { return nil }

// columnNames lists a recordset's column names in order.
func columnNames(rs recordset.Recordset) []string {
	cols := rs.Columns()
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.Name()
	}
	return names
}

// notStructuredQuery is a dal.Query that is not a dal.StructuredQuery.
type notStructuredQuery struct{}

func (notStructuredQuery) String() string { return "not structured" }
func (notStructuredQuery) Offset() int    { return 0 }
func (notStructuredQuery) Limit() int     { return 0 }
func (notStructuredQuery) GetRecordsReader(context.Context, dal.QueryExecutor) (dal.RecordsReader, error) {
	return nil, dal.ErrNotSupported
}
func (notStructuredQuery) GetRecordsetReader(context.Context, dal.QueryExecutor) (dal.RecordsetReader, error) {
	return nil, dal.ErrNotSupported
}
