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

// accessFieldListTest proves, on a real adapter, that a policy's field
// allow-list holds when the caller reaches for a hidden field indirectly
// (dalgo issue 148): under an alias that is itself allowed, in a selection
// where every column is refused, or in a filter or order the result never
// shows. Each case must be denied by the access layer before the adapter
// runs, on both read paths, and it is run under both shapes of allow-list:
// an enumerable one (the access layer projects the query) and one with a
// wildcard (the access layer falls back to redacting the result). Population
// and State are never in either list.
func accessFieldListTest(ctx context.Context, t *testing.T, db dal.DB) {
	lists := map[string][]string{
		"enumerable_list": {"Name", "Country"},
		"wildcard_list":   {"Name", "Count*"},
	}
	cities := func() dal.IQueryBuilder {
		return dal.From(dal.NewRootCollectionRef(models.CitiesCollection, "")).NewQuery()
	}
	for name, fields := range lists {
		t.Run(name, func(t *testing.T) {
			secured := access.MustSecureDB(db, access.WithDatabasePolicies(access.MustPolicy("cities-fields",
				access.Collection(models.CitiesCollection, access.Allow(access.Query, "query-allowed-fields").Fields(fields...)),
			)))
			// assertDenied runs the query on both read paths; the adapter must not
			// be reached, so a denial is the only acceptable answer. It pins the
			// reason too: the decision's code, the slot it was found in and the
			// columns it names (none when the expression cannot be checked).
			assertDenied := func(t *testing.T, q dal.Query, code access.ReasonCode, slot access.DecisionSlot, columns [][]string) {
				t.Helper()
				check := func(path, msg string, run func(ctx context.Context, tx dal.ReadTransaction) error) {
					err := secured.RunReadonlyTransaction(ctx, run, dal.TxWithMessage(msg))
					assert.ErrorIs(t, err, access.ErrAccessDenied, path)
					var denied *access.DeniedError
					if assert.ErrorAs(t, err, &denied, path) {
						assert.Equal(t, code, denied.Decision.Code, path)
						assert.Equal(t, slot, denied.Decision.Slot, path)
						assert.Equal(t, columns, denied.Decision.Columns, path)
					}
				}
				check("records reader", "access field list: records", func(ctx context.Context, tx dal.ReadTransaction) error {
					_, err := tx.ExecuteQueryToRecordsReader(ctx, q)
					return err
				})
				check("recordset reader", "access field list: recordset", func(ctx context.Context, tx dal.ReadTransaction) error {
					_, err := tx.ExecuteQueryToRecordsetReader(ctx, q)
					return err
				})
			}
			columnDenied := func(t *testing.T, q dal.Query, slot access.DecisionSlot, column string) {
				t.Helper()
				assertDenied(t, q, access.CodeColumnDenied, slot, [][]string{{column}})
			}

			t.Run("allowed_columns_still_read", func(t *testing.T) {
				// Positive control: the policy is not simply denying everything.
				q := cities().WhereField("Country", dal.Equal, "JP").OrderBy(dal.AscendingField("Name")).
					SelectColumns(dal.Column{Expression: dal.Field("Name")})
				rows, err := readMapRecords(ctx, secured, q, "access field list: allowed columns")
				if errors.Is(err, dal.ErrNotSupported) {
					t.Skip("column projection not supported by adapter:", err)
				}
				require.NoError(t, err)
				require.NotEmpty(t, rows)
				for _, row := range rows {
					require.Len(t, row, 1, "only the requested column comes back")
					assert.Contains(t, row, "Name")
					assert.NotContains(t, row, "Population")
					assert.NotContains(t, row, "State")
				}
			})
			t.Run("hidden_field_under_an_allowed_alias", func(t *testing.T) {
				columnDenied(t, cities().SelectColumns(dal.Column{Alias: "Name", Expression: dal.Field("Population")}), access.DecisionSlotFields, "Population")
				columnDenied(t, cities().SelectColumns(dal.Column{Alias: "Country", Expression: dal.Field("State")}), access.DecisionSlotFields, "State")
				columnDenied(t, cities().SelectColumns(
					dal.Column{Expression: dal.Field("Name")},
					dal.Column{Alias: "Count_hidden", Expression: dal.Field("Population")},
				), access.DecisionSlotFields, "Population")
			})
			t.Run("every_selected_column_refused", func(t *testing.T) {
				columnDenied(t, cities().SelectColumns(dal.Column{Expression: dal.Field("Population")}), access.DecisionSlotFields, "Population")
				// Columns are checked in order, so the first refused one is named.
				columnDenied(t, cities().SelectColumns(
					dal.Column{Expression: dal.Field("Population")},
					dal.Column{Alias: "where", Expression: dal.Field("State")},
				), access.DecisionSlotFields, "Population")
			})
			t.Run("aggregate_cannot_be_checked", func(t *testing.T) {
				// An aggregate is not a plain field, so its operand cannot be
				// held to the allow-list and the selection fails closed. The
				// same holds for an aggregate over an allowed field.
				assertDenied(t, cities().SelectColumns(dal.SumAs(dal.Field("Population"), "total")), access.CodeEnforcementUnsupported, access.DecisionSlotFields, nil)
				assertDenied(t, cities().SelectColumns(dal.CountAs(dal.Field("Name"), "n")), access.CodeEnforcementUnsupported, access.DecisionSlotFields, nil)
			})
			t.Run("filter_or_order_on_a_hidden_field", func(t *testing.T) {
				selectName := dal.Column{Expression: dal.Field("Name")}
				columnDenied(t, cities().WhereField("Population", dal.GreaterThen, 1).SelectColumns(selectName), access.DecisionSlotWhere, "Population")
				columnDenied(t, cities().WhereField("Name", dal.Equal, "Tokyo").WhereField("State", dal.Equal, "Tokyo").SelectColumns(selectName), access.DecisionSlotWhere, "State")
				columnDenied(t, cities().Where(dal.NewIsNullCondition(dal.Field("Population"))).SelectColumns(selectName), access.DecisionSlotWhere, "Population")
				columnDenied(t, cities().Where(dal.NewIsNotNullCondition(dal.Field("State"))).SelectColumns(selectName), access.DecisionSlotWhere, "State")
				columnDenied(t, cities().OrderBy(dal.DescendingField("Population")).SelectColumns(selectName), access.DecisionSlotFields, "Population")
				columnDenied(t, cities().WhereField("Population", dal.GreaterThen, 1).SelectKeysOnly(reflect.String), access.DecisionSlotWhere, "Population")
				columnDenied(t, cities().GroupBy(dal.Field("State")).SelectColumns(selectName), access.DecisionSlotFields, "State")
			})
			t.Run("having_on_a_hidden_field", func(t *testing.T) {
				selectCountry := dal.Column{Expression: dal.Field("Country")}
				columnDenied(t, cities().GroupBy(dal.Field("Country")).
					Having(dal.WhereField("Population", dal.GreaterThen, 1)).SelectColumns(selectCountry), access.DecisionSlotWhere, "Population")
				columnDenied(t, cities().GroupBy(dal.Field("Country")).
					Having(dal.NewIsNullCondition(dal.Field("State"))).SelectColumns(selectCountry), access.DecisionSlotWhere, "State")
			})
		})
	}
}
