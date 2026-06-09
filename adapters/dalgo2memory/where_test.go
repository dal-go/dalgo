package dalgo2memory

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/require"
)

func TestMatchesWhere(t *testing.T) {
	data := map[string]any{
		"Name":   "Alice",
		"Age":    42,
		"Tags":   []string{"a", "b"},
		"AnyTag": []any{"x", "y", float64(7)},
		"Bad":    []any{[]any{"nested"}},
	}
	fieldRef := dal.Field
	cmp := func(left dal.Expression, op dal.Operator, right dal.Expression) dal.Comparison {
		return dal.Comparison{Left: left, Operator: op, Right: right}
	}
	for _, tt := range []struct {
		name      string
		condition dal.Condition
		want      bool
	}{
		{"nil_condition", nil, true},
		{"unsupported_condition_type", notComparison{}, false},

		// FieldRef op Constant
		{"equal_match", cmp(fieldRef("Name"), dal.Equal, dal.Constant{Value: "Alice"}), true},
		{"equal_mismatch", cmp(fieldRef("Name"), dal.Equal, dal.Constant{Value: "Bob"}), false},
		{"greater_true", cmp(fieldRef("Age"), dal.GreaterThen, dal.Constant{Value: 41}), true},
		{"greater_false", cmp(fieldRef("Age"), dal.GreaterThen, dal.Constant{Value: 42}), false},
		{"greater_or_equal_true", cmp(fieldRef("Age"), dal.GreaterOrEqual, dal.Constant{Value: 42}), true},
		{"greater_or_equal_false", cmp(fieldRef("Age"), dal.GreaterOrEqual, dal.Constant{Value: 43}), false},
		{"less_true", cmp(fieldRef("Age"), dal.LessThen, dal.Constant{Value: 43}), true},
		{"less_false", cmp(fieldRef("Age"), dal.LessThen, dal.Constant{Value: 42}), false},
		{"less_or_equal_true", cmp(fieldRef("Age"), dal.LessOrEqual, dal.Constant{Value: 42}), true},
		{"less_or_equal_false", cmp(fieldRef("Age"), dal.LessOrEqual, dal.Constant{Value: 41}), false},
		{"ordering_missing_field", cmp(fieldRef("Missing"), dal.GreaterThen, dal.Constant{Value: 0}), false},
		{"unsupported_operator", cmp(fieldRef("Age"), dal.In, dal.Constant{Value: 42}), false},
		{"unsupported_right_operand", cmp(fieldRef("Age"), dal.Equal, fieldRef("Name")), false},

		// Constant In FieldRef → array-contains
		{"array_contains_typed_slice", cmp(dal.Constant{Value: "a"}, dal.In, fieldRef("Tags")), true},
		{"array_contains_any_slice", cmp(dal.Constant{Value: "x"}, dal.In, fieldRef("AnyTag")), true},
		{"array_contains_numeric_coercion", cmp(dal.Constant{Value: 7}, dal.In, fieldRef("AnyTag")), true},
		{"array_contains_no_match", cmp(dal.Constant{Value: "z"}, dal.In, fieldRef("Tags")), false},
		{"array_contains_missing_field", cmp(dal.Constant{Value: "a"}, dal.In, fieldRef("Missing")), false},
		{"array_contains_non_slice_field", cmp(dal.Constant{Value: "Alice"}, dal.In, fieldRef("Name")), false},
		{"array_contains_uncomparable_element", cmp(dal.Constant{Value: "nested"}, dal.In, fieldRef("Bad")), false},
		{"array_contains_uncomparable_constant", cmp(dal.Constant{Value: []string{"a"}}, dal.In, fieldRef("Tags")), false},
		{"array_contains_number_vs_string", cmp(dal.Constant{Value: 1}, dal.In, fieldRef("Tags")), false},
		{"constant_left_wrong_operator", cmp(dal.Constant{Value: "a"}, dal.Equal, fieldRef("Tags")), false},
		{"constant_left_wrong_right_operand", cmp(dal.Constant{Value: "a"}, dal.In, dal.Constant{Value: "a"}), false},

		// FieldRef op dal.Array → array-contains-any
		{"array_contains_any_match", cmp(fieldRef("Tags"), dal.In, dal.Array{Value: []string{"z", "b"}}), true},
		{"array_contains_any_no_match", cmp(fieldRef("Tags"), dal.In, dal.Array{Value: []string{"y", "z"}}), false},
		{"array_contains_any_non_slice_field", cmp(fieldRef("Name"), dal.In, dal.Array{Value: []string{"Alice"}}), false},
		{"array_contains_any_non_slice_values", cmp(fieldRef("Tags"), dal.In, dal.Array{Value: "a"}), false},
		{"array_contains_any_nil_values", cmp(fieldRef("Tags"), dal.In, dal.Array{}), false},

		// unsupported left operand
		{"unsupported_left_operand", cmp(dal.Array{Value: []string{"a"}}, dal.In, fieldRef("Tags")), false},

		// group conditions
		{"group_and_all_match", dal.NewGroupCondition(dal.And,
			cmp(fieldRef("Name"), dal.Equal, dal.Constant{Value: "Alice"}),
			cmp(dal.Constant{Value: "b"}, dal.In, fieldRef("Tags")),
		), true},
		{"group_and_one_fails", dal.NewGroupCondition(dal.And,
			cmp(fieldRef("Name"), dal.Equal, dal.Constant{Value: "Alice"}),
			cmp(fieldRef("Age"), dal.Equal, dal.Constant{Value: 1}),
		), false},
		{"group_or_unsupported", dal.NewGroupCondition(dal.Or,
			cmp(fieldRef("Name"), dal.Equal, dal.Constant{Value: "Alice"}),
		), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, matchesWhere(data, tt.condition))
		})
	}
}

func TestElementEquals(t *testing.T) {
	for _, tt := range []struct {
		name string
		a, b any
		want bool
	}{
		{"equal_strings", "a", "a", true},
		{"different_strings", "a", "b", false},
		{"numbers_cross_type", float64(7), 7, true},
		{"numbers_unequal", float64(7), 8, false},
		{"number_vs_string", 7, "7", false},
		{"string_vs_number", "7", 7, false},
		{"uncomparable_element", []any{"x"}, "x", false},
		{"uncomparable_constant", "x", []any{"x"}, false},
		{"both_nil", nil, nil, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, elementEquals(tt.a, tt.b))
		})
	}
}

type taggedThing struct {
	Name string
	Rank int
	Tags []string
}

// queryTaggedThings runs a keys-only query over the "tagged" collection and
// returns the sorted matched IDs.
func queryTaggedThings(t *testing.T, db *database, where func(qb *dal.QueryBuilder) dal.IQueryBuilder) []string {
	t.Helper()
	q := where(dal.From(dal.NewRootCollectionRef("tagged", "")).NewQuery()).
		SelectKeysOnly(reflect.String)
	reader, err := db.ExecuteQueryToRecordsReader(context.Background(), q)
	require.NoError(t, err)
	ids := make([]string, 0)
	for _, record := range readAll(t, reader) {
		ids = append(ids, record.Key().ID.(string))
	}
	sort.Strings(ids)
	return ids
}

func insertTaggedThings(t *testing.T, db *database) {
	t.Helper()
	ctx := context.Background()
	for id, data := range map[string]*taggedThing{
		"t1": {Name: "first", Rank: 1, Tags: []string{"red", "blue"}},
		"t2": {Name: "second", Rank: 2, Tags: []string{"blue", "green"}},
		"t3": {Name: "third", Rank: 3, Tags: nil},
	} {
		key := dal.NewKeyWithID("tagged", id)
		require.NoError(t, db.Insert(ctx, dal.NewRecordWithData(key, data)))
	}
}

// TestQueryWhereArrayContainsEndToEnd exercises the firestore "array-contains"
// shape end to end: records persisted with a []string field (the serialized
// engine decodes it back as []any of string) matched via WhereArrayContains.
func TestQueryWhereArrayContainsEndToEnd(t *testing.T) {
	db := NewDB().(*database)
	insertTaggedThings(t, db)

	for _, tt := range []struct {
		value string
		want  []string
	}{
		{"blue", []string{"t1", "t2"}},
		{"green", []string{"t2"}},
		{"purple", []string{}},
	} {
		t.Run(tt.value, func(t *testing.T) {
			ids := queryTaggedThings(t, db, func(qb *dal.QueryBuilder) dal.IQueryBuilder {
				return qb.WhereArrayContains("Tags", tt.value)
			})
			require.Equal(t, tt.want, ids)
		})
	}
}

// TestQueryWhereArrayContainsAnyEndToEnd exercises the firestore
// "array-contains-any" shape end to end.
func TestQueryWhereArrayContainsAnyEndToEnd(t *testing.T) {
	db := NewDB().(*database)
	insertTaggedThings(t, db)

	for _, tt := range []struct {
		name   string
		values []string
		want   []string
	}{
		{"one_common", []string{"green", "purple"}, []string{"t2"}},
		{"matches_all_tagged", []string{"red", "green"}, []string{"t1", "t2"}},
		{"no_match", []string{"purple"}, []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ids := queryTaggedThings(t, db, func(qb *dal.QueryBuilder) dal.IQueryBuilder {
				return qb.WhereArrayContainsAny("Tags", tt.values)
			})
			require.Equal(t, tt.want, ids)
		})
	}
}

// TestQueryWhereOperatorsEndToEnd exercises ordering operators end to end.
func TestQueryWhereOperatorsEndToEnd(t *testing.T) {
	db := NewDB().(*database)
	insertTaggedThings(t, db)

	for _, tt := range []struct {
		name     string
		operator dal.Operator
		value    int
		want     []string
	}{
		{"greater", dal.GreaterThen, 1, []string{"t2", "t3"}},
		{"greater_or_equal", dal.GreaterOrEqual, 2, []string{"t2", "t3"}},
		{"less", dal.LessThen, 3, []string{"t1", "t2"}},
		{"less_or_equal", dal.LessOrEqual, 1, []string{"t1"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ids := queryTaggedThings(t, db, func(qb *dal.QueryBuilder) dal.IQueryBuilder {
				return qb.WhereField("Rank", tt.operator, tt.value)
			})
			require.Equal(t, tt.want, ids)
		})
	}
}

// TestQueryWhereMultipleConditionsEndToEnd verifies that multiple Where
// conditions (compiled into an AND GroupCondition) are all applied.
func TestQueryWhereMultipleConditionsEndToEnd(t *testing.T) {
	db := NewDB().(*database)
	insertTaggedThings(t, db)

	ids := queryTaggedThings(t, db, func(qb *dal.QueryBuilder) dal.IQueryBuilder {
		return qb.
			WhereArrayContains("Tags", "blue").
			WhereField("Rank", dal.GreaterThen, 1)
	})
	require.Equal(t, []string{"t2"}, ids)
}
