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

func TestQueryWhereTimeEqualityNonUTCLocation(t *testing.T) {
	ctx := context.Background()
	utcInstant := time.Date(2026, 11, 4, 10, 11, 12, 123456000, time.UTC)
	fixedZone := time.FixedZone("UTC+2", 2*3600)
	zones := []struct {
		name string
		loc  *time.Location
	}{
		{"FixedZone+2", fixedZone},
		{"FixedZone-5", time.FixedZone("UTC-5", -5*3600)},
	}
	if loaded, err := time.LoadLocation("America/New_York"); err == nil {
		zones = append(zones, struct {
			name string
			loc  *time.Location
		}{"America/New_York", loaded})
	}

	for _, engine := range []struct {
		name   string
		option CollectionOption
	}{
		{"serialized", WithSerializedStorage()},
		{"columnar", WithColumnarStorage()},
		{"columnar with custom strategy", WithColumnarStorage(WithColumnStrategy("DtNext", newRecordingStrategy(true, func(int) bool { return false })))},
	} {
		t.Run(engine.name, func(t *testing.T) {
			db := newDatabase(WithSchema(false, WithCollection[timedThing]("timed", nil, engine.option)))
			require.NoError(t, db.Insert(ctx, record.NewRecordWithData(record.NewKeyWithID("timed", "r1"), &timedThing{DtNext: utcInstant})))

			for _, z := range zones {
				t.Run(z.name, func(t *testing.T) {
					nonUTC := utcInstant.In(z.loc)
					require.True(t, nonUTC.Equal(utcInstant))
					ids := queryTimedThings(t, db, func(q *dal.QueryBuilder) dal.IQueryBuilder {
						return q.WhereField("DtNext", dal.Equal, nonUTC)
					})
					require.Equal(t, []string{"r1"}, ids)
				})
			}
		})
	}

	// Direct unit check for *time.Time and non-time stored values in valuesEqualForStrategy.
	nonUTC := utcInstant.In(fixedZone)
	ptr := &utcInstant
	var nilPtr *time.Time
	require.True(t, valuesEqualForStrategy(ptr, nonUTC))
	require.False(t, valuesEqualForStrategy(nilPtr, nonUTC))
	require.False(t, valuesEqualForStrategy("not-a-time", nonUTC))
}
