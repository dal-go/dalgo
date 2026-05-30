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

type embeddedInnerForMap struct {
	Inner string `json:"inner" db:"inner_col"`
}

type embeddedOuterForMap struct {
	embeddedInnerForMap
	Outer string `json:"outer"`
}

type commaJSONTag struct {
	Name string `json:",omitempty"` // empty name → key falls back to field name
}

func TestDataToMap_NilMarshalAndShape(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		m, err := DataToMap(nil)
		if err != nil || m != nil {
			t.Fatalf("DataToMap(nil) = %v, %v; want nil, nil", m, err)
		}
	})
	t.Run("marshal_error", func(t *testing.T) {
		if _, err := DataToMap(make(chan int)); err == nil {
			t.Fatal("want marshal error for a chan value")
		}
	})
	t.Run("not_a_json_object", func(t *testing.T) {
		if _, err := DataToMap([]int{1, 2}); err == nil {
			t.Fatal("want error converting a non-object to map[string]any")
		}
	})
}

func TestMapToData_Errors(t *testing.T) {
	t.Run("nil_src", func(t *testing.T) {
		var got embeddedOuterForMap
		if err := MapToData(&got, nil); err != nil {
			t.Fatalf("MapToData(&struct, nil) = %v; want nil", err)
		}
	})
	t.Run("marshal_error", func(t *testing.T) {
		err := MapToData(&struct{ X int }{}, map[string]any{"x": make(chan int)})
		if err == nil {
			t.Fatal("want marshal error for a chan value in src")
		}
	})
	t.Run("unmarshal_error_non_struct_target", func(t *testing.T) {
		// *int is not a struct: fieldTagRenames returns nil and unmarshalling a
		// JSON object into *int fails.
		if err := MapToData(new(int), map[string]any{"x": 1}); err == nil {
			t.Fatal("want unmarshal error for object into *int")
		}
	})
}

func TestDataToMap_EmbeddedAndCommaTag(t *testing.T) {
	t.Run("anonymous_embedded_db_tag", func(t *testing.T) {
		m, err := DataToMap(embeddedOuterForMap{embeddedInnerForMap{Inner: "i"}, "o"})
		if err != nil {
			t.Fatal(err)
		}
		if m["inner_col"] != "i" {
			t.Errorf("embedded field db tag not applied: %v", m)
		}
		if m["outer"] != "o" {
			t.Errorf("outer: %v", m)
		}
	})
	t.Run("comma_only_json_tag_falls_back_to_field_name", func(t *testing.T) {
		m, err := DataToMap(commaJSONTag{Name: "n"})
		if err != nil {
			t.Fatal(err)
		}
		if m["Name"] != "n" {
			t.Errorf("comma-only json tag should fall back to field name: %v", m)
		}
	})
}
