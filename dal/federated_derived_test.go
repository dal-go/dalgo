package dal

import (
	"fmt"
	"testing"
)

func TestDerivedOutputNames(t *testing.T) {
	base := From(NewRootCollectionRef("Invoice", "i")).NewQuery().SelectIntoRecord(nil)
	scalar := NewQueryExpression(base, "Latest")
	for _, tc := range []struct {
		name    string
		query   StructuredQuery
		columns []Column
		want    string
	}{
		{name: "nil query", want: "[]"},
		{name: "implicit projection", query: base, want: "[]"},
		{name: "named columns", query: base, columns: []Column{{Expression: Field("Total")}, {Expression: Field("BillingCountry"), Alias: "Country"}, {Expression: scalar}}, want: "[Total Country Latest]"},
		{name: "unnamed aggregate", query: base, columns: []Column{{Expression: Field("Total")}, {Expression: NewAggregate("SUM", false, Field("Total"))}}, want: "[]"},
		{name: "wildcard", query: base, columns: []Column{{Wildcard: &WildcardProjection{Source: "i"}}}, want: "[]"},
	} {
		query := tc.query
		if query != nil && tc.columns != nil {
			query = WithColumns(query, tc.columns)
		}
		if got := fmt.Sprint(derivedOutputNames(query)); got != tc.want {
			t.Errorf("%s: %s, want %s", tc.name, got, tc.want)
		}
	}
}
