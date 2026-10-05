package dal

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestOrderedSourceTimestampValuesFollowGenericRawFieldPolicy(t *testing.T) {
	instant := time.Date(2024, 1, 1, 1, 0, 0, 0, time.FixedZone("plus2", 2*3600))
	malformedTag := reflect.StructTag(`json:"\q"`)
	if name := malformedTag.Get("json"); name != "" {
		t.Fatalf("malformed StructTag unexpectedly has JSON name %q", name)
	}
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
	malformedType := reflect.StructOf([]reflect.StructField{{Name: "Stamp", Type: reflect.TypeOf(time.Time{}), Tag: malformedTag}})
	malformedRow := reflect.New(malformedType).Elem()
	malformedRow.Field(0).Set(reflect.ValueOf(instant))
	encoded, err := json.Marshal(malformedRow.Interface())
	if err != nil {
		t.Fatal(err)
	}
	var jsonFields map[string]any
	if err := json.Unmarshal(encoded, &jsonFields); err != nil {
		t.Fatal(err)
	}
	if _, ok := jsonFields["Stamp"]; !ok || len(jsonFields) != 1 {
		t.Fatalf("malformed tag did not fall back to Stamp: %s", encoded)
	}
	malformedSource := From(NewRootCollectionRef("events", "e")).NewQuery().SelectColumns(Column{
		Alias: "first", Expression: NewOrderedAggregate(FIRST, []OrderExpression{AscendingField("Stamp")}, Field("value")),
	})
	values := newAggregationSourceQuery(malformedSource, false).(aggregationSourceQuery).OrderedAggregateSourceValues(malformedRow.Interface())
	if stamp, ok := values["Stamp"].(time.Time); !ok || !stamp.Equal(instant) {
		t.Fatalf("malformed-tag timestamp extraction = %v, want %v", values, instant)
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
