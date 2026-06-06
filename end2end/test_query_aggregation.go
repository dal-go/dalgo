package end2end

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/end2end/models"
	"github.com/dal-go/dalgo/recordset"
	"github.com/stretchr/testify/require"
)

// readMapRecords executes a structured query via the RecordsReader path and
// collects each record's data as a map (column projection and GROUP BY
// aggregation return map-shaped records). An adapter that does not implement
// such queries is expected to return dal.ErrNotSupported; the error is returned
// so callers can detect it via errors.Is and t.Skip — the standard end2end
// capability-reporting approach.
func readMapRecords(ctx context.Context, db dal.DB, q dal.Query, txMsg string) ([]map[string]any, error) {
	var out []map[string]any
	err := db.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
		records, err := dal.ExecuteQueryAndReadAllToRecords(ctx, q, tx)
		if err != nil {
			return err
		}
		out = make([]map[string]any, len(records))
		for i, rec := range records {
			m, err := recordDataAsMap(rec)
			if err != nil {
				return err
			}
			out[i] = m
		}
		return nil
	}, dal.TxWithMessage(txMsg))
	return out, err
}

// readRecordset executes the same structured query via the RecordsetReader
// path. The in-memory adapter returns dal.ErrNotSupported here; adapters that
// implement columnar reads return a populated recordset.
func readRecordset(ctx context.Context, db dal.DB, q dal.Query, txMsg string) (recordset.Recordset, error) {
	var rs recordset.Recordset
	err := db.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
		var e error
		rs, e = dal.ExecuteQueryAndReadAllToRecordset(ctx, q, tx)
		return e
	}, dal.TxWithMessage(txMsg))
	return rs, err
}

// recordDataAsMap returns a projected/aggregated record's data as a map,
// accepting either a map value or a pointer to one.
func recordDataAsMap(rec dal.Record) (map[string]any, error) {
	switch d := rec.Data().(type) {
	case map[string]any:
		return d, nil
	case *map[string]any:
		return *d, nil
	default:
		return nil, fmt.Errorf("expected map[string]any record data, got %T", rec.Data())
	}
}

// indexByString groups rows by the string value of a given key (e.g. the
// GROUP BY column), for order-independent assertions.
func indexByString(t *testing.T, rows []map[string]any, key string) map[string]map[string]any {
	t.Helper()
	out := make(map[string]map[string]any, len(rows))
	for _, r := range rows {
		k, ok := r[key].(string)
		require.Truef(t, ok, "row %v missing string key %q", r, key)
		out[k] = r
	}
	return out
}

// assertRecordsetRowCount runs a query through the RecordsetReader path and
// asserts the row count, skipping when the adapter reports the columnar read is
// unsupported. This exercises the second of dalgo's two read paths (records vs
// recordsets) for the same query.
func assertRecordsetRowCount(ctx context.Context, t *testing.T, db dal.DB, q dal.Query, txMsg string, wantRows int) {
	t.Helper()
	rs, err := readRecordset(ctx, db, q, txMsg)
	if errors.Is(err, dal.ErrNotSupported) {
		t.Skip("recordset reader not supported by adapter:", err)
		return
	}
	require.NoError(t, err)
	require.NotNil(t, rs)
	require.Equal(t, wantRows, rs.RowsCount())
}

// queryColumnProjectionTest exercises SELECT of a subset of columns (one
// aliased) over both read paths. Skips a path when the adapter reports it is
// unsupported.
func queryColumnProjectionTest(ctx context.Context, t *testing.T, db dal.DB) {
	const txMsg = "SELECT Name AS city, Country FROM Cities"
	newQuery := func() dal.Query {
		return dal.From(dal.NewRootCollectionRef(models.CitiesCollection, "")).NewQuery().
			SelectColumns(
				dal.Column{Alias: "city", Expression: dal.Field("Name")},
				dal.Column{Expression: dal.Field("Country")},
			)
	}

	t.Run("records reader", func(t *testing.T) {
		rows, err := readMapRecords(ctx, db, newQuery(), txMsg)
		if errors.Is(err, dal.ErrNotSupported) {
			t.Skip("column projection not supported by adapter:", err)
			return
		}
		require.NoError(t, err)
		require.Len(t, rows, len(models.Cities))
		for _, r := range rows {
			require.Len(t, r, 2, "exactly the two selected columns")
			require.Contains(t, r, "city", "aliased Name column")
			require.Contains(t, r, "Country")
			require.NotContains(t, r, "Name", "unaliased original field must not appear")
			require.NotContains(t, r, "Population", "unselected field must not appear")
		}
	})

	t.Run("recordset reader", func(t *testing.T) {
		assertRecordsetRowCount(ctx, t, db, newQuery(), txMsg, len(models.Cities))
	})
}

// queryGroupByTest exercises GROUP BY with COUNT(*)/SUM aggregates plus a
// HAVING filter, over both read paths. Skips a path when the adapter reports it
// is unsupported.
//
// The dataset has two cities each in IN (Delhi, Mumbai) and CN (Shanghai,
// Beijing) and one in each of JP, BR, EG, BD, PK, TR — eight distinct
// countries across ten cities.
func queryGroupByTest(ctx context.Context, t *testing.T, db dal.DB) {
	coll := dal.NewRootCollectionRef(models.CitiesCollection, "")

	t.Run("COUNT and SUM per Country", func(t *testing.T) {
		const txMsg = "SELECT Country, COUNT(*), SUM(Population) GROUP BY Country"
		newQuery := func() dal.Query {
			countAll := dal.Count()
			countAll.Alias = "cities"
			return dal.From(coll).NewQuery().
				GroupBy(dal.Field("Country")).
				SelectColumns(
					dal.Column{Expression: dal.Field("Country")},
					countAll,
					dal.SumAs(dal.Field("Population"), "population"),
				)
		}

		t.Run("records reader", func(t *testing.T) {
			rows, err := readMapRecords(ctx, db, newQuery(), txMsg)
			if errors.Is(err, dal.ErrNotSupported) {
				t.Skip("GROUP BY not supported by adapter:", err)
				return
			}
			require.NoError(t, err)
			require.Len(t, rows, 8, "eight distinct countries")
			byCountry := indexByString(t, rows, "Country")
			require.EqualValues(t, 2, byCountry["IN"]["cities"], "India has Delhi and Mumbai")
			require.EqualValues(t, 2, byCountry["CN"]["cities"], "China has Shanghai and Beijing")
			require.EqualValues(t, 1, byCountry["JP"]["cities"])
			// Delhi 30290936 + Mumbai 21344117.
			require.EqualValues(t, 51635053, byCountry["IN"]["population"])
		})

		t.Run("recordset reader", func(t *testing.T) {
			assertRecordsetRowCount(ctx, t, db, newQuery(), txMsg, 8)
		})
	})

	t.Run("HAVING COUNT(*) > 1", func(t *testing.T) {
		const txMsg = "SELECT Country, COUNT(*) GROUP BY Country HAVING COUNT(*) > 1"
		newQuery := func() dal.Query {
			countAll := dal.Count()
			countAll.Alias = "cities"
			return dal.From(coll).NewQuery().
				GroupBy(dal.Field("Country")).
				Having(dal.NewComparison(dal.Count().Expression, dal.GreaterThen, dal.NewConstant(1))).
				SelectColumns(
					dal.Column{Expression: dal.Field("Country")},
					countAll,
				)
		}

		t.Run("records reader", func(t *testing.T) {
			rows, err := readMapRecords(ctx, db, newQuery(), txMsg)
			if errors.Is(err, dal.ErrNotSupported) {
				t.Skip("GROUP BY/HAVING not supported by adapter:", err)
				return
			}
			require.NoError(t, err)
			require.Len(t, rows, 2, "only IN and CN have more than one city")
			byCountry := indexByString(t, rows, "Country")
			require.Contains(t, byCountry, "IN")
			require.Contains(t, byCountry, "CN")
			require.EqualValues(t, 2, byCountry["IN"]["cities"])
			require.EqualValues(t, 2, byCountry["CN"]["cities"])
		})

		t.Run("recordset reader", func(t *testing.T) {
			assertRecordsetRowCount(ctx, t, db, newQuery(), txMsg, 2)
		})
	})
}
