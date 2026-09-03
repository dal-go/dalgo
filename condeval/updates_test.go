package condeval

import (
	"reflect"
	"testing"

	"github.com/dal-go/record/update"
)

func TestCloneMap(t *testing.T) {
	source := map[string]any{"a": 1.0, "nested": map[string]any{"b": []any{"x", map[string]any{"c": true}}}}
	clone := CloneMap(source)
	if !reflect.DeepEqual(clone, source) {
		t.Fatalf("clone = %v, want %v", clone, source)
	}
	clone["nested"].(map[string]any)["b"].([]any)[1].(map[string]any)["c"] = false
	if source["nested"].(map[string]any)["b"].([]any)[1].(map[string]any)["c"] != true {
		t.Error("clone must not share nested values with the source")
	}
}

func TestApplyUpdates(t *testing.T) {
	data := map[string]any{"name": "Ann", "address": map[string]any{"city": "Cork"}, "flag": "x"}
	err := ApplyUpdates(data, []update.Update{
		update.ByFieldName("name", "Bob"),
		update.ByFieldPath(update.FieldPath{"address", "city"}, "Limerick"),
		update.ByFieldPath(update.FieldPath{"meta", "created"}, 42),
		update.DeleteByFieldName("flag"),
	})
	if err != nil {
		t.Fatalf("ApplyUpdates: %v", err)
	}
	want := map[string]any{"name": "Bob", "address": map[string]any{"city": "Limerick"}, "meta": map[string]any{"created": 42.0}}
	if !reflect.DeepEqual(data, want) {
		t.Errorf("data = %v, want %v", data, want)
	}
	for name, bad := range map[string][]update.Update{
		"intermediate not a map": {update.ByFieldPath(update.FieldPath{"name", "first"}, "A")},
		"unserialisable value":   {update.ByFieldName("ch", make(chan int))},
		"empty update":           {emptyUpdate{}},
	} {
		if err := ApplyUpdates(CloneMap(data), bad); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

type emptyUpdate struct{}

func (emptyUpdate) FieldName() string           { return "" }
func (emptyUpdate) FieldPath() update.FieldPath { return nil }
func (emptyUpdate) Value() any                  { return nil }
