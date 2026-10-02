package dalgo2memory

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
)

func seedWherePeople(t *testing.T) (*database, context.Context) {
	t.Helper()
	db := newDatabase()
	ctx := context.Background()
	for id, data := range map[string]map[string]any{
		"1": {"name": "a", "role": "admin", "city": "Oslo"},
		"2": {"name": "b", "role": "user", "city": "Bergen"},
		"3": {"name": "c", "role": "guest", "city": "Oslo"},
	} {
		data := data
		require.NoError(t, db.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("people", id), &data)))
	}
	return db, ctx
}

func peopleNames(t *testing.T, db *database, ctx context.Context, where dal.Condition) []string {
	t.Helper()
	q := dal.From(dal.NewRootCollectionRef("people", "")).NewQuery().Where(where).
		OrderBy(dal.Ascending(dal.Field("name"))).SelectColumns(dal.Column{Expression: dal.Field("name")})
	var names []string
	for _, row := range runProjection(t, db, ctx, q) {
		names = append(names, row["name"].(string))
	}
	return names
}

func whereEq(field, value string) dal.Condition {
	return dal.NewComparison(dal.Field(field), dal.Equal, dal.Constant{Value: value})
}

// An OR group used to match nothing in a single-source query, silently; it is
// evaluated now.
func TestSingleSourceOrGroups(t *testing.T) {
	db, ctx := seedWherePeople(t)
	require.Equal(t, []string{"a", "b"}, peopleNames(t, db, ctx, dal.NewGroupCondition(dal.Or, whereEq("role", "admin"), whereEq("role", "user"))))
	require.Equal(t, []string{"a", "c"}, peopleNames(t, db, ctx, dal.NewGroupCondition(dal.And,
		whereEq("city", "Oslo"),
		dal.NewGroupCondition(dal.Or, whereEq("role", "admin"), whereEq("role", "guest")))))
	require.Equal(t, []string{"a", "b", "c"}, peopleNames(t, db, ctx, dal.NewGroupCondition(dal.Or,
		dal.NewGroupCondition(dal.And, whereEq("city", "Oslo"), whereEq("role", "admin")),
		whereEq("city", "Bergen"),
		whereEq("role", "guest"))))
	require.Empty(t, peopleNames(t, db, ctx, dal.NewGroupCondition(dal.Or, whereEq("role", "x"), whereEq("role", "y"))))
}

// A condition the adapter cannot evaluate fails the query instead of returning
// whatever remains.
func TestSingleSourceUnsupportedWhereFailsTheQuery(t *testing.T) {
	db, ctx := seedWherePeople(t)
	for name, where := range map[string]dal.Condition{
		"exists":                     dal.NewExistsCondition(dal.From(dal.NewRootCollectionRef("x", "")).NewQuery().SelectIntoRecordset()),
		"or with an unsupported":     dal.NewGroupCondition(dal.Or, whereEq("role", "nobody"), dal.NewNotExistsCondition(dal.From(dal.NewRootCollectionRef("x", "")).NewQuery().SelectIntoRecordset())),
		"null test on a param":       dal.NewIsNullCondition(dal.NewParam("p")),
		"null test on a values list": dal.NewIsNotNullCondition(dal.Array{Value: []string{"a"}}),
	} {
		t.Run(name, func(t *testing.T) {
			q := dal.From(dal.NewRootCollectionRef("people", "")).NewQuery().Where(where).SelectColumns(dal.Column{Expression: dal.Field("name")})
			reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
			require.Nil(t, reader)
			require.Error(t, err)
		})
	}
}
