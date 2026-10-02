package dalgo2memory

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
)

func TestFieldValue(t *testing.T) {
	data := map[string]any{
		"name":          "a",
		"address":       map[string]any{"city": "Oslo", "geo": map[string]any{"lat": 59.9}},
		"flat.key":      "stored under a dotted key",
		"scalar":        5,
		"nil.child":     nil,
		"holder":        map[string]any{"child": nil},
		"not.an.object": "x",
	}
	for _, tt := range []struct {
		path    string
		want    any
		present bool
	}{
		{"name", "a", true},
		{"address.city", "Oslo", true},
		{"address.geo.lat", 59.9, true},
		{"flat.key", "stored under a dotted key", true},
		{"nil.child", nil, true},
		{"holder.child", nil, true},
		{"address.missing", nil, false},
		{"missing.city", nil, false},
		{"scalar.child", nil, false},
		{"missing", nil, false},
	} {
		got, ok := fieldValue(data, tt.path)
		require.Equal(t, tt.present, ok, tt.path)
		require.Equal(t, tt.want, got, tt.path)
	}
}

// A dotted field name is a path into nested maps in a single-source query, as
// it is in the generic executor, in WHERE, in projection and in ORDER BY.
func TestSingleSourceResolvesNestedFields(t *testing.T) {
	db := newDatabase()
	ctx := context.Background()
	for id, data := range map[string]map[string]any{
		"1": {"name": "a", "address": map[string]any{"city": "Oslo"}},
		"2": {"name": "b", "address": map[string]any{"city": "Bergen"}},
		"3": {"name": "c", "address": map[string]any{}},
		"4": {"name": "d"},
	} {
		data := data
		require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("people", id), &data)))
	}
	query := func(where dal.Condition) []map[string]any {
		q := dal.From(dal.NewRootCollectionRef("people", "")).NewQuery().Where(where).
			OrderBy(dal.Ascending(dal.Field("address.city"))).
			SelectColumns(dal.Column{Expression: dal.Field("name")}, dal.Column{Expression: dal.Field("address.city"), Alias: "city"})
		return runProjection(t, db, ctx, q)
	}
	got := query(dal.NewComparison(dal.Field("address.city"), dal.Equal, dal.Constant{Value: "Oslo"}))
	require.Equal(t, []map[string]any{{"name": "a", "city": "Oslo"}}, got)

	got = query(dal.NewComparison(dal.Field("address.city"), dal.GreaterOrEqual, dal.Constant{Value: "Bergen"}))
	require.Equal(t, []map[string]any{{"name": "b", "city": "Bergen"}, {"name": "a", "city": "Oslo"}}, got)

	got = query(dal.Field("address.city").IsNull())
	require.Equal(t, []map[string]any{{"name": "c", "city": nil}, {"name": "d", "city": nil}}, got)

	got = query(dal.Field("address.city").IsNotNull())
	require.Equal(t, []map[string]any{{"name": "b", "city": "Bergen"}, {"name": "a", "city": "Oslo"}}, got)
}

func TestJoinedQueryResolvesNestedFields(t *testing.T) {
	db := newDatabase()
	ctx := context.Background()
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("users", "1"), &map[string]any{"id": 1, "profile": map[string]any{"tier": "gold"}})))
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("users", "2"), &map[string]any{"id": 2, "profile": map[string]any{}})))
	require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("orders", "a"), &map[string]any{"userId": 1, "meta": map[string]any{"channel": "web"}})))
	join := dal.NewJoinedSource(ordersAlias(), dal.JoinLeft, onUserEqOrder())
	run := func(where dal.Condition) []map[string]any {
		q := dal.From(usersAlias()).Join(join).NewQuery().Where(where).
			OrderBy(dal.Ascending(dal.NewFieldRef("u", "profile.tier"))).
			SelectColumns(dal.Column{Expression: dal.NewFieldRef("u", "id"), Alias: "id"}, dal.Column{Expression: dal.NewFieldRef("o", "meta.channel"), Alias: "channel"})
		return runProjection(t, db, ctx, q)
	}
	require.Equal(t, []map[string]any{{"id": float64(1), "channel": "web"}}, run(dal.NewComparison(dal.NewFieldRef("u", "profile.tier"), dal.Equal, dal.Constant{Value: "gold"})))
	require.Equal(t, []map[string]any{{"id": float64(2), "channel": nil}}, run(dal.NewIsNullCondition(dal.NewFieldRef("u", "profile.tier"))))
	require.Equal(t, []map[string]any{{"id": float64(2), "channel": nil}}, run(dal.NewIsNullCondition(dal.NewFieldRef("o", "meta.channel"))))
}

// A nil pointer behind an interface is NULL, like an untyped nil. The value is
// handed to the evaluator directly, as an adapter-built row could carry it.
func TestTypedNilIsNullInTheEvaluators(t *testing.T) {
	var unset *string
	set := "x"
	sources := map[string]map[string]any{"": {"unset": unset, "set": &set}}
	known := map[string]bool{"": true}
	for _, tt := range []struct {
		name      string
		condition dal.Condition
		want      bool
	}{
		{"typed nil is null", dal.NewIsNullCondition(dal.Field("unset")), true},
		{"typed nil is not not-null", dal.NewIsNotNullCondition(dal.Field("unset")), false},
		{"pointer to a value is not null", dal.NewIsNullCondition(dal.Field("set")), false},
		{"pointer to a value is not-null", dal.NewIsNotNullCondition(dal.Field("set")), true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			single, err := matchesWhere(tt.condition, sources, known)
			require.NoError(t, err)
			require.Equal(t, tt.want, single)
			joined, err := matchesJoinWhere(tt.condition, sources, known)
			require.NoError(t, err)
			require.Equal(t, tt.want, joined)
		})
	}
	// An ordered comparison with a typed nil side is false, like with nil.
	ordered := dal.NewComparison(dal.Field("unset"), dal.GreaterThen, dal.Constant{Value: "a"})
	ok, err := matchesJoinWhere(ordered, sources, known)
	require.NoError(t, err)
	require.False(t, ok)
	// HAVING reads the group's value the same way.
	group := &aggGroup{out: map[string]any{"top": unset, "other": &set}, rows: []rowSources{{"": {}}}}
	for name, want := range map[string]bool{"top": true, "other": false} {
		got, err := matchesHaving(dal.NewIsNullCondition(dal.Field(name)), group, known)
		require.NoError(t, err)
		require.Equal(t, want, got, name)
	}
	// An ON predicate without a condition holds; one that is not a comparison fails.
	ok, err = matchesJoinCondition(nil, sources, known)
	require.NoError(t, err)
	require.True(t, ok)
}
