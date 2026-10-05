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
// where every column is refused, or in a filter, order, HAVING, null test, join
// condition (at any depth) or scan order the result never shows. Each denial pins the decision's code, slot and
// column. An aggregate is held to the list by its operands: over allowed fields
// it runs and comes back under the name it was given, over a hidden field
// (alone or inside arithmetic) it is refused as ACL_COLUMN_DENIED. An allowed
// field selected under an alias comes back under that alias. Each refused case
// must be denied by the access layer before the adapter runs, on both read
// paths, and it is run under both shapes of allow-list: an enumerable one (the
// access layer projects the query) and one with a wildcard (the access layer
// falls back to redacting the result). Population and State are never in
// either list.
func accessFieldListTest(ctx context.Context, t *testing.T, db dal.DB) {
	lists := map[string][]string{
		"enumerable_list": {"Name", "Country", "AreaSqKm"},
		"wildcard_list":   {"Name", "Count*", "AreaSqKm"},
	}
	cities := func() dal.IQueryBuilder {
		return dal.From(dal.NewRootCollectionRef(models.CitiesCollection, "")).NewQuery()
	}
	for name, fields := range lists {
		t.Run(name, func(t *testing.T) {
			secured := access.MustSecureDB(db, access.WithDatabasePolicies(access.MustPolicy("cities-fields",
				access.Collection(models.CitiesCollection, access.Allow(access.Query, "query-allowed-fields").Fields(fields...)),
				access.Collection(joinedSourceCollection, access.Allow(access.Query, "query-joined-source")),
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
			aliased := func() dal.Query {
				return cities().OrderBy(dal.AscendingField("Name")).
					SelectColumns(dal.Column{Alias: "city", Expression: dal.Field("Name")})
			}
			countAll := dal.Count()
			countAll.Alias = "cities"
			aggregated := func() dal.Query {
				return cities().SelectColumns(countAll, dal.CountAs(dal.Field("Name"), "named"), dal.SumAs(dal.Field("AreaSqKm"), "area"))
			}
			t.Run("allowed_field_under_an_alias_comes_back_under_it", func(t *testing.T) {
				rows, err := readMapRecords(ctx, secured, aliased(), "access field list: allowed columns")
				if errors.Is(err, dal.ErrNotSupported) {
					t.Skip("column projection not supported by adapter:", err)
				}
				require.NoError(t, err)
				require.Len(t, rows, len(models.Cities))
				for _, row := range rows {
					require.Len(t, row, 1, "only the aliased column comes back")
					assert.NotEmpty(t, row["city"], "the allowed field's value under its alias")
				}
			})
			t.Run("allowed_field_under_an_alias_on_the_recordset_path", func(t *testing.T) {
				assertRecordsetRowCount(ctx, t, secured, aliased(), "access field list: allowed recordset", len(models.Cities))
			})
			t.Run("aggregates_over_allowed_fields_still_read", func(t *testing.T) {
				var area int
				for _, city := range models.Cities {
					area += city.AreaSqKm
				}
				rows, err := readMapRecords(ctx, secured, aggregated(), "access field list: allowed columns")
				if errors.Is(err, dal.ErrNotSupported) {
					t.Skip("aggregation not supported by adapter:", err)
				}
				require.NoError(t, err)
				require.Len(t, rows, 1)
				assert.EqualValues(t, len(models.Cities), rows[0]["cities"], "COUNT(*)")
				assert.EqualValues(t, len(models.Cities), rows[0]["named"], "COUNT(Name)")
				assert.InDelta(t, float64(area), rows[0]["area"], 0.5, "SUM(AreaSqKm)")
				assert.NotContains(t, rows[0], "Population")

				grouped := cities().GroupBy(dal.Field("Country")).SelectColumns(dal.Column{Expression: dal.Field("Country")}, countAll)
				rows, err = readMapRecords(ctx, secured, grouped, "access field list: allowed columns")
				require.NoError(t, err)
				assert.EqualValues(t, 2, indexByString(t, rows, "Country")["IN"]["cities"], "COUNT(*) per country")
			})
			t.Run("aggregates_over_allowed_fields_on_the_recordset_path", func(t *testing.T) {
				assertRecordsetRowCount(ctx, t, secured, aggregated(), "access field list: allowed recordset", 1)
			})
			// A grouped query that names no columns selects its group keys; the
			// list must not add its other allowed fields to that selection.
			groupedNoColumns := func() dal.Query {
				return cities().GroupBy(dal.Field("Country")).
					Having(dal.NewComparison(dal.Count().Expression, dal.GreaterThen, dal.Constant{Value: 1})).
					SelectKeysOnly(reflect.String)
			}
			t.Run("grouped_query_with_no_columns_reads_its_group_keys", func(t *testing.T) {
				rows, err := readMapRecords(ctx, secured, groupedNoColumns(), "access field list: allowed columns")
				if errors.Is(err, dal.ErrNotSupported) {
					t.Skip("grouped query with no columns not supported by adapter:", err)
				}
				require.NoError(t, err)
				var countries []string
				for _, row := range rows {
					require.Len(t, row, 1, "only the group key comes back")
					country, isString := row["Country"].(string)
					require.True(t, isString, "Country holds %v", row["Country"])
					countries = append(countries, country)
				}
				assert.ElementsMatch(t, []string{"CN", "IN"}, countries, "the countries with more than one city")
			})
			t.Run("grouped_query_with_no_columns_on_the_recordset_path", func(t *testing.T) {
				assertRecordsetRowCount(ctx, t, secured, groupedNoColumns(), "access field list: allowed recordset", 2)
			})
			t.Run("allowed_column_under_a_refused_name_never_returns_the_stored_field", func(t *testing.T) {
				// Population is in neither list. A column selected under that name
				// carries what its own expression produced, or nothing when the
				// adapter ignores the projection; it never carries the stored
				// Population of the row, whatever the adapter returns besides.
				names := map[string]bool{}
				for _, city := range models.Cities {
					names[city.Name] = true
				}
				rows, err := readMapRecords(ctx, secured,
					cities().SelectColumns(dal.Column{Alias: "Population", Expression: dal.Field("Name")}),
					"access field list: allowed columns")
				if errors.Is(err, dal.ErrNotSupported) {
					t.Skip("column projection not supported by adapter:", err)
				}
				require.NoError(t, err)
				for _, row := range rows {
					if value, ok := row["Population"]; ok {
						name, isString := value.(string)
						assert.True(t, isString && names[name], "Population holds %v, not a city name", value)
					}
				}
				rows, err = readMapRecords(ctx, secured,
					cities().SelectColumns(dal.CountAs(dal.Field("Name"), "Population")),
					"access field list: allowed columns")
				require.NoError(t, err)
				for _, row := range rows {
					if value, ok := row["Population"]; ok {
						assert.EqualValues(t, len(models.Cities), value, "COUNT(Name) under the name Population")
					}
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
			t.Run("aggregates_are_checked_by_operand", func(t *testing.T) {
				columnDenied(t, cities().SelectColumns(dal.SumAs(dal.Field("Population"), "total")), access.DecisionSlotFields, "Population")
				columnDenied(t, cities().SelectColumns(dal.CountAs(dal.Field("State"), "n")), access.DecisionSlotFields, "State")
				columnDenied(t, cities().SelectColumns(dal.MaxAs(dal.Binary(dal.Field("AreaSqKm"), dal.Add, dal.Field("Population")), "m")), access.DecisionSlotFields, "Population")
				columnDenied(t, cities().GroupBy(dal.Field("Country")).
					Having(dal.NewComparison(dal.SumAs(dal.Field("Population"), "").Expression, dal.GreaterThen, dal.Constant{Value: 1})).
					SelectColumns(dal.Column{Expression: dal.Field("Country")}), access.DecisionSlotWhere, "Population")
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
			// HAVING and ORDER BY may name an aggregate of the select list by its
			// alias. Under a list the alias is not a field, so the name an allowed
			// aggregate was given is let through and no other: the same name for an
			// aggregate over a hidden field, for a plain field or for nothing is
			// refused.
			aliasedGroups := func(having dal.Condition, order dal.OrderExpression, columns ...dal.Column) dal.Query {
				return cities().GroupBy(dal.Field("Country")).Having(having).OrderBy(order).SelectColumns(columns...)
			}
			moreThanOne := func(alias string) dal.Condition {
				return dal.NewComparison(dal.Field(alias), dal.GreaterThen, dal.Constant{Value: 1})
			}
			selectCountry := dal.Column{Expression: dal.Field("Country")}
			t.Run("having_and_order_by_over_the_alias_of_an_allowed_aggregate", func(t *testing.T) {
				q := aliasedGroups(moreThanOne("n"), dal.Descending(dal.Field("n")), selectCountry, dal.CountAs(dal.Field("Name"), "n"))
				// An adapter that does not run a HAVING or an ORDER BY over an
				// aggregate alias on its own skips the control: the unsecured
				// database is asked first.
				if _, err := readMapRecords(ctx, db, q, "access field list: aggregate alias probe"); err != nil {
					t.Skip("adapter does not run HAVING or ORDER BY over an aggregate alias:", err)
				}
				rows, err := readMapRecords(ctx, secured, q, "access field list: aggregate alias")
				require.NoError(t, err)
				var countries []string
				for _, row := range rows {
					country, isString := row["Country"].(string)
					require.True(t, isString, "Country holds %v", row["Country"])
					countries = append(countries, country)
					assert.Greater(t, numberOf(t, row["n"]), float64(1), "the groups kept have more than one city")
				}
				assert.ElementsMatch(t, []string{"CN", "IN"}, countries, "the countries with more than one city")
				for i := 1; i < len(rows); i++ {
					assert.GreaterOrEqual(t, numberOf(t, rows[i-1]["n"]), numberOf(t, rows[i]["n"]), "ordered by the alias, descending")
				}
			})
			t.Run("having_over_the_alias_is_refused_on_the_recordset_path", func(t *testing.T) {
				// The recordset path sends the aggregate under the caller's own alias
				// and renames nothing, so an alias the list does not allow is not a
				// name it may use there; the aggregate itself is.
				q := aliasedGroups(moreThanOne("n"), dal.Descending(dal.Field("Country")), selectCountry, dal.CountAs(dal.Field("Name"), "n"))
				err := secured.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
					_, err := tx.ExecuteQueryToRecordsetReader(ctx, q)
					return err
				}, dal.TxWithMessage("access field list: recordset"))
				assert.ErrorIs(t, err, access.ErrAccessDenied)
				var denied *access.DeniedError
				if assert.ErrorAs(t, err, &denied) {
					assert.Equal(t, access.CodeColumnDenied, denied.Decision.Code)
					assert.Equal(t, [][]string{{"n"}}, denied.Decision.Columns)
				}
				byAggregate := cities().GroupBy(dal.Field("Country")).
					Having(dal.NewComparison(dal.Count().Expression, dal.GreaterThen, dal.Constant{Value: 1})).
					SelectColumns(selectCountry, dal.CountAs(dal.Field("Name"), "n"))
				assertRecordsetRowCount(ctx, t, secured, byAggregate, "access field list: allowed recordset", 2)
			})
			t.Run("an_alias_that_is_not_an_allowed_aggregate_is_refused", func(t *testing.T) {
				columnDenied(t, aliasedGroups(moreThanOne("p"), dal.Descending(dal.Field("Country")), selectCountry, dal.SumAs(dal.Field("Population"), "p")),
					access.DecisionSlotWhere, "p")
				columnDenied(t, aliasedGroups(moreThanOne("c"), dal.Descending(dal.Field("Country")), selectCountry, dal.Column{Alias: "c", Expression: dal.Field("Country")}),
					access.DecisionSlotWhere, "c")
				columnDenied(t, aliasedGroups(moreThanOne("unknown"), dal.Descending(dal.Field("Country")), selectCountry, dal.CountAs(dal.Field("Name"), "n")),
					access.DecisionSlotWhere, "unknown")
				columnDenied(t, aliasedGroups(dal.NewComparison(dal.Count().Expression, dal.GreaterThen, dal.Constant{Value: 1}), dal.Descending(dal.Field("p")), selectCountry, dal.CountAs(dal.Field("Name"), "n"), dal.SumAs(dal.Field("Population"), "p")),
					access.DecisionSlotFields, "p")
			})
			// A join condition and a scan order read the stored fields of the
			// source they belong to, so the list applies to them as it does to a
			// filter. The joined collection is not in the fixture: a query that
			// names a hidden field is refused before the adapter runs.
			citiesRef := dal.NewRootCollectionRef(models.CitiesCollection, "c")
			joined := func(alias string) dal.CollectionRef { return dal.NewRootCollectionRef(joinedSourceCollection, alias) }
			joinOn := func(left, right dal.Expression) dal.Condition { return dal.NewComparison(left, dal.Equal, right) }
			citiesField := func(name string) dal.Expression { return dal.NewFieldRef("c", name) }
			selectCityName := func(from dal.FromSource) dal.StructuredQuery {
				return from.NewQuery().SelectColumns(dal.Column{Expression: dal.Field("Name")})
			}
			t.Run("join_on_a_hidden_field", func(t *testing.T) {
				columnDenied(t, selectCityName(dal.From(citiesRef).Join(dal.NewJoinedSource(joined("j"), dal.JoinInner,
					joinOn(citiesField("Population"), dal.NewFieldRef("j", "ref"))))), access.DecisionSlotWhere, "Population")
				columnDenied(t, selectCityName(dal.From(citiesRef).Join(dal.NewJoinedSource(joined("j"), dal.JoinInner,
					joinOn(dal.NewFieldRef("j", "ref"), citiesField("State"))))), access.DecisionSlotWhere, "State")
				columnDenied(t, selectCityName(dal.From(citiesRef).Join(dal.NewJoinedSource(joined("j"), dal.JoinInner,
					joinOn(citiesField("Population"), dal.Constant{Value: 1})))), access.DecisionSlotWhere, "Population")
			})
			t.Run("join_on_a_hidden_field_at_depth_two_and_three", func(t *testing.T) {
				depthTwo := dal.From(citiesRef).Join(dal.NewJoinedFrom(
					dal.From(joined("j")).Join(dal.NewJoinedSource(joined("k"), dal.JoinInner,
						joinOn(citiesField("Population"), dal.NewFieldRef("k", "ref")))),
					dal.JoinInner, joinOn(citiesField("Name"), dal.NewFieldRef("j", "ref"))))
				columnDenied(t, selectCityName(depthTwo), access.DecisionSlotWhere, "Population")
				depthThree := dal.From(citiesRef).Join(dal.NewJoinedFrom(
					dal.From(joined("j")).Join(dal.NewJoinedFrom(
						dal.From(joined("k")).Join(dal.NewJoinedSource(joined("l"), dal.JoinInner,
							joinOn(citiesField("State"), dal.NewFieldRef("l", "ref")))),
						dal.JoinInner, joinOn(dal.NewFieldRef("j", "ref"), dal.NewFieldRef("k", "ref")))),
					dal.JoinInner, joinOn(citiesField("Name"), dal.NewFieldRef("j", "ref"))))
				columnDenied(t, selectCityName(depthThree), access.DecisionSlotWhere, "State")
			})
			t.Run("scan_order_on_a_hidden_field", func(t *testing.T) {
				columnDenied(t, selectCityName(dal.From(citiesRef.WithScan(5, dal.AscendingField("Population")))), access.DecisionSlotFields, "Population")
				columnDenied(t, selectCityName(dal.From(citiesRef).Join(dal.NewJoinedSource(
					joined("j").WithScan(5, dal.Descending(citiesField("State"))), dal.JoinInner,
					joinOn(citiesField("Name"), dal.NewFieldRef("j", "ref"))))), access.DecisionSlotFields, "State")
				// A scan orders its own source whatever qualifier its field carries,
				// so a scan order of the listed source is held to the list when its
				// field is qualified by a joined source too.
				columnDenied(t, selectCityName(dal.From(citiesRef.WithScan(1, dal.AscendingField("Population"))).Join(dal.NewJoinedSource(
					joined("j"), dal.JoinLeft, joinOn(citiesField("Name"), dal.NewFieldRef("j", "ref"))))), access.DecisionSlotFields, "Population")
				columnDenied(t, selectCityName(dal.From(citiesRef.WithScan(1, dal.Ascending(dal.NewFieldRef("j", "Population")))).Join(dal.NewJoinedSource(
					joined("j"), dal.JoinLeft, joinOn(citiesField("Name"), dal.NewFieldRef("j", "ref"))))), access.DecisionSlotFields, "Population")
				columnDenied(t, selectCityName(dal.From(citiesRef.WithScan(1, dal.Ascending(dal.NewFieldRef(joinedSourceCollection, "Population")))).Join(dal.NewJoinedSource(
					joined("j"), dal.JoinLeft, joinOn(citiesField("Name"), dal.NewFieldRef("j", "ref"))))), access.DecisionSlotFields, "Population")
			})
			t.Run("join_fields_of_the_joined_source_are_not_held_to_the_list", func(t *testing.T) {
				// Negative control: Population here is a field of the joined source,
				// not of the listed one, so this policy does not refuse it. The
				// adapter may refuse a query it cannot run, so only a refusal by the
				// field list counts.
				run := func(q dal.Query) error {
					return secured.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
						_, err := tx.ExecuteQueryToRecordsReader(ctx, q)
						return err
					}, dal.TxWithMessage("access field list: joined source field"))
				}
				policy, _ := denialOf(run(selectCityName(dal.From(citiesRef).Join(dal.NewJoinedSource(joined("j"), dal.JoinInner,
					joinOn(dal.NewFieldRef("j", "Population"), citiesField("Name")))))))
				assert.NotEqual(t, "fields", policy, "a field of the joined source in a join condition")
				policy, _ = denialOf(run(selectCityName(dal.From(citiesRef).Join(dal.NewJoinedSource(
					joined("j").WithScan(5, dal.AscendingField("Population")), dal.JoinInner,
					joinOn(dal.NewFieldRef("j", "ref"), citiesField("Name")))))))
				assert.NotEqual(t, "fields", policy, "a field of the joined source in its own scan order")
			})
		})
	}

	// A secured database over another one is held to the fields both lists
	// allow. Whichever database lists AreaSqKm, the other does not, so a query
	// that names no columns, or a wildcard, must come back without it: a column
	// the outer database adds to such a query is one the inner database removes
	// from the result.
	t.Run("nested_secured_databases_return_the_fields_both_lists_allow", func(t *testing.T) {
		secureWith := func(db dal.DB, name string, fields ...string) dal.DB {
			return access.MustSecureDB(db, access.WithDatabasePolicies(access.MustPolicy(name,
				access.Collection(models.CitiesCollection, access.Allow(access.Query, name+"-fields").Fields(fields...)),
			)))
		}
		nestings := map[string]dal.DB{
			"inner_list_narrower": secureWith(secureWith(db, "inner", "Name", "Country"), "outer", "Name", "Country", "AreaSqKm"),
			"outer_list_narrower": secureWith(secureWith(db, "inner", "Name", "Country", "AreaSqKm"), "outer", "Name", "Country"),
		}
		shapes := map[string]dal.Query{
			"no_columns": cities().SelectKeysOnly(reflect.String),
			"wildcard":   cities().SelectColumns(dal.AllColumnsExcept("State")),
		}
		for nesting, secured := range nestings {
			for shape, q := range shapes {
				t.Run(nesting+"_"+shape, func(t *testing.T) {
					rows, err := readMapRecords(ctx, secured, q, "access field list: allowed columns")
					if errors.Is(err, dal.ErrNotSupported) {
						t.Skip("column projection not supported by adapter:", err)
					}
					require.NoError(t, err)
					require.Len(t, rows, len(models.Cities))
					for _, row := range rows {
						assert.NotContains(t, row, "AreaSqKm", "a field only one of the lists allows")
						assert.NotContains(t, row, "Population")
						assert.NotContains(t, row, "State")
						assert.NotEmpty(t, row["Name"], "a field both lists allow")
					}
				})
			}
		}
	})
}

// joinedSourceCollection is a collection the field-list policy allows without a
// field list. No test creates it: every query that reads it is refused before an
// adapter runs, or tolerates the adapter's answer.
const joinedSourceCollection = models.CitiesCollection + "_Joined"

// hiddenSourceCollection is a collection the sources policy denies. No test
// creates it: every query that reads it is refused before an adapter runs.
const hiddenSourceCollection = models.CitiesCollection + "_Hidden"

// accessSourcesTest proves that a policy is applied to every source a query
// reads, wherever the source sits: in a join at any depth, in a derived source,
// or in a subquery in any clause. With one collection denied, each query below
// reads it somewhere and must be refused, as a denial of that collection, on
// both read paths before the adapter runs. The same shapes are not refused when
// the collection is allowed, and a query whose nested source is allowed still
// runs and returns its rows.
func accessSourcesTest(ctx context.Context, t *testing.T, db dal.DB) {
	cities := dal.NewRootCollectionRef(models.CitiesCollection, "c")
	middle := dal.NewRootCollectionRef(models.CitiesCollection, "m")
	last := dal.NewRootCollectionRef(models.CitiesCollection, "n")
	hidden := dal.NewRootCollectionRef(hiddenSourceCollection, "h")
	on := func(left, right string) dal.Condition {
		return dal.NewComparison(dal.NewFieldRef(left, "Name"), dal.Equal, dal.NewFieldRef(right, "Name"))
	}
	hiddenRows := func() dal.StructuredQuery { return dal.From(hidden).NewQuery().SelectKeysOnly(reflect.String) }
	scalar := func() dal.Expression { return dal.NewQueryExpression(hiddenRows(), "x") }
	citiesQuery := func() dal.IQueryBuilder { return dal.From(cities).NewQuery() }
	keys := func(builder dal.IQueryBuilder) dal.StructuredQuery { return builder.SelectKeysOnly(reflect.String) }

	shapes := []struct {
		name  string
		build func() dal.StructuredQuery
	}{
		{"join_at_depth_two", func() dal.StructuredQuery {
			return keys(dal.From(cities).Join(dal.NewJoinedFrom(
				dal.From(middle).Join(dal.NewJoinedSource(hidden, dal.JoinInner, on("m", "h"))),
				dal.JoinInner, on("c", "m"))).NewQuery())
		}},
		{"join_at_depth_three", func() dal.StructuredQuery {
			return keys(dal.From(cities).Join(dal.NewJoinedFrom(
				dal.From(middle).Join(dal.NewJoinedFrom(
					dal.From(last).Join(dal.NewJoinedSource(hidden, dal.JoinInner, on("n", "h"))),
					dal.JoinInner, on("m", "n"))),
				dal.JoinInner, on("c", "m"))).NewQuery())
		}},
		{"derived_source", func() dal.StructuredQuery {
			return keys(dal.From(dal.NewQuerySource(hiddenRows(), "d")).NewQuery())
		}},
		{"derived_source_inside_a_join", func() dal.StructuredQuery {
			return keys(dal.From(cities).Join(dal.NewJoinedSource(dal.NewQuerySource(hiddenRows(), "d"), dal.JoinInner, on("c", "d"))).NewQuery())
		}},
		{"exists", func() dal.StructuredQuery { return keys(citiesQuery().Where(dal.NewExistsCondition(hiddenRows()))) }},
		{"not_exists", func() dal.StructuredQuery { return keys(citiesQuery().Where(dal.NewNotExistsCondition(hiddenRows()))) }},
		{"scalar_subquery_in_the_select_list", func() dal.StructuredQuery {
			return citiesQuery().SelectColumns(dal.Column{Alias: "x", Expression: scalar()})
		}},
		{"scalar_subquery_in_where", func() dal.StructuredQuery {
			return keys(citiesQuery().Where(dal.NewComparison(dal.Field("Name"), dal.Equal, scalar())))
		}},
		{"scalar_subquery_in_having", func() dal.StructuredQuery {
			return keys(citiesQuery().GroupBy(dal.Field("Country")).Having(dal.NewComparison(dal.Count().Expression, dal.Equal, scalar())))
		}},
		{"scalar_subquery_in_order_by", func() dal.StructuredQuery {
			return keys(citiesQuery().OrderBy(dal.Ascending(scalar())))
		}},
		{"scalar_subquery_in_a_join_on", func() dal.StructuredQuery {
			return keys(dal.From(cities).Join(dal.NewJoinedSource(middle, dal.JoinInner, dal.NewComparison(dal.NewFieldRef("m", "Name"), dal.Equal, scalar()))).NewQuery())
		}},
	}

	denying := access.MustSecureDB(db, access.WithDatabasePolicies(access.MustPolicy("sources-denied",
		access.Root(access.Allow(access.Query, "ordinary collections")),
		access.Collection(hiddenSourceCollection, access.Deny(access.Query, "hidden collection")),
	)))
	allowing := access.MustSecureDB(db, access.WithDatabasePolicies(access.MustPolicy("sources-allowed",
		access.Root(access.Allow(access.Query, "ordinary collections")),
	)))
	refused := func(t *testing.T, q dal.Query, msg string, read func(ctx context.Context, tx dal.ReadTransaction, q dal.Query) error) {
		t.Helper()
		err := denying.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
			return read(ctx, tx, q)
		}, dal.TxWithMessage(msg))
		var denied *access.DeniedError
		if assert.ErrorIs(t, err, access.ErrAccessDenied, msg) && assert.ErrorAs(t, err, &denied, msg) {
			assert.Equal(t, "/"+hiddenSourceCollection, denied.Decision.Resource.String(), "%s: the denied resource", msg)
		}
	}
	for _, shape := range shapes {
		t.Run(shape.name, func(t *testing.T) {
			refused(t, shape.build(), "access sources: records", func(ctx context.Context, tx dal.ReadTransaction, q dal.Query) error {
				_, err := tx.ExecuteQueryToRecordsReader(ctx, q)
				return err
			})
			refused(t, shape.build(), "access sources: recordset", func(ctx context.Context, tx dal.ReadTransaction, q dal.Query) error {
				_, err := tx.ExecuteQueryToRecordsetReader(ctx, q)
				return err
			})
			// Allowing the collection lifts the refusal. The adapter may still
			// answer a query it cannot run with a denial of its own, among other
			// errors, so only a refusal by this policy, or of the collection,
			// counts.
			err := allowing.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
				_, err := tx.ExecuteQueryToRecordsReader(ctx, shape.build())
				return err
			}, dal.TxWithMessage("access sources: allowed"))
			policy, resource := denialOf(err)
			assert.NotEqual(t, "sources-allowed", policy, "the collection is allowed")
			assert.NotEqual(t, "/"+hiddenSourceCollection, resource, "the collection is allowed")
		})
	}
	t.Run("an_allowed_nested_source_still_reads", func(t *testing.T) {
		// A query with nested sources is read one source at a time, each source by
		// the scan DALgo's generic engine issues: a query with no target record and
		// no key kind. An adapter that cannot answer that scan skips this control,
		// as it skips any query it does not support. The scan is tried on the
		// unsecured database first, so the control runs only where the adapter
		// serves it.
		scan := dal.From(dal.NewRootCollectionRef(models.CitiesCollection, "i")).NewQuery().SelectIntoRecord(nil)
		probe := db.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
			reader, err := tx.ExecuteQueryToRecordsReader(ctx, scan)
			if err != nil {
				return err
			}
			defer func() { _ = reader.Close() }()
			_, err = reader.Next()
			return err
		}, dal.TxWithMessage("access sources: nested read probe"))
		if errors.Is(probe, dal.ErrNotSupported) {
			t.Skip("adapter does not serve the scan of a nested source:", probe)
		}
		inner := dal.From(dal.NewRootCollectionRef(models.CitiesCollection, "i")).NewQuery().SelectKeysOnly(reflect.String)
		q := keys(citiesQuery().Where(dal.NewExistsCondition(inner)))
		var records []record.Record
		err := denying.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) (err error) {
			records, err = dal.ExecuteQueryAndReadAllToRecords(ctx, q, tx)
			return err
		}, dal.TxWithMessage("access sources: nested read"))
		require.NoError(t, err)
		assert.Len(t, records, len(models.Cities), "an uncorrelated EXISTS over a populated allowed collection keeps every row")
	})
}

// numberOf is a value an adapter returned for a count or a sum as a float.
func numberOf(t *testing.T, value any) float64 {
	t.Helper()
	number := reflect.ValueOf(value)
	require.True(t, number.IsValid() && number.CanConvert(reflect.TypeOf(0.0)), "%v is not a number", value)
	return number.Convert(reflect.TypeOf(0.0)).Float()
}

// denialOf returns the policy and the resource of an access denial in err, and
// empty strings for any other answer, an absent error included.
func denialOf(err error) (policy, resource string) {
	var denied *access.DeniedError
	if errors.As(err, &denied) {
		return denied.Decision.Policy, denied.Decision.Resource.String()
	}
	return "", ""
}
