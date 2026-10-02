package dalgo2memory

import (
	"context"
	"sort"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
)

func TestMatchesWhereIsNull(t *testing.T) {
	data := map[string]any{"Name": "Alice", "Company": nil}
	for _, tt := range []struct {
		name      string
		condition dal.Condition
		want      bool
	}{
		{"explicit null is null", dal.NewIsNullCondition(dal.Field("Company")), true},
		{"explicit null is not not-null", dal.NewIsNotNullCondition(dal.Field("Company")), false},
		{"missing is null", dal.NewIsNullCondition(dal.Field("Missing")), true},
		{"missing is not not-null", dal.NewIsNotNullCondition(dal.Field("Missing")), false},
		{"value is not null", dal.NewIsNullCondition(dal.Field("Name")), false},
		{"value is not-null", dal.NewIsNotNullCondition(dal.Field("Name")), true},
		{"unsupported operand", dal.NewIsNullCondition(dal.Constant{Value: nil}), false},
		{"in and group", dal.NewGroupCondition(dal.And, dal.NewIsNullCondition(dal.Field("Company")), dal.NewIsNotNullCondition(dal.Field("Name"))), true},
		{"in and group false", dal.NewGroupCondition(dal.And, dal.NewIsNullCondition(dal.Field("Company")), dal.NewIsNullCondition(dal.Field("Name"))), false},
		{"== nil matches null and missing, as before", dal.NewComparison(dal.Field("Missing"), dal.Equal, dal.Constant{Value: nil}), true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, matchesWhere(data, tt.condition))
		})
	}
}

func idsOf(rows []map[string]any, key string) []string {
	var ids []string
	for _, r := range rows {
		ids = append(ids, r[key].(string))
	}
	sort.Strings(ids)
	return ids
}

func TestSingleSourceQueryIsNull(t *testing.T) {
	db := newDatabase()
	ctx := context.Background()
	for id, data := range map[string]map[string]any{
		"1": {"name": "a", "company": nil},
		"2": {"name": "b", "company": "acme"},
		"3": {"name": "c"},
	} {
		data := data
		require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("chat", id), &data)))
	}
	run := func(cond dal.Condition) []string {
		q := dal.From(dal.NewRootCollectionRef("chat", "")).NewQuery().Where(cond).SelectColumns(dal.Column{Expression: dal.Field("name")})
		return idsOf(runProjection(t, db, ctx, q), "name")
	}
	require.Equal(t, []string{"a", "c"}, run(dal.NewIsNullCondition(dal.Field("company"))))
	require.Equal(t, []string{"b"}, run(dal.NewIsNotNullCondition(dal.Field("company"))))
}

func TestJoinWhereIsNull(t *testing.T) {
	db, ctx := seedUsersOrders(t)
	join := dal.NewJoinedSource(ordersAlias(), dal.JoinLeft, onUserEqOrder())
	run := func(where dal.Condition) []map[string]any {
		q := dal.From(usersAlias()).Join(join).NewQuery().Where(where).SelectIntoRecord(intoMapRecord())
		return runJoinQuery(t, db, ctx, q)
	}
	// Anti-join: users without an order (the null-extended LEFT side).
	got := run(dal.NewIsNullCondition(dal.NewFieldRef("o", "userId")))
	require.Len(t, got, 1)
	require.EqualValues(t, 2, got[0]["id"])
	require.Len(t, run(dal.NewIsNotNullCondition(dal.NewFieldRef("o", "userId"))), 2)
	// Groups: AND, OR (OR is new for joined WHERE and short-circuits on a match).
	require.Len(t, run(dal.NewGroupCondition(dal.And,
		dal.NewIsNotNullCondition(dal.NewFieldRef("o", "userId")),
		dal.NewComparison(dal.NewFieldRef("o", "status"), dal.Equal, dal.Constant{Value: "shipped"}))), 2)
	require.Len(t, run(dal.NewGroupCondition(dal.Or,
		dal.NewIsNullCondition(dal.NewFieldRef("o", "userId")),
		dal.NewComparison(dal.NewFieldRef("o", "status"), dal.Equal, dal.Constant{Value: "shipped"}))), 3)
	require.Empty(t, run(dal.NewGroupCondition(dal.Or,
		dal.NewIsNullCondition(dal.NewFieldRef("u", "id")),
		dal.NewComparison(dal.NewFieldRef("o", "status"), dal.Equal, dal.Constant{Value: "lost"}))))
	// An unknown qualifier fails loudly instead of matching nothing.
	q := dal.From(usersAlias()).Join(join).NewQuery().Where(dal.NewIsNullCondition(dal.NewFieldRef("zzz", "id"))).SelectIntoRecord(intoMapRecord())
	_, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.ErrorContains(t, err, "unknown source")
	q = dal.From(usersAlias()).Join(join).NewQuery().Where(dal.NewGroupCondition(dal.And, dal.NewIsNullCondition(dal.NewFieldRef("zzz", "id")))).SelectIntoRecord(intoMapRecord())
	_, err = db.ExecuteQueryToRecordsReader(ctx, q)
	require.ErrorContains(t, err, "unknown source")
}

func TestGroupByHavingIsNull(t *testing.T) {
	db, ctx := seedSales(t)
	run := func(having dal.Condition) map[string]map[string]any {
		q := salesQuery().GroupBy(dal.Field("category")).Having(having).
			SelectColumns(dal.Column{Expression: dal.Field("category")}, dal.MaxAs(dal.Field("amount"), "top"))
		return byCategory(t, runProjection(t, db, ctx, q))
	}
	require.Len(t, run(dal.NewIsNullCondition(dal.Field("top"))), 0, "no group's max is null: A has 10 and 20, B has 5")
	got := run(dal.NewIsNotNullCondition(dal.Field("top")))
	require.Len(t, got, 2)
	// Aggregate expression operand, and a group inside HAVING.
	require.Len(t, run(dal.NewGroupCondition(dal.Or,
		dal.NewIsNullCondition(dal.NewAggregate(dal.MIN, false, dal.Field("amount"))),
		dal.NewIsNotNullCondition(dal.Field("category")))), 2)
	// An operand HAVING cannot resolve is an error, not a silent miss.
	q := salesQuery().GroupBy(dal.Field("category")).Having(dal.NewIsNullCondition(fakeExpr{})).
		SelectColumns(dal.Column{Expression: dal.Field("category")}, dal.MaxAs(dal.Field("amount"), "top"))
	_, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.Error(t, err)
}
