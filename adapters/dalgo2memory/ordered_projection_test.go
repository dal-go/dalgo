package dalgo2memory

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

type orderedProjectionRecord struct {
	At    time.Time `json:"at"`
	Text  string    `json:"text"`
	Value int       `json:"value"`
}

type marshaledProjection struct {
	At time.Time `json:"at"`
}

func (m marshaledProjection) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"at": m.At.Format(time.RFC3339)})
}

type leftProjection struct {
	Stamp time.Time
}
type rightProjection struct {
	Stamp time.Time
}
type ambiguousProjection struct {
	leftProjection
	rightProjection
}

func TestOrderedProjectionRetainsAnInstantWithoutReinterpretingText(t *testing.T) {
	early := time.Date(2024, 1, 1, 1, 0, 0, 0, time.FixedZone("plus2", 2*3600))
	late := time.Date(2024, 1, 1, 0, 0, 0, 0, time.FixedZone("minus2", -2*3600))
	for _, columnar := range []bool{false, true} {
		for _, reversed := range []bool{false, true} {
			storage := []CollectionOption{}
			if columnar {
				storage = append(storage, WithColumnarStorage())
			}
			backend := newDatabase(WithSchema(false, WithCollection[orderedProjectionRecord]("events", nil, storage...)))
			rows := []orderedProjectionRecord{
				{At: late, Text: "2024-01-01T00:00:00-02:00", Value: 2},
				{At: early, Text: "2024-01-01T01:00:00+02:00", Value: 1},
			}
			if reversed {
				rows[0], rows[1] = rows[1], rows[0]
			}
			for i, row := range rows {
				if err := backend.Set(context.Background(), record.NewRecordWithData(record.NewKeyWithID("events", i), row)); err != nil {
					t.Fatal(err)
				}
			}
			q := dal.From(dal.NewRootCollectionRef("events", "")).NewQuery().SelectColumns(
				dal.Column{Alias: "by_instant", Expression: dal.NewOrderedAggregate(dal.FIRST, []dal.OrderExpression{dal.AscendingField("at")}, dal.Field("value"))},
				dal.Column{Alias: "by_text", Expression: dal.NewOrderedAggregate(dal.FIRST, []dal.OrderExpression{dal.AscendingField("text")}, dal.Field("value"))},
			)
			reader, err := dal.NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
			if err != nil {
				t.Fatal(err)
			}
			records, err := dal.ReadAllToRecords(context.Background(), reader)
			if err != nil {
				t.Fatal(err)
			}
			if len(records) != 1 {
				t.Fatalf("rows = %v", records)
			}
			got := records[0].Data().(map[string]any)
			if !reflect.DeepEqual(got, map[string]any{"by_instant": float64(1), "by_text": float64(2)}) {
				t.Fatalf("columnar=%v reversed=%v: rows = %#v", columnar, reversed, got)
			}
		}
	}
}

func TestOrdinaryProjectionKeepsItsJSONShape(t *testing.T) {
	instant := time.Date(2024, 1, 1, 1, 0, 0, 0, time.FixedZone("plus2", 2*3600))
	for _, columnar := range []bool{false, true} {
		storage := []CollectionOption{}
		if columnar {
			storage = append(storage, WithColumnarStorage())
		}
		backend := newDatabase(WithSchema(false, WithCollection[orderedProjectionRecord]("events", nil, storage...)))
		if err := backend.Set(context.Background(), record.NewRecordWithData(record.NewKeyWithID("events", 1), orderedProjectionRecord{At: instant, Text: "literal"})); err != nil {
			t.Fatal(err)
		}
		q := dal.From(dal.NewRootCollectionRef("events", "")).NewQuery().SelectColumns(
			dal.Column{Expression: dal.Field("at")}, dal.Column{Expression: dal.Field("text")})
		reader, err := dal.NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := dal.ReadAllToRecords(context.Background(), reader)
		if err != nil || len(rows) != 1 {
			t.Fatalf("rows = %v, err = %v", rows, err)
		}
		encoded, err := json.Marshal(rows[0].Data())
		if err != nil || string(encoded) != `{"at":"2024-01-01T01:00:00+02:00","text":"literal"}` {
			t.Fatalf("columnar=%v ordinary bytes = %s, err = %v", columnar, encoded, err)
		}
	}
}

func TestOrderedProjectionRetainsNestedDeclaredTimestamp(t *testing.T) {
	type detail struct {
		Stamp *time.Time `json:"stamp"`
	}
	type event struct {
		Detail *detail `json:"detail"`
		Value  int     `json:"value"`
	}
	early := time.Date(2024, 1, 1, 1, 0, 0, 0, time.FixedZone("plus2", 2*3600))
	late := time.Date(2024, 1, 1, 0, 0, 0, 0, time.FixedZone("minus2", -2*3600))
	for _, columnar := range []bool{false, true} {
		storage := []CollectionOption{}
		if columnar {
			storage = append(storage, WithColumnarStorage())
		}
		backend := newDatabase(WithSchema(false, WithCollection[event]("events", nil, storage...)))
		for i, item := range []event{{Detail: &detail{Stamp: &late}, Value: 1}, {Detail: &detail{Stamp: &early}, Value: 2}} {
			if err := backend.Set(context.Background(), record.NewRecordWithData(record.NewKeyWithID("events", i), item)); err != nil {
				t.Fatal(err)
			}
		}
		q := dal.From(dal.NewRootCollectionRef("events", "")).NewQuery().SelectColumns(
			dal.Column{Alias: "first", Expression: dal.NewOrderedAggregate(dal.FIRST, []dal.OrderExpression{dal.AscendingField("detail.stamp")}, dal.Field("value"))})
		reader, err := dal.NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := dal.ReadAllToRecords(context.Background(), reader)
		if err != nil || len(rows) != 1 {
			t.Fatalf("rows = %v, err = %v", rows, err)
		}
		if got := rows[0].Data().(map[string]any)["first"]; got != float64(2) {
			t.Fatalf("columnar=%v nested first = %v", columnar, got)
		}
	}
}

func TestOrderedProjectionKeepsCustomJSONAndAmbiguousFieldsAsWritten(t *testing.T) {
	early := time.Date(2024, 1, 1, 1, 0, 0, 0, time.FixedZone("plus2", 2*3600))
	late := time.Date(2024, 1, 1, 0, 0, 0, 0, time.FixedZone("minus2", -2*3600))
	type customEvent struct {
		Custom marshaledProjection `json:"custom"`
		Value  int                 `json:"value"`
	}
	type ambiguousEvent struct {
		ambiguousProjection
		Value int `json:"value"`
	}
	for _, columnar := range []bool{false, true} {
		storage := []CollectionOption{}
		if columnar {
			storage = append(storage, WithColumnarStorage())
		}
		custom := newDatabase(WithSchema(false, WithCollection[customEvent]("events", nil, storage...)))
		ambiguous := newDatabase(WithSchema(false, WithCollection[ambiguousEvent]("events", nil, storage...)))
		for i, item := range []struct {
			at    time.Time
			value int
		}{{late, 1}, {early, 2}} {
			key := record.NewKeyWithID("events", i)
			if err := custom.Set(context.Background(), record.NewRecordWithData(key, customEvent{Custom: marshaledProjection{At: item.at}, Value: item.value})); err != nil {
				t.Fatal(err)
			}
			if err := ambiguous.Set(context.Background(), record.NewRecordWithData(key, ambiguousEvent{ambiguousProjection: ambiguousProjection{leftProjection{item.at}, rightProjection{item.at}}, Value: item.value})); err != nil {
				t.Fatal(err)
			}
		}
		for name, tc := range map[string]struct {
			backend *database
			key     string
		}{
			"custom JSON":    {custom, "custom.at"},
			"ambiguous JSON": {ambiguous, "Stamp"},
		} {
			q := dal.From(dal.NewRootCollectionRef("events", "")).NewQuery().SelectColumns(
				dal.Column{Alias: "first", Expression: dal.NewOrderedAggregate(dal.FIRST, []dal.OrderExpression{dal.AscendingField(tc.key)}, dal.Field("value"))})
			reader, err := dal.NewDB(tc.backend).ExecuteQueryToRecordsReader(context.Background(), q)
			if err != nil {
				t.Fatal(err)
			}
			rows, err := dal.ReadAllToRecords(context.Background(), reader)
			if err != nil || len(rows) != 1 {
				t.Fatalf("%s rows = %v, err = %v", name, rows, err)
			}
			if got := rows[0].Data().(map[string]any)["first"]; got != float64(1) {
				t.Fatalf("columnar=%v %s first = %v; want serialized value/tie order", columnar, name, got)
			}
		}
	}
}

func TestOrderedProjectionRefusesTypedMaterializationFailure(t *testing.T) {
	backend := newDatabase(WithSchema(false, WithCollection[orderedProjectionRecord]("events", nil)))
	key := record.NewKeyWithID("events", 1)
	if err := backend.Set(context.Background(), record.NewRecordWithData(key, orderedProjectionRecord{At: time.Now()})); err != nil {
		t.Fatal(err)
	}
	backend.collections["events"].(*serializedEngine).records["1"] = []byte(`{"at":"not-a-timestamp","value":1}`)
	q := dal.From(dal.NewRootCollectionRef("events", "")).NewQuery().SelectColumns(
		dal.Column{Alias: "first", Expression: dal.NewOrderedAggregate(dal.FIRST, []dal.OrderExpression{dal.AscendingField("at")}, dal.Field("value"))})
	if _, err := dal.NewDB(backend).ExecuteQueryToRecordsReader(context.Background(), q); err == nil || !strings.Contains(err.Error(), "cannot parse") {
		t.Fatalf("ordered source materialization = %v", err)
	}
}

func TestOrderedTimestampPromotionLeavesOrdinaryFieldsUntouched(t *testing.T) {
	instant := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	projected := map[string]any{"at": "serialized", "text": "literal"}
	row := memoryRow{materialize: func(target any) error {
		*target.(*orderedProjectionRecord) = orderedProjectionRecord{At: instant, Text: "literal"}
		return nil
	}}
	values := func(raw any) map[string]any {
		return map[string]any{"at": raw.(*orderedProjectionRecord).At}
	}
	if err := promoteOrderedSourceTimestamps(projected, row, func() any { return &orderedProjectionRecord{} }, values); err != nil {
		t.Fatal(err)
	}
	if got, ok := projected["at"].(time.Time); !ok || !got.Equal(instant) || projected["text"] != "literal" {
		t.Fatalf("promoted projection = %#v", projected)
	}
	for _, factory := range []func() any{nil, func() any { return nil }} {
		if err := promoteOrderedSourceTimestamps(projected, row, factory, values); err != nil {
			t.Fatal(err)
		}
	}
	refusal := errors.New("cannot materialize")
	broken := memoryRow{materialize: func(any) error { return refusal }}
	if err := promoteOrderedSourceTimestamps(projected, broken, func() any { return &orderedProjectionRecord{} }, values); !errors.Is(err, refusal) {
		t.Fatalf("materialization error = %v", err)
	}
}
