package dalgo2memory

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
)

// --- fakes to reach the unsupported-type error branches ---

type fakeCond struct{}

func (fakeCond) String() string { return "FAKE_COND" }

type fakeExpr struct{}

func (fakeExpr) String() string { return "FAKE_EXPR" }

type fakeAgg struct{}

func (fakeAgg) String() string             { return "MEDIAN(amount)" }
func (fakeAgg) FuncName() string           { return "MEDIAN" }
func (fakeAgg) FuncArgs() []dal.Expression { return []dal.Expression{dal.Field("amount")} }

// MIN/MAX over a group, with the null amount skipped.
func TestGroupBy_MinMax(t *testing.T) {
	db, ctx := seedSales(t)
	q := salesQuery().
		GroupBy(dal.Field("category")).
		SelectColumns(
			dal.Column{Expression: dal.Field("category")},
			dal.MinAs(dal.Field("amount"), "lo"),
			dal.MaxAs(dal.Field("amount"), "hi"),
		)
	groups := byCategory(t, runProjection(t, db, ctx, q))
	require.EqualValues(t, 10, groups["A"]["lo"])
	require.EqualValues(t, 20, groups["A"]["hi"])
	require.EqualValues(t, 5, groups["B"]["lo"])
	require.EqualValues(t, 5, groups["B"]["hi"])
}

// COUNT(field) counts non-null values only (group A's null amount excluded).
func TestGroupBy_CountField(t *testing.T) {
	db, ctx := seedSales(t)
	q := salesQuery().
		GroupBy(dal.Field("category")).
		SelectColumns(
			dal.Column{Expression: dal.Field("category")},
			dal.CountAs(dal.Field("amount"), "nonNull"),
		)
	groups := byCategory(t, runProjection(t, db, ctx, q))
	require.EqualValues(t, 2, groups["A"]["nonNull"], "A has 2 non-null amounts of 3 rows")
	require.EqualValues(t, 1, groups["B"]["nonNull"])
}

// OFFSET skips leading groups; an offset beyond the group count yields none.
func TestGroupBy_Offset(t *testing.T) {
	db, ctx := seedSales(t)
	mk := func(offset int) []map[string]any {
		countAll := dal.Count()
		countAll.Alias = "n"
		q := salesQuery().
			GroupBy(dal.Field("category")).
			OrderBy(dal.AscendingField("category")).
			Offset(offset).
			SelectColumns(dal.Column{Expression: dal.Field("category")}, countAll)
		return runProjection(t, db, ctx, q)
	}
	after1 := mk(1)
	require.Len(t, after1, 1)
	require.Equal(t, "B", after1[0]["category"])
	require.Empty(t, mk(5), "offset past the last group yields no rows")
}

// An unaliased aggregate column is keyed by its expression text.
func TestGroupBy_UnaliasedAggregateKey(t *testing.T) {
	db, ctx := seedSales(t)
	q := salesQuery().
		GroupBy(dal.Field("category")).
		SelectColumns(dal.Column{Expression: dal.Field("category")}, dal.Count())
	rows := runProjection(t, db, ctx, q)
	require.Len(t, rows, 2)
	for _, r := range rows {
		require.Contains(t, r, "COUNT(*)")
	}
}

// Every comparison operator routed through HAVING, plus an unsupported operator
// that drops all groups.
func TestGroupBy_HavingOperators(t *testing.T) {
	cases := []struct {
		name string
		op   dal.Operator
		val  int
		want []string
	}{
		{"equal", dal.Equal, 30, []string{"A"}},
		{"greaterOrEqual", dal.GreaterOrEqual, 30, []string{"A"}},
		{"lessThen", dal.LessThen, 10, []string{"B"}},
		{"lessOrEqual", dal.LessOrEqual, 5, []string{"B"}},
		{"unsupportedOp", dal.In, 30, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, ctx := seedSales(t)
			q := salesQuery().
				GroupBy(dal.Field("category")).
				Having(dal.NewComparison(dal.Field("total"), tc.op, dal.NewConstant(tc.val))).
				SelectColumns(dal.Column{Expression: dal.Field("category")}, dal.SumAs(dal.Field("amount"), "total"))
			got := byCategory(t, runProjection(t, db, ctx, q))
			require.Len(t, got, len(tc.want))
			for _, cat := range tc.want {
				require.Contains(t, got, cat)
			}
		})
	}
}

// Multiple HAVING conditions AND-compose; A passes both, B fails one.
func TestGroupBy_HavingAnd(t *testing.T) {
	db, ctx := seedSales(t)
	q := salesQuery().
		GroupBy(dal.Field("category")).
		Having(dal.NewComparison(dal.Field("total"), dal.GreaterThen, dal.NewConstant(10))).
		Having(dal.NewComparison(dal.Count().Expression, dal.GreaterOrEqual, dal.NewConstant(2))).
		SelectColumns(dal.Column{Expression: dal.Field("category")}, dal.SumAs(dal.Field("amount"), "total"))
	rows := runProjection(t, db, ctx, q)
	require.Len(t, rows, 1)
	require.Equal(t, "A", rows[0]["category"])
}

// A HAVING GroupCondition with OR: A matches the second disjunct, B matches
// neither.
func TestGroupBy_HavingOr(t *testing.T) {
	db, ctx := seedSales(t)
	or := dal.NewGroupCondition(dal.Or,
		dal.NewComparison(dal.Field("total"), dal.GreaterThen, dal.NewConstant(100)),
		dal.NewComparison(dal.Field("total"), dal.GreaterThen, dal.NewConstant(10)),
	)
	q := salesQuery().
		GroupBy(dal.Field("category")).
		Having(or).
		SelectColumns(dal.Column{Expression: dal.Field("category")}, dal.SumAs(dal.Field("amount"), "total"))
	rows := runProjection(t, db, ctx, q)
	require.Len(t, rows, 1)
	require.Equal(t, "A", rows[0]["category"])
}

// HAVING comparing two constants resolves both operands as constants.
func TestGroupBy_HavingConstants(t *testing.T) {
	db, ctx := seedSales(t)
	q := salesQuery().
		GroupBy(dal.Field("category")).
		Having(dal.NewComparison(dal.NewConstant(5), dal.GreaterThen, dal.NewConstant(2))).
		SelectColumns(dal.Column{Expression: dal.Field("category")})
	require.Len(t, runProjection(t, db, ctx, q), 2, "5 > 2 is always true; both groups kept")
}

// --- error branches ---

func runGroupedExpectError(t *testing.T, q dal.Query, contains string) {
	t.Helper()
	db, ctx := seedSales(t)
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.Nil(t, reader)
	require.ErrorContains(t, err, contains)
}

func TestGroupBy_UnknownGroupBySource(t *testing.T) {
	q := salesQuery().
		GroupBy(dal.NewFieldRef("bad", "category")).
		SelectColumns(dal.Column{Expression: dal.NewFieldRef("bad", "category")})
	runGroupedExpectError(t, q, "GROUP BY field")
}

func TestGroupBy_GroupByUnsupportedExpr(t *testing.T) {
	q := salesQuery().
		GroupBy(dal.Count().Expression).
		SelectColumns(dal.Count())
	runGroupedExpectError(t, q, "unsupported expression")
}

func TestGroupBy_AggregateBadSourceInProjection(t *testing.T) {
	q := salesQuery().
		GroupBy(dal.Field("category")).
		SelectColumns(
			dal.Column{Expression: dal.Field("category")},
			dal.SumAs(dal.NewFieldRef("bad", "amount"), "total"),
		)
	runGroupedExpectError(t, q, "unknown source")
}

func TestGroupBy_CountBadSourceInProjection(t *testing.T) {
	q := salesQuery().
		GroupBy(dal.Field("category")).
		SelectColumns(
			dal.Column{Expression: dal.Field("category")},
			dal.CountAs(dal.NewFieldRef("bad", "amount"), "n"),
		)
	runGroupedExpectError(t, q, "unknown source")
}

func TestGroupBy_MinBadSourceInProjection(t *testing.T) {
	q := salesQuery().
		GroupBy(dal.Field("category")).
		SelectColumns(
			dal.Column{Expression: dal.Field("category")},
			dal.MinAs(dal.NewFieldRef("bad", "amount"), "lo"),
		)
	runGroupedExpectError(t, q, "unknown source")
}

// Two groups tie on the ORDER BY aggregate; the stable comparator keeps both.
func TestGroupBy_OrderByTie(t *testing.T) {
	db := NewDB().(*database)
	ctx := context.Background()
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("sales", "1"), &map[string]any{"category": "A"})))
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("sales", "2"), &map[string]any{"category": "B"})))
	countAll := dal.Count()
	countAll.Alias = "n"
	q := salesQuery().
		GroupBy(dal.Field("category")).
		OrderBy(dal.Descending(dal.Count().Expression)).
		SelectColumns(dal.Column{Expression: dal.Field("category")}, countAll)
	rows := runProjection(t, db, ctx, q)
	require.Len(t, rows, 2, "both groups have COUNT(*)=1; tie keeps both")
}

// A HAVING whose right operand errors propagates the error.
func TestGroupBy_HavingRightOperandError(t *testing.T) {
	q := salesQuery().
		GroupBy(dal.Field("category")).
		Having(dal.NewComparison(dal.Field("total"), dal.GreaterThen, dal.SumAs(dal.NewFieldRef("bad", "amount"), "").Expression)).
		SelectColumns(dal.Column{Expression: dal.Field("category")}, dal.SumAs(dal.Field("amount"), "total"))
	runGroupedExpectError(t, q, "unknown source")
}

func TestGroupBy_HavingBadSource(t *testing.T) {
	q := salesQuery().
		GroupBy(dal.Field("category")).
		Having(dal.NewComparison(dal.SumAs(dal.NewFieldRef("bad", "amount"), "").Expression, dal.GreaterThen, dal.NewConstant(1))).
		SelectColumns(dal.Column{Expression: dal.Field("category")})
	runGroupedExpectError(t, q, "unknown source")
}

func TestGroupBy_OrderByBadSource(t *testing.T) {
	q := salesQuery().
		GroupBy(dal.Field("category")).
		OrderBy(dal.Descending(dal.SumAs(dal.NewFieldRef("bad", "amount"), "").Expression)).
		SelectColumns(dal.Column{Expression: dal.Field("category")})
	runGroupedExpectError(t, q, "unknown source")
}

// SUM skips a non-numeric (non-null) value in the group.
func TestGroupBy_SumSkipsNonNumeric(t *testing.T) {
	db := NewDB().(*database)
	ctx := context.Background()
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("sales", "1"), &map[string]any{"category": "A", "amount": 10})))
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("sales", "2"), &map[string]any{"category": "A", "amount": "oops"})))
	q := salesQuery().
		GroupBy(dal.Field("category")).
		SelectColumns(dal.Column{Expression: dal.Field("category")}, dal.SumAs(dal.Field("amount"), "total"))
	rows := runProjection(t, db, ctx, q)
	require.Len(t, rows, 1)
	require.EqualValues(t, 10, rows[0]["total"], "non-numeric amount skipped")
}

// A HAVING OR whose first disjunct errors propagates the error.
func TestGroupBy_HavingOrError(t *testing.T) {
	or := dal.NewGroupCondition(dal.Or,
		dal.NewComparison(dal.SumAs(dal.NewFieldRef("bad", "amount"), "").Expression, dal.GreaterThen, dal.NewConstant(1)),
		dal.NewComparison(dal.Field("total"), dal.GreaterThen, dal.NewConstant(1)),
	)
	q := salesQuery().
		GroupBy(dal.Field("category")).
		Having(or).
		SelectColumns(dal.Column{Expression: dal.Field("category")}, dal.SumAs(dal.Field("amount"), "total"))
	runGroupedExpectError(t, q, "unknown source")
}

// A HAVING AND whose first conjunct errors propagates the error.
func TestGroupBy_HavingAndError(t *testing.T) {
	q := salesQuery().
		GroupBy(dal.Field("category")).
		Having(dal.NewComparison(dal.SumAs(dal.NewFieldRef("bad", "amount"), "").Expression, dal.GreaterThen, dal.NewConstant(1))).
		Having(dal.NewComparison(dal.Field("total"), dal.GreaterThen, dal.NewConstant(1))).
		SelectColumns(dal.Column{Expression: dal.Field("category")}, dal.SumAs(dal.Field("amount"), "total"))
	runGroupedExpectError(t, q, "unknown source")
}

func TestGroupBy_HavingUnsupportedExpr(t *testing.T) {
	q := salesQuery().
		GroupBy(dal.Field("category")).
		Having(dal.NewComparison(fakeExpr{}, dal.GreaterThen, dal.NewConstant(1))).
		SelectColumns(dal.Column{Expression: dal.Field("category")})
	runGroupedExpectError(t, q, "unsupported grouped expression")
}

func TestGroupBy_HavingUnsupportedCondition(t *testing.T) {
	q := salesQuery().
		GroupBy(dal.Field("category")).
		Having(fakeCond{}).
		SelectColumns(dal.Column{Expression: dal.Field("category")})
	runGroupedExpectError(t, q, "unsupported HAVING condition")
}

func TestGroupBy_UnsupportedAggregate(t *testing.T) {
	q := salesQuery().
		GroupBy(dal.Field("category")).
		SelectColumns(
			dal.Column{Expression: dal.Field("category")},
			dal.Column{Alias: "m", Expression: fakeAgg{}},
		)
	runGroupedExpectError(t, q, "unsupported aggregate")
}
