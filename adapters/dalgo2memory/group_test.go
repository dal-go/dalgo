package dalgo2memory

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/require"
)

// seedSales loads sales rows carrying category and amount to exercise GROUP BY
// aggregation. Category A has three rows (10, 20, null); category B has one (5).
func seedSales(t *testing.T) (*database, context.Context) {
	t.Helper()
	db := NewDB().(*database)
	ctx := context.Background()
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("sales", "1"), &map[string]any{"category": "A", "amount": 10})))
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("sales", "2"), &map[string]any{"category": "A", "amount": 20})))
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("sales", "3"), &map[string]any{"category": "A", "amount": nil})))
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("sales", "4"), &map[string]any{"category": "B", "amount": 5})))
	return db, ctx
}

func salesQuery() *dal.QueryBuilder {
	return dal.From(dal.NewRootCollectionRef("sales", "")).NewQuery()
}

// byCategory indexes grouped result rows by their "category" output key.
func byCategory(t *testing.T, rows []map[string]any) map[string]map[string]any {
	t.Helper()
	out := make(map[string]map[string]any, len(rows))
	for _, r := range rows {
		cat, ok := r["category"].(string)
		require.True(t, ok, "row missing string category: %v", r)
		out[cat] = r
	}
	return out
}

// AC single-source-grouping-with-aggregates: COUNT(*), SUM and AVG per group,
// with the null amount skipped so A's avg divides by 2.
func TestGroupBy_SingleSourceAggregates(t *testing.T) {
	db, ctx := seedSales(t)
	countAll := dal.Count()
	countAll.Alias = "n"
	q := salesQuery().
		GroupBy(dal.Field("category")).
		SelectColumns(
			dal.Column{Expression: dal.Field("category")},
			countAll,
			dal.SumAs(dal.Field("amount"), "total"),
			dal.AverageAs(dal.Field("amount"), "avg"),
		)
	rows := runProjection(t, db, ctx, q)
	require.Len(t, rows, 2)
	groups := byCategory(t, rows)

	require.EqualValues(t, 3, groups["A"]["n"])
	require.EqualValues(t, 30, groups["A"]["total"])
	require.EqualValues(t, 15, groups["A"]["avg"])

	require.EqualValues(t, 1, groups["B"]["n"])
	require.EqualValues(t, 5, groups["B"]["total"])
	require.EqualValues(t, 5, groups["B"]["avg"])
}

// AC all-null-group-aggregates-null: a group whose values are all null yields
// nil SUM/MAX while COUNT(*) still counts the rows.
func TestGroupBy_AllNullGroup(t *testing.T) {
	db := NewDB().(*database)
	ctx := context.Background()
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("sales", "1"), &map[string]any{"category": "A", "amount": nil})))
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("sales", "2"), &map[string]any{"category": "A", "amount": nil})))

	countAll := dal.Count()
	countAll.Alias = "n"
	q := salesQuery().
		GroupBy(dal.Field("category")).
		SelectColumns(
			dal.Column{Expression: dal.Field("category")},
			dal.SumAs(dal.Field("amount"), "total"),
			dal.MaxAs(dal.Field("amount"), "hi"),
			countAll,
		)
	rows := runProjection(t, db, ctx, q)
	require.Len(t, rows, 1)
	require.Nil(t, rows[0]["total"])
	require.Nil(t, rows[0]["hi"])
	require.EqualValues(t, 2, rows[0]["n"])
}

// AC non-grouped-select-column-errors: selecting a non-aggregate column absent
// from GROUP BY errors before producing rows.
func TestGroupBy_NonGroupedColumnErrors(t *testing.T) {
	db, ctx := seedSales(t)
	q := salesQuery().
		GroupBy(dal.Field("category")).
		SelectColumns(
			dal.Column{Expression: dal.Field("category")},
			dal.Column{Expression: dal.Field("amount")}, // not aggregated, not in GROUP BY
		)
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.Nil(t, reader)
	require.ErrorContains(t, err, "neither an aggregate nor in GROUP BY")
}

// AC having-filters-by-alias: HAVING total > 10 keeps only group A.
func TestGroupBy_HavingByAlias(t *testing.T) {
	db, ctx := seedSales(t)
	q := salesQuery().
		GroupBy(dal.Field("category")).
		Having(dal.NewComparison(dal.Field("total"), dal.GreaterThen, dal.NewConstant(10))).
		SelectColumns(
			dal.Column{Expression: dal.Field("category")},
			dal.SumAs(dal.Field("amount"), "total"),
		)
	rows := runProjection(t, db, ctx, q)
	require.Len(t, rows, 1)
	require.Equal(t, "A", rows[0]["category"])
	require.EqualValues(t, 30, rows[0]["total"])
}

// AC having-filters-by-aggregate-expression: HAVING SUM(amount) > 10 yields the
// identical result to the alias form.
func TestGroupBy_HavingByAggregateExpression(t *testing.T) {
	db, ctx := seedSales(t)
	sum := dal.SumAs(dal.Field("amount"), "total")
	q := salesQuery().
		GroupBy(dal.Field("category")).
		Having(dal.NewComparison(sum.Expression, dal.GreaterThen, dal.NewConstant(10))).
		SelectColumns(
			dal.Column{Expression: dal.Field("category")},
			sum,
		)
	rows := runProjection(t, db, ctx, q)
	require.Len(t, rows, 1)
	require.Equal(t, "A", rows[0]["category"])
}

// AC having-on-unselected-aggregate: HAVING references SUM(amount) which is not
// selected; group A survives and the output carries only category and n.
func TestGroupBy_HavingOnUnselectedAggregate(t *testing.T) {
	db, ctx := seedSales(t)
	countAll := dal.Count()
	countAll.Alias = "n"
	q := salesQuery().
		GroupBy(dal.Field("category")).
		Having(dal.NewComparison(dal.SumAs(dal.Field("amount"), "").Expression, dal.GreaterThen, dal.NewConstant(10))).
		SelectColumns(
			dal.Column{Expression: dal.Field("category")},
			countAll,
		)
	rows := runProjection(t, db, ctx, q)
	require.Len(t, rows, 1)
	require.Equal(t, "A", rows[0]["category"])
	require.EqualValues(t, 3, rows[0]["n"])
	require.NotContains(t, rows[0], "total")
	require.Len(t, rows[0], 2, "output must contain only category and n")
}

// AC grouped-order-and-limit: order by COUNT(*) descending with LIMIT 2 returns
// the two highest-count groups in order.
func TestGroupBy_OrderAndLimit(t *testing.T) {
	db := NewDB().(*database)
	ctx := context.Background()
	// 5 rows in A, 3 in B, 1 in C.
	add := func(id, cat string) {
		require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("sales", id), &map[string]any{"category": cat})))
	}
	add("a1", "A")
	add("a2", "A")
	add("a3", "A")
	add("a4", "A")
	add("a5", "A")
	add("b1", "B")
	add("b2", "B")
	add("b3", "B")
	add("c1", "C")

	countAll := dal.Count()
	countAll.Alias = "n"
	q := salesQuery().
		GroupBy(dal.Field("category")).
		OrderBy(dal.Descending(dal.Count().Expression)).
		Limit(2).
		SelectColumns(
			dal.Column{Expression: dal.Field("category")},
			countAll,
		)
	rows := runProjection(t, db, ctx, q)
	require.Len(t, rows, 2)
	require.Equal(t, "A", rows[0]["category"])
	require.EqualValues(t, 5, rows[0]["n"])
	require.Equal(t, "B", rows[1]["category"])
	require.EqualValues(t, 3, rows[1]["n"])
}

// AC empty-groupby-unchanged: a query with no GroupBy is unaffected — the
// projection path still returns one record per row.
func TestGroupBy_EmptyUnchanged(t *testing.T) {
	db, ctx := seedSales(t)
	q := salesQuery().SelectColumns(dal.Column{Expression: dal.Field("category")})
	rows := runProjection(t, db, ctx, q)
	require.Len(t, rows, 4, "no GROUP BY: one record per source row, not aggregated")
}

// AC join-grouping-qualified: GROUP BY u.country over a join, COUNT(*) of joined
// rows per country.
func TestGroupBy_JoinQualified(t *testing.T) {
	db := NewDB().(*database)
	ctx := context.Background()
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("users", "1"), &map[string]any{"id": 1, "country": "US"})))
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("users", "2"), &map[string]any{"id": 2, "country": "US"})))
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("users", "3"), &map[string]any{"id": 3, "country": "DE"})))
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("orders", "a"), &map[string]any{"userId": 1})))
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("orders", "b"), &map[string]any{"userId": 2})))
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("orders", "c"), &map[string]any{"userId": 3})))
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("orders", "d"), &map[string]any{"userId": 1})))

	join := dal.NewJoinedSource(ordersAlias(), dal.JoinInner,
		dal.NewComparison(dal.NewFieldRef("u", "id"), dal.Equal, dal.NewFieldRef("o", "userId")))
	countAll := dal.Count()
	countAll.Alias = "orders"
	q := dal.From(usersAlias()).Join(join).NewQuery().
		GroupBy(dal.NewFieldRef("u", "country")).
		SelectColumns(
			dal.Column{Expression: dal.NewFieldRef("u", "country")},
			countAll,
		)
	rows := runJoinQuery(t, db, ctx, q)
	require.Len(t, rows, 2)
	groups := make(map[string]any, 2)
	for _, r := range rows {
		groups[r["country"].(string)] = r["orders"]
	}
	require.EqualValues(t, 3, groups["US"], "US: orders a,b,d joined")
	require.EqualValues(t, 1, groups["DE"], "DE: order c joined")
}
