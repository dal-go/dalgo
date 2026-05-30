package record

import (
	"reflect"
	"testing"
)

type dataMapTestData struct {
	StringProp  string `json:"StringProp,omitempty" db:"StringProp"`
	IntegerProp int    `json:"IntegerProp" db:"IntegerProp"`
}

type dataMapTagged struct {
	Name    string `json:"name" db:"full_name"`
	Age     int    `json:"age"`
	Ignored string `json:"-"`
	hidden  string //nolint:unused
}

func TestDataToMap(t *testing.T) {
	t.Run("map_passthrough", func(t *testing.T) {
		in := map[string]any{"a": 1}
		got, err := DataToMap(in)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, in) {
			t.Errorf("got %v, want %v", got, in)
		}
	})

	t.Run("struct_json_tags", func(t *testing.T) {
		got, err := DataToMap(&dataMapTestData{StringProp: "str1", IntegerProp: 1})
		if err != nil {
			t.Fatal(err)
		}
		if got["StringProp"] != "str1" {
			t.Errorf("StringProp: got %v", got["StringProp"])
		}
		if got["IntegerProp"].(float64) != 1 {
			t.Errorf("IntegerProp: got %v", got["IntegerProp"])
		}
	})

	t.Run("db_tag_takes_precedence", func(t *testing.T) {
		got, err := DataToMap(dataMapTagged{Name: "Alice", Age: 30, Ignored: "x"})
		if err != nil {
			t.Fatal(err)
		}
		if got["full_name"] != "Alice" {
			t.Errorf("expected key 'full_name'=Alice, got map %v", got)
		}
		if _, ok := got["name"]; ok {
			t.Errorf("json key 'name' should have been renamed to db key 'full_name': %v", got)
		}
		if _, ok := got["Ignored"]; ok {
			t.Error("json:\"-\" field must be omitted")
		}
	})
}

func TestMapToData_RoundTrip(t *testing.T) {
	t.Run("struct", func(t *testing.T) {
		src := dataMapTagged{Name: "Bob", Age: 25}
		m, err := DataToMap(src)
		if err != nil {
			t.Fatal(err)
		}
		var got dataMapTagged
		if err := MapToData(&got, m); err != nil {
			t.Fatal(err)
		}
		if got.Name != "Bob" || got.Age != 25 {
			t.Errorf("round-trip mismatch: got %+v, want %+v", got, src)
		}
	})

	t.Run("map_target", func(t *testing.T) {
		target := map[string]any{}
		if err := MapToData(target, map[string]any{"k": "v"}); err != nil {
			t.Fatal(err)
		}
		if target["k"] != "v" {
			t.Errorf("got %v", target)
		}
	})
}
