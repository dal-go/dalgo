package end2end

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/dal-go/dalgo/access"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/end2end/models"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// accessConditionsTest proves row-level access conditions on a real adapter:
// a query narrowed by a policy's `where` must return exactly the rows the
// condition selects (paging intact), a point read of a row outside the
// condition must be denied, and a string literal with a quote must not break
// the adapter. It runs over the cities fixture already seeded by the query
// suite. An adapter that cannot execute the conjoined condition reports
// dal.ErrNotSupported and the sub-test is skipped, per the suite's contract.
func accessConditionsTest(ctx context.Context, t *testing.T, db dal.DB) {
	countryOfCaller := dal.WhereField("Country", dal.Equal, dal.NewParam("country"))
	policy := access.MustPolicy("cities-by-country",
		access.Collection(models.CitiesCollection, access.Allow(access.Query, "query-own-country").Where(countryOfCaller)),
		access.Scope(models.CitiesCollection, access.AnyID, access.Allow(access.Get|access.Exists, "read-own-country").Where(countryOfCaller)),
	)
	secured := access.MustSecureDB(db, access.WithDatabasePolicies(policy))
	const country = "JP"
	callerCtx := access.WithVariables(ctx, map[string]any{"country": country})

	var expected []string
	var otherCountryID string
	for _, city := range models.Cities {
		if city.Country == country {
			expected = append(expected, models.CityID(city))
		} else if otherCountryID == "" {
			otherCountryID = models.CityID(city)
		}
	}
	sort.Strings(expected)
	require.NotEmpty(t, expected, "fixture must contain cities in %s", country)
	require.NotEmpty(t, otherCountryID, "fixture must contain a city outside %s", country)

	newCityRecord := func() record.Record {
		return record.NewRecordWithIncompleteKey(models.CitiesCollection, reflect.String, &models.City{})
	}
	// queriesRan records whether the adapter executed a narrowed query; point
	// reads are only meaningful on an adapter that did.
	queriesRan := false
	queryIDs := func(t *testing.T, q dal.StructuredQuery) []string {
		var ids []string
		err := secured.RunReadonlyTransaction(callerCtx, func(ctx context.Context, tx dal.ReadTransaction) error {
			records, err := dal.ExecuteQueryAndReadAllToRecords(ctx, q, tx)
			if err != nil {
				return err
			}
			for _, rec := range records {
				ids = append(ids, rec.Key().ID.(string))
			}
			return nil
		}, dal.TxWithMessage("access conditions"))
		if errors.Is(err, dal.ErrNotSupported) {
			t.Skip("adapter does not support the conjoined condition:", err)
		}
		require.NoError(t, err)
		queriesRan = true
		sort.Strings(ids)
		return ids
	}

	t.Run("query_is_narrowed_to_the_callers_country", func(t *testing.T) {
		q := dal.From(dal.NewRootCollectionRef(models.CitiesCollection, "")).NewQuery().SelectIntoRecord(newCityRecord)
		assert.Equal(t, expected, queryIDs(t, q))
	})
	t.Run("callers_own_condition_is_conjoined", func(t *testing.T) {
		q := dal.From(dal.NewRootCollectionRef(models.CitiesCollection, "")).NewQuery().
			WhereField("IsCapital", dal.Equal, true).SelectIntoRecord(newCityRecord)
		var want []string
		for _, city := range models.Cities {
			if city.Country == country && city.IsCapital {
				want = append(want, models.CityID(city))
			}
		}
		sort.Strings(want)
		assert.Equal(t, want, queryIDs(t, q))
	})
	t.Run("limit_applies_after_narrowing", func(t *testing.T) {
		q := dal.From(dal.NewRootCollectionRef(models.CitiesCollection, "")).NewQuery().Limit(1).SelectIntoRecord(newCityRecord)
		ids := queryIDs(t, q)
		require.Len(t, ids, 1)
		assert.Contains(t, expected, ids[0])
	})
	t.Run("quoted_literal_does_not_break_the_adapter", func(t *testing.T) {
		q := dal.From(dal.NewRootCollectionRef(models.CitiesCollection, "")).NewQuery().
			WhereField("Name", dal.Equal, "O'Hare").SelectIntoRecord(newCityRecord)
		assert.Empty(t, queryIDs(t, q))
	})
	t.Run("missing_variable_denies_before_the_adapter", func(t *testing.T) {
		q := dal.From(dal.NewRootCollectionRef(models.CitiesCollection, "")).NewQuery().SelectIntoRecord(newCityRecord)
		err := secured.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
			_, err := tx.ExecuteQueryToRecordsReader(ctx, q)
			return err
		}, dal.TxWithMessage("access conditions"))
		assert.ErrorIs(t, err, access.ErrAccessDenied)
	})
	t.Run("point_reads_follow_the_condition", func(t *testing.T) {
		if !queriesRan {
			t.Skip("adapter did not execute a narrowed query; point reads are not exercised")
		}
		own := record.NewRecordWithData(record.NewKeyWithID(models.CitiesCollection, expected[0]), &models.City{})
		require.NoError(t, secured.Get(callerCtx, own))
		require.True(t, own.Exists())
		assert.Equal(t, country, own.Data().(*models.City).Country)

		other := &models.City{}
		err := secured.Get(callerCtx, record.NewRecordWithData(record.NewKeyWithID(models.CitiesCollection, otherCountryID), other))
		assert.ErrorIs(t, err, access.ErrAccessDenied)
		assert.Equal(t, models.City{}, *other, "denied record must carry no data")

		exists, err := secured.Exists(callerCtx, record.NewKeyWithID(models.CitiesCollection, expected[0]))
		require.NoError(t, err)
		assert.True(t, exists)
		exists, err = secured.Exists(callerCtx, record.NewKeyWithID(models.CitiesCollection, otherCountryID))
		assert.ErrorIs(t, err, access.ErrAccessDenied)
		assert.False(t, exists)
	})
	t.Run("writes_follow_the_condition", func(t *testing.T) {
		if !queriesRan {
			t.Skip("adapter did not execute a narrowed query; writes are not exercised")
		}
		writer := access.MustSecureDB(db, access.WithDatabasePolicies(access.MustPolicy("cities-writes",
			access.Scope(models.CitiesCollection, access.AnyID, access.Allow(access.Update|access.Delete, "edit-own-country").Where(countryOfCaller)),
		)))
		touch := func(id string) error {
			return writer.RunReadwriteTransaction(callerCtx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
				return tx.Update(ctx, record.NewKeyWithID(models.CitiesCollection, id), []update.Update{update.ByFieldName("HasAirport", true)})
			}, dal.TxWithMessage("access conditions write"))
		}
		require.NoError(t, touch(expected[0]), "update of an own-country city")
		assert.ErrorIs(t, touch(otherCountryID), access.ErrAccessDenied, "update of another country's city")
		err := writer.RunReadwriteTransaction(callerCtx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			return tx.Delete(ctx, record.NewKeyWithID(models.CitiesCollection, otherCountryID))
		}, dal.TxWithMessage("access conditions write"))
		assert.ErrorIs(t, err, access.ErrAccessDenied, "delete of another country's city")
		other := &models.City{}
		require.NoError(t, db.Get(ctx, record.NewRecordWithData(record.NewKeyWithID(models.CitiesCollection, otherCountryID), other)))
		assert.NotEqual(t, country, other.Country, "the other country's city must be untouched")
	})
}
