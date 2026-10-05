package dalgo2memory

import (
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
)

// The adapter's own grouped reader computes count, sum, avg, min and max. It does
// not compute first or last, with an order or without: it refuses both, and an
// ordered first or last over this adapter is computed by DALgo's engine instead.
func TestGroupedReaderKeepsRefusingFirstAndLast(t *testing.T) {
	byAmount := []dal.OrderExpression{dal.AscendingField("amount")}
	for name, aggregate := range map[string]dal.AggregateFunc{
		"first":               dal.NewAggregate(dal.FIRST, false, dal.Field("amount")),
		"last":                dal.NewAggregate(dal.LAST, false, dal.Field("amount")),
		"first with an order": dal.NewOrderedAggregate(dal.FIRST, byAmount, dal.Field("amount")),
		"last with an order":  dal.NewOrderedAggregate(dal.LAST, byAmount, dal.Field("amount")),
	} {
		t.Run(name, func(t *testing.T) {
			q := salesQuery().
				GroupBy(dal.Field("category")).
				SelectColumns(dal.Column{Expression: dal.Field("category")}, dal.Column{Alias: "v", Expression: aggregate})
			runGroupedExpectError(t, q, "unsupported aggregate")
		})
	}
}

// DALgo's engine computes an ordered first and last over the adapter, and the answer
// does not depend on the order the adapter returns its rows in.
func TestOrderedFirstAndLastOverTheAdapterAreComputedByDALgo(t *testing.T) {
	ctx := t.Context()
	db := newDatabase()
	// The worked example: customer 1 has two invoices on 2024-01-02, and customer 2's
	// invoice 14 has no date.
	rows := []map[string]any{
		{"id": 10, "customer": 1, "at": "2024-01-01", "total": 100},
		{"id": 11, "customer": 1, "at": "2024-01-02", "total": nil},
		{"id": 13, "customer": 1, "at": "2024-01-02", "total": 50},
		{"id": 12, "customer": 2, "at": "2024-01-03", "total": 200},
		{"id": 14, "customer": 2, "at": nil, "total": 75},
	}
	for i := len(rows) - 1; i >= 0; i-- { // inserted in reverse
		row := rows[i]
		require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("invoices", row["id"]), &row)))
	}
	byDate := []dal.OrderExpression{dal.AscendingField("at")}
	q := dal.From(dal.NewRootCollectionRef("invoices", "")).NewQuery().
		GroupBy(dal.Field("customer")).
		OrderBy(dal.AscendingField("customer")).
		SelectColumns(
			dal.Column{Expression: dal.Field("customer")},
			dal.Column{Alias: "first_total", Expression: dal.NewOrderedAggregate(dal.FIRST, byDate, dal.Field("total"))},
			dal.Column{Alias: "last_total", Expression: dal.NewOrderedAggregate(dal.LAST, byDate, dal.Field("total"))})
	reader, err := dal.NewDB(db).ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	require.Equal(t, []map[string]any{
		{"customer": float64(1), "first_total": float64(100), "last_total": float64(50)},
		{"customer": float64(2), "first_total": float64(75), "last_total": float64(200)},
	}, readResultMaps(t, reader))
}
