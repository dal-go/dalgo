package dalgo2memory

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

func TestTimestampOrderKeepsSourceStringsAndProjectionShape(t *testing.T) {
	ctx := context.Background()
	type source struct {
		At string `json:"at"`
	}
	type target struct {
		At time.Time `json:"at"`
	}
	backend := newDatabase(WithSchema(false, WithCollection[source]("events", nil)))
	for id, at := range map[string]string{"whole": "2026-10-09T12:01:00Z", "fraction": "2026-10-09T12:01:00.1Z"} {
		if err := backend.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("events", id), &source{At: at})); err != nil {
			t.Fatal(err)
		}
	}
	query := dal.From(dal.NewRootCollectionRef("events", "")).NewQuery().OrderBy(dal.AscendingField("at")).Limit(1).SelectIntoRecord(func() record.Record { return record.NewRecordWithData(record.NewKeyWithID("events", ""), new(target)) })
	rows, err := dal.ExecuteQueryAndReadAllToRecords(ctx, query, dal.NewDB(backend))
	if err != nil || len(rows) != 1 || rows[0].Key().ID != "fraction" {
		t.Fatalf("result type overrode source string: %v %v", rows, err)
	}

	type timestampSource struct {
		At time.Time `json:"at"`
	}
	backend = newDatabase(WithSchema(false, WithCollection[timestampSource]("events", nil)))
	for id, at := range map[string]string{"early": "2026-10-09T13:01:00+02:00", "late": "2026-10-09T12:01:00Z"} {
		instant, err := time.Parse(time.RFC3339, at)
		if err != nil {
			t.Fatal(err)
		}
		if err := backend.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("events", id), &timestampSource{At: instant})); err != nil {
			t.Fatal(err)
		}
	}
	projection := dal.From(dal.NewRootCollectionRef("events", "")).NewQuery().OrderBy(dal.AscendingField("at")).Limit(1).SelectColumns(dal.Column{Alias: "stamp", Expression: dal.Field("at")})
	rows, err = dal.ExecuteQueryAndReadAllToRecords(ctx, projection, dal.NewDB(backend))
	if err != nil || len(rows) != 1 || rows[0].Key().ID != "early" {
		t.Fatalf("offset chronology: %v %v", rows, err)
	}
	encoded, err := json.Marshal(rows[0].Data())
	if err != nil || string(encoded) != `{"stamp":"2026-10-09T13:01:00+02:00"}` {
		t.Fatalf("projection shape changed: %s %v", encoded, err)
	}
}

func TestTimestampOrderResolvesJSONDeclarationsWithoutInventingNullValues(t *testing.T) {
	type nested struct {
		At *time.Time `json:"stamp"`
	}
	type event struct {
		Detail *nested   `json:"detail"`
		Hidden time.Time `json:"-"`
	}
	instant := time.Date(2026, 10, 9, 12, 1, 0, 0, time.UTC)
	backend := newDatabase()
	ctx := context.Background()
	for id, value := range map[string]*event{"null": {}, "whole": {Detail: &nested{At: &instant}}} {
		if err := backend.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("events", id), value)); err != nil {
			t.Fatal(err)
		}
	}
	q := dal.From(dal.NewRootCollectionRef("events", "")).NewQuery().OrderBy(dal.AscendingField("detail.stamp")).SelectIntoRecord(func() record.Record {
		return record.NewRecordWithData(record.NewIncompleteKey("events", reflect.String, nil), new(event))
	})
	rows, err := dal.ExecuteQueryAndReadAllToRecords(ctx, q, dal.NewDB(backend))
	if err != nil || len(rows) != 2 {
		t.Fatalf("nested timestamp: %v %v", rows, err)
	}
	for _, row := range rows {
		if row.Key().ID == "null" && row.Data().(*event).Detail != nil {
			t.Fatal("null became a zero timestamp")
		}
	}
	if orderedTimestampField(new(event), "Hidden") != nil || orderedTimestampField(new(event), "private") != nil {
		t.Fatal("non-JSON field used as timestamp declaration")
	}
}

type TimestampOrderEmbedded struct {
	At time.Time `json:"at"`
}
type timestampOrderEmbeddedRow struct{ *TimestampOrderEmbedded }
type timestampOrderMapKey string

type timestampOrderOpaqueMap map[string]time.Time

func (timestampOrderOpaqueMap) MarshalJSON() ([]byte, error) { return []byte(`{"at":"literal"}`), nil }

func TestTimestampOrderOpaqueAndAmbiguousJSONDeclarations(t *testing.T) {
	type left struct {
		At time.Time `json:"at"`
	}
	ambiguousType := reflect.StructOf([]reflect.StructField{
		{Name: "One", Type: reflect.TypeFor[time.Time](), Tag: `json:"at"`},
		{Name: "Two", Type: reflect.TypeFor[time.Time](), Tag: `json:"at"`},
	})
	type ignored struct {
		At time.Time `json:"-"`
	}
	type cycle struct{ *cycle }
	type dynamic struct{ Value any }
	type namedPrivate struct {
		left `json:"private"`
	}
	for _, tc := range []struct {
		name  string
		value any
		path  string
	}{
		{"nil", nil, "at"}, {"scalar", 7, "at"},
		{"nil-interface", new(dynamic), "Value.at"},
		{"non-string-map", map[int]time.Time{1: {}}, "at"},
		{"opaque-map", timestampOrderOpaqueMap{}, "at"},
		{"opaque-struct", new(marshaledProjection), "at"},
		{"ambiguous", reflect.New(ambiguousType).Interface(), "at"}, {"ignored", new(ignored), "at"},
		{"cyclic", new(cycle), "at"}, {"private", new(namedPrivate), "private"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if value := orderedTimestampField(tc.value, tc.path); value != nil {
				t.Fatalf("opaque field inferred as timestamp: %T", value)
			}
		})
	}
	type tagged struct {
		At time.Time `json:"at"`
	}
	type Plain struct{ At string }
	type TagWins struct {
		Plain
		tagged
	}
	if _, ok := orderedTimestampField(new(TagWins), "at").(time.Time); !ok {
		t.Fatal("explicit JSON timestamp tag lost")
	}
	type TaggedCapital struct {
		Stamp time.Time `json:"At"`
	}
	type Competing struct {
		Plain
		TaggedCapital
	}
	if _, ok := orderedTimestampField(new(Competing), "At").(time.Time); !ok {
		t.Fatal("tagged field did not dominate untagged field")
	}
	if _, ok := orderedTimestampField(&timestampOrderEmbeddedRow{&TimestampOrderEmbedded{}}, "at").(time.Time); !ok {
		t.Fatal("populated embedded declaration lost")
	}
}

func TestTimestampOrderMalformedTypedValuesFailBeforeSorting(t *testing.T) {
	type typed struct {
		At time.Time `json:"at"`
	}
	q := dal.From(dal.NewRootCollectionRef("events", "")).NewQuery().OrderBy(dal.AscendingField("at")).SelectIntoRecord(nil)
	for _, value := range []any{"bad-date", 42} {
		if _, err := orderedTimestampSources([]memoryRow{{id: "bad", data: map[string]any{"at": value}}}, q, func() any { return new(typed) }); err == nil {
			t.Fatalf("invalid stored timestamp accepted: %v", value)
		}
		backend := newDatabase()
		ctx := context.Background()
		if err := backend.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("events", "bad"), map[string]any{"at": value})); err != nil {
			t.Fatal(err)
		}
		query := dal.From(dal.NewRootCollectionRef("events", "")).NewQuery().OrderBy(dal.AscendingField("at")).Limit(1).SelectIntoRecord(func() record.Record { return record.NewRecordWithData(record.NewKeyWithID("events", ""), new(typed)) })
		if _, err := dal.ExecuteQueryAndReadAllToRecords(ctx, query, dal.NewDB(backend)); err == nil {
			t.Fatalf("query hid malformed timestamp: %v", value)
		}
	}
	instant := time.Date(2026, 10, 9, 12, 1, 0, 0, time.UTC)
	values, err := orderedTimestampSources([]memoryRow{{id: "typed", data: map[string]any{"at": instant}}}, q, func() any { return new(typed) })
	if err != nil || values["typed"]["at"] != instant {
		t.Fatalf("typed engine timestamp lost: %v %v", values, err)
	}
	values, err = orderedTimestampSources([]memoryRow{{id: "nil"}}, q, func() any { return nil })
	if err != nil || len(values) != 0 {
		t.Fatalf("nil schema factory invented typed data: %v %v", values, err)
	}
}

func TestTimestampOrderTraversesNilEmbeddedAndNamedMapDeclarations(t *testing.T) {
	whole := time.Date(2026, 10, 9, 12, 1, 0, 0, time.UTC)
	for _, variant := range []string{"query", "schema"} {
		t.Run(variant, func(t *testing.T) {
			var opts []Option
			if variant != "query" {
				var storage []CollectionOption
				opts = append(opts, WithSchema(false, WithCollection[timestampOrderEmbeddedRow]("events", nil, storage...)))
			}
			backend := newDatabase(opts...)
			ctx := context.Background()
			for id, instant := range map[string]time.Time{"whole": whole, "fraction": whole.Add(time.Millisecond)} {
				if err := backend.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("events", id), &timestampOrderEmbeddedRow{&TimestampOrderEmbedded{instant}})); err != nil {
					t.Fatal(err)
				}
			}
			q := dal.From(dal.NewRootCollectionRef("events", "")).NewQuery().OrderBy(dal.AscendingField("at")).Limit(1).SelectIntoRecord(func() record.Record {
				return record.NewRecordWithData(record.NewKeyWithID("events", ""), new(timestampOrderEmbeddedRow))
			})
			rows, err := dal.ExecuteQueryAndReadAllToRecords(ctx, q, dal.NewDB(backend))
			if err != nil || len(rows) != 1 || rows[0].Key().ID != "whole" {
				t.Fatalf("nil embedded declaration lost: %v %v", rows, err)
			}
		})
	}
	for _, value := range []any{map[timestampOrderMapKey]time.Time(nil), map[timestampOrderMapKey]time.Time{"at": whole}} {
		if _, ok := orderedTimestampField(value, "at").(time.Time); !ok {
			t.Fatalf("named map declaration lost: %T", value)
		}
	}
}

// Timestamp values and strings with identical bytes have different ordering
// contracts. In particular RFC3339Nano's omitted fractional part sorts after
// its nonzero fractional part as text, but before it as an instant.
func TestTimestampOrderUsesDeclaredInstantsBeforeLimit(t *testing.T) {
	type deadline struct {
		At   time.Time `json:"at"`
		Text string    `json:"text"`
	}
	whole := time.Date(2026, 10, 9, 12, 1, 0, 0, time.UTC)
	for _, variant := range []string{"query-type", "source-type", "columnar-source-type"} {
		registered := variant != "query-type"
		name := variant
		var options []Option
		if registered {
			var storage []CollectionOption
			if variant == "columnar-source-type" {
				storage = append(storage, WithColumnarStorage())
			}
			options = append(options, WithSchema(false, WithCollection[deadline]("deadlines", nil, storage...)))
		}
		t.Run(name, func(t *testing.T) {
			backend := newDatabase(options...)
			db := dal.NewDB(backend)
			ctx := context.Background()
			for id, at := range map[string]time.Time{"whole": whole, "fraction": whole.Add(100 * time.Millisecond)} {
				if err := backend.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("deadlines", id), &deadline{At: at, Text: at.Format(time.RFC3339Nano)})); err != nil {
					t.Fatal(err)
				}
			}
			for _, tc := range []struct {
				field, want string
				descending  bool
			}{
				{"at", "whole", false}, {"at", "fraction", true},
				{"text", "fraction", false}, {"text", "whole", true},
			} {
				var order = dal.AscendingField(tc.field)
				if tc.descending {
					order = dal.DescendingField(tc.field)
				}
				builder := dal.From(dal.NewRootCollectionRef("deadlines", "")).NewQuery().OrderBy(order).Limit(1)
				var query dal.Query
				if registered {
					query = builder.SelectIntoRecord(nil)
				} else {
					query = builder.SelectIntoRecord(func() record.Record {
						return record.NewRecordWithData(record.NewKeyWithID("deadlines", ""), new(deadline))
					})
				}
				rows, err := dal.ExecuteQueryAndReadAllToRecords(ctx, query, db)
				if err != nil {
					t.Fatal(err)
				}
				if len(rows) != 1 || rows[0].Key().ID != tc.want {
					t.Fatalf("field=%s descending=%v: got %v, want %s", tc.field, tc.descending, rows, tc.want)
				}
			}
		})
	}
}
