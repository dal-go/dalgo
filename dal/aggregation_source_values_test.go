package dal

import (
	"reflect"
	"testing"
	"time"
)

func TestOrderedSourceTimestampValuesFollowGenericRawFieldPolicy(t *testing.T) {
	instant := time.Date(2024, 1, 1, 1, 0, 0, 0, time.FixedZone("plus2", 2*3600))
	type nested struct {
		Stamp *time.Time `json:"stamp"`
	}
	type row struct {
		Detail *nested `json:"detail"`
	}
	for _, tc := range []struct {
		name string
		raw  any
		key  string
	}{
		{"plain timestamp", struct {
			At time.Time `json:"at"`
		}{instant}, "at"},
		{"nested pointer", row{Detail: &nested{Stamp: &instant}}, "detail.stamp"},
		{"nil timestamp pointer", row{Detail: &nested{}}, "detail.stamp"},
		{"custom row JSON", writtenRow{At: instant, Text: "literal"}, "at"},
		{"custom nested JSON", nestedRow{Meta: stampObject{When: instant, Text: "literal"}}, "meta.when"},
		{"custom map JSON", writtenMap{"at": instant, "text": "literal"}, "at"},
		{"ambiguous embedded", promotedRow{promotedDupA: promotedDupA{Dup: instant}, promotedDupB: promotedDupB{Dup: instant}}, "Dup"},
		{"invalid tag", struct {
			Stamp time.Time `json:"bad\\name"`
		}{instant}, "Stamp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := From(NewRootCollectionRef("events", "e")).NewQuery().SelectColumns(Column{
				Alias: "first", Expression: NewOrderedAggregate(FIRST, []OrderExpression{AscendingField(tc.key)}, Field("value")),
			})
			source := newAggregationSourceQuery(q, false).(aggregationSourceQuery)
			got := source.OrderedAggregateSourceValues(tc.raw)
			formatted := map[string]any{}
			for name, value := range got {
				formatted[name] = value.(time.Time).UTC().Format(timestampSortLayout)
			}
			want, err := buildSortValues(tc.raw, []string{tc.key})
			if err != nil {
				t.Fatal(err)
			}
			if want == nil {
				want = map[string]any{}
			}
			if !reflect.DeepEqual(formatted, want) {
				t.Fatalf("source timestamps = %v, generic sort values = %v", formatted, want)
			}
		})
	}
	q := From(NewRootCollectionRef("events", "e")).NewQuery().SelectColumns(Column{
		Alias: "first", Expression: NewOrderedAggregate(FIRST,
			[]OrderExpression{Ascending(NewFieldRef("other", "at")), Ascending(NewFieldRef("e", "at"))}, Field("value")),
	})
	got := newAggregationSourceQuery(q, false).(aggregationSourceQuery).OrderedAggregateSourceValues(struct {
		At time.Time `json:"at"`
	}{instant})
	if len(got) != 1 || !got["at"].(time.Time).Equal(instant) {
		t.Fatalf("source-filtered timestamps = %v", got)
	}
}
