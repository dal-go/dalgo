package dal

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/dal-go/record"
)

// writtenRow is a provider row whose MarshalJSON writes another text under the
// name of its key than the time its field At holds.
type writtenRow struct {
	Group int
	At    time.Time `json:"at"`
	Value int
	Text  string
}

func (r writtenRow) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"g": r.Group, "at": r.Text, "v": r.Value})
}

// pointerWrittenRow writes itself through the method of its pointer type, which
// encoding/json calls for a row given by pointer and not for one given by value.
type pointerWrittenRow struct {
	Group int
	At    time.Time `json:"at"`
	Value int
	Text  string
}

func (r *pointerWrittenRow) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"g": r.Group, "at": r.Text, "v": r.Value})
}

// stampText writes itself as one text, and stampObject as an object of its own.
type (
	stampText   struct{ When time.Time }
	stampObject struct {
		When time.Time `json:"when"`
		Text string
	}
)

func (s stampText) MarshalText() ([]byte, error) { return []byte(s.When.Format(time.RFC3339Nano)), nil }

func (s stampObject) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"when": s.Text})
}

// nestedRow holds the struct that writes itself on the path of a nested key.
type nestedRow struct {
	Group int         `json:"g"`
	Meta  stampObject `json:"meta"`
	Value int         `json:"v"`
}

type writtenMap map[string]any

func (m writtenMap) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"g": m["g"], "at": m["text"], "v": m["v"]})
}

// A struct whose type writes itself is not read by its fields: a key in it has no sort
// value, so it is compared by the text the row writes.
func TestSortValuesOfAStructThatWritesItselfAreNotBuiltFromItsFields(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	byValue := writtenRow{At: at, Text: "x"}
	byPointer := pointerWrittenRow{At: at, Text: "x"}
	for name, tc := range map[string]struct {
		row  any
		key  string
		want map[string]any
	}{
		"MarshalJSON of the row, given by value":                      {byValue, "at", nil},
		"MarshalJSON of the row, given by pointer":                    {&byValue, "at", nil},
		"MarshalJSON of the pointer type, row given by pointer":       {&byPointer, "at", nil},
		"MarshalJSON of the pointer type, row given by value":         {byPointer, "at", map[string]any{"at": at.Format(timestampSortLayout)}},
		"MarshalJSON of a struct on the path of a nested key":         {nestedRow{Meta: stampObject{When: at}}, "meta.when", nil},
		"MarshalJSON of a struct on the path, row given by pointer":   {&nestedRow{Meta: stampObject{When: at}}, "meta.when", nil},
		"MarshalText of a struct on the path of a nested key":         {map[string]any{"stamp": stampText{When: at}}, "stamp.When", nil},
		"MarshalText of a struct on the path, held by pointer":        {map[string]any{"stamp": &stampText{When: at}}, "stamp.When", nil},
		"a struct that writes no method of its own is read as before": {map[string]any{"stamp": struct{ When time.Time }{at}}, "stamp.When", map[string]any{"stamp.When": at.Format(timestampSortLayout)}},
		"a time is still the value read":                              {map[string]any{"at": at}, "at", map[string]any{"at": at.Format(timestampSortLayout)}},
		"MarshalJSON of a named map":                                  {writtenMap{"at": at, "text": "x"}, "at", nil},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := buildSortValues(tc.row, []string{tc.key})
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("sort values = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}

// first and last follow the text a row that writes itself writes under the key's name,
// in both arrival orders, and not the time its field holds.
func TestOrderedAggregateComparesAKeyOfARowThatWritesItselfByTheTextItWrites(t *testing.T) {
	early := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	late := early.Add(48 * time.Hour)
	// The row of the earlier instant writes the later text.
	records := func(make func(group int, at time.Time, value int, text string) any) []record.Record {
		return []record.Record{
			record.NewRecordWithData(record.NewKeyWithID("T", "1"), make(1, early, 1, "z")),
			record.NewRecordWithData(record.NewKeyWithID("T", "2"), make(1, late, 2, "a")),
		}
	}
	reverse := func(rows []record.Record) []record.Record { return []record.Record{rows[1], rows[0]} }
	row := func(group int, at time.Time, value int, text string) any {
		return writtenRow{Group: group, At: at, Value: value, Text: text}
	}
	pointer := func(group int, at time.Time, value int, text string) any {
		return &pointerWrittenRow{Group: group, At: at, Value: value, Text: text}
	}
	nested := func(group int, at time.Time, value int, text string) any {
		return &nestedRow{Group: group, Meta: stampObject{When: at, Text: text}, Value: value}
	}
	mapRow := func(group int, at time.Time, value int, text string) any {
		return writtenMap{"g": group, "at": at, "v": value, "text": text}
	}
	for name, tc := range map[string]struct {
		rows []record.Record
		key  string
	}{
		"MarshalJSON of the row":              {records(row), "at"},
		"MarshalJSON of the pointer type":     {records(pointer), "at"},
		"MarshalJSON of a struct in the row":  {records(nested), "meta.when"},
		"the same, the rows in reverse order": {reverse(records(row)), "at"},
		"MarshalJSON of a named map":          {records(mapRow), "at"},
		"named map in reverse order":          {reverse(records(mapRow)), "at"},
	} {
		t.Run(name, func(t *testing.T) {
			q := From(NewRootCollectionRef("T", "")).NewQuery().GroupBy(Field("g")).SelectColumns(
				Column{Alias: "first", Expression: NewOrderedAggregate(FIRST, orderedBy(AscendingField(tc.key)), Field("v"))},
				Column{Alias: "last", Expression: NewOrderedAggregate(LAST, orderedBy(AscendingField(tc.key)), Field("v"))})
			reader, err := NewDB(&orderedStub{rows: map[string][]record.Record{"T": tc.rows}}).ExecuteQueryToRecordsReader(context.Background(), q)
			if err != nil {
				t.Fatal(err)
			}
			// "a" is written by the later instant and sorts before "z": it is first, and the earlier instant is last.
			if got := readOrderedRows(t, reader); !reflect.DeepEqual(got, []map[string]any{{"first": 2.0, "last": 1.0}}) {
				t.Fatalf("rows = %v", got)
			}
		})
	}

	// A row of a type whose pointer type writes itself is written by its fields when it is given by
	// value, and its timestamps are then compared by instant: the later one is written with a fraction,
	// which as text sorts before the earlier one.
	t.Run("a row given by value that only its pointer type writes is compared by instant", func(t *testing.T) {
		first := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		second := first.Add(500 * time.Millisecond)
		rows := []record.Record{
			record.NewRecordWithData(record.NewKeyWithID("T", "1"), pointerWrittenRow{Group: 1, At: first, Value: 1}),
			record.NewRecordWithData(record.NewKeyWithID("T", "2"), pointerWrittenRow{Group: 1, At: second, Value: 2}),
		}
		q := From(NewRootCollectionRef("T", "")).NewQuery().GroupBy(Field("Group")).SelectColumns(
			Column{Alias: "last", Expression: NewOrderedAggregate(LAST, orderedBy(AscendingField("at")), Field("Value"))})
		reader, err := NewDB(&orderedStub{rows: map[string][]record.Record{"T": rows}}).ExecuteQueryToRecordsReader(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		if got := readOrderedRows(t, reader); !reflect.DeepEqual(got, []map[string]any{{"last": 2.0}}) {
			t.Fatalf("rows = %v", got)
		}
	})
}
