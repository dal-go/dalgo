package dalgo2memory

import (
	"context"
	"testing"
	"time"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
)

func TestQueryWhereTimeFractionalPrefix(t *testing.T) {
	db := newDatabase()
	for id, stamp := range map[string]string{
		"whole": "2026-10-31T19:47:17Z",
		"short": "2026-10-31T19:47:17.7Z",
		"bound": "2026-10-31T19:47:17.700964Z",
		"later": "2026-10-31T19:47:17.71Z",
	} {
		at, err := time.Parse(time.RFC3339Nano, stamp)
		require.NoError(t, err)
		require.NoError(t, db.Insert(context.Background(), record.NewRecordWithData(record.NewKeyWithID("timed", id), &timedThing{DtNext: at})))
	}
	for _, stamp := range []string{"2026-10-31T19:47:17.700964Z", "2026-10-31T20:47:17.700964+01:00"} {
		bound, err := time.Parse(time.RFC3339Nano, stamp)
		require.NoError(t, err)
		for _, tc := range []struct {
			op   dal.Operator
			want []string
		}{
			{dal.LessThen, []string{"short", "whole"}},
			{dal.LessOrEqual, []string{"bound", "short", "whole"}},
			{dal.Equal, []string{"bound"}},
			{dal.GreaterThen, []string{"later"}},
			{dal.GreaterOrEqual, []string{"bound", "later"}},
		} {
			t.Run(stamp+string(tc.op), func(t *testing.T) {
				ids := queryTimedThings(t, db, func(q *dal.QueryBuilder) dal.IQueryBuilder { return q.WhereField("DtNext", tc.op, bound) })
				require.Equal(t, tc.want, ids)
			})
		}
	}
}

func TestTimeComparisonRejectsMalformedAndKeepsStringSemantics(t *testing.T) {
	bound, err := time.Parse(time.RFC3339Nano, "2026-10-31T19:47:17.700964Z")
	require.NoError(t, err)
	for _, op := range []dal.Operator{dal.Equal, dal.LessThen, dal.LessOrEqual, dal.GreaterThen, dal.GreaterOrEqual, dal.In} {
		for _, data := range []map[string]any{{}, {"At": nil}, {"At": 123}, {"At": "not-a-time"}} {
			require.False(t, matchesComparison(data, dal.NewComparison(dal.Field("At"), op, dal.NewConstant(bound))))
		}
	}
	data := map[string]any{"At": "2026-10-31T19:47:17.7Z"}
	require.False(t, matchesComparison(data, dal.NewComparison(dal.Field("At"), dal.In, dal.NewConstant(bound))))
	// This is intentionally lexical: an explicit string bound is not a timestamp.
	require.False(t, matchesComparison(data, dal.NewComparison(dal.Field("At"), dal.LessOrEqual, dal.NewConstant(bound.Format(time.RFC3339Nano)))))
	require.True(t, matchesComparison(data, dal.NewComparison(dal.Field("At"), dal.GreaterThen, dal.NewConstant(bound.Format(time.RFC3339Nano)))))
}
