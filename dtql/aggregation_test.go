package dtql

import (
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

func TestAggregationYAMLRoundTrip(t *testing.T) {
	revenue := dal.SumAs(dal.Binary(dal.Field("quantity"), dal.Multiply, dal.Field("unit_price")), "revenue")
	customers := dal.CountDistinctAs(dal.Field("customer_id"), "customers")
	q := dal.From(dal.NewRootCollectionRef("orders", "")).NewQuery().
		Where(dal.WhereField("status", dal.Equal, "paid")).
		GroupBy(dal.Field("country")).
		Having(dal.NewComparison(dal.Field("revenue"), dal.GreaterThen, dal.NewConstant(10_000))).
		OrderBy(dal.Descending(dal.Field("revenue"))).
		Limit(10).
		SelectColumns(
			dal.Column{Expression: dal.Field("country")},
			customers,
			revenue,
		)

	encoded, err := Serialize(q)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, fragment := range []string{"groupBy:", "having:", "aggregate:", "distinct: true", "binary:", "columns:"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("missing %q in:\n%s", fragment, text)
		}
	}
	if strings.Index(text, "columns:") < strings.Index(text, "orderBy:") {
		t.Fatalf("columns must be the final DTQL pipeline stage:\n%s", text)
	}
	decoded, err := Deserialize(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !Equal(q, decoded) {
		t.Fatalf("round trip differs:\n%s", text)
	}
}

func TestAggregationYAMLOmittedColumnsDefaultsToGroupBy(t *testing.T) {
	q, err := Deserialize([]byte("from: {name: orders}\ngroupBy:\n  - field: country\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Columns()) != 0 {
		t.Fatalf("explicit columns = %d, want 0", len(q.Columns()))
	}
	effective := dal.EffectiveAggregationColumns(q)
	if len(effective) != 1 || effective[0].Expression.String() != "country" {
		t.Fatalf("effective columns = %#v", effective)
	}
}

func TestAggregationYAMLRejectsNonGroupedProjection(t *testing.T) {
	_, err := Deserialize([]byte("from: {name: customers}\ngroupBy: [{field: country}]\ncolumns: [{field: city}]\n"))
	if err == nil || !strings.Contains(err.Error(), "neither aggregated nor present in GROUP BY") {
		t.Fatalf("got %v", err)
	}
}

func TestQualifiedFieldYAMLRoundTrip(t *testing.T) {
	qualified := dal.NewFieldRef("o", "country")
	q := dal.From(dal.NewRootCollectionRef("orders", "o")).NewQuery().
		GroupBy(qualified).
		SelectColumns(dal.Column{Expression: qualified}, dal.SumAs(dal.NewFieldRef("o", "amount"), "total"))

	encoded, err := Serialize(q)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), "field: country\n    source: o") {
		t.Fatalf("qualified field is not structural:\n%s", encoded)
	}
	decoded, err := Deserialize(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !Equal(q, decoded) {
		t.Fatalf("qualified field round trip differs:\n%s", encoded)
	}
}

func TestQualifiedFieldYAMLRejectsSourceWithoutField(t *testing.T) {
	_, err := Deserialize([]byte("from: {name: orders}\ncolumns: [{source: o, value: 1}]\n"))
	if err == nil || !strings.Contains(err.Error(), "source is valid only with field") {
		t.Fatalf("got %v", err)
	}
}

func TestQualifiedFieldYAMLRejectsEmptyAndWildcardSiblingSource(t *testing.T) {
	for _, document := range []string{
		"from: {name: orders}\ncolumns: [{field: country, source: ''}]\n",
		"from: {name: orders}\ncolumns: [{source: o, wildcard: {exclude: [secret]}}]\n",
	} {
		if _, err := Deserialize([]byte(document)); err == nil {
			t.Fatalf("expected source validation error for:\n%s", document)
		}
	}
}
