package condeval

import (
	"fmt"

	"github.com/dal-go/record/update"
)

// CloneMap deep-copies JSON-shaped record data (maps, slices and scalars) so a
// post-image can be computed without touching the pre-image.
func CloneMap(data map[string]any) map[string]any {
	out := make(map[string]any, len(data))
	for key, value := range data {
		out[key] = cloneValue(value)
	}
	return out
}

func cloneValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return CloneMap(v)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = cloneValue(item)
		}
		return out
	default:
		return v
	}
}

// ApplyUpdates applies field updates to JSON-shaped record data, producing the
// post-image of an Update: a field name addresses a top-level key, a field
// path a nested key, and update.DeleteField removes the leaf. Missing
// intermediate maps are created; an intermediate value that is not a map is an
// error, because the write would fail or corrupt the record and the policy
// must not guess the outcome.
func ApplyUpdates(data map[string]any, updates []update.Update) error {
	for _, item := range updates {
		var path []string
		if fieldPath := item.FieldPath(); len(fieldPath) > 0 {
			path = fieldPath
		} else if fieldName := item.FieldName(); fieldName != "" {
			path = []string{fieldName}
		} else {
			return fmt.Errorf("condeval: update has neither a field name nor a field path")
		}
		if err := applyPathUpdate(data, path, item.Value()); err != nil {
			return err
		}
	}
	return nil
}

func applyPathUpdate(data map[string]any, path []string, value any) error {
	current := data
	for _, key := range path[:len(path)-1] {
		switch next := current[key].(type) {
		case map[string]any:
			current = next
		case nil:
			created := make(map[string]any)
			current[key] = created
			current = created
		default:
			return fmt.Errorf("condeval: field path segment %q is not a map (got %T)", key, current[key])
		}
	}
	leaf := path[len(path)-1]
	if value == update.DeleteField {
		delete(current, leaf)
		return nil
	}
	normalized, ok := normalize(value)
	if !ok {
		return fmt.Errorf("condeval: update value for %q is not JSON-serialisable", leaf)
	}
	current[leaf] = normalized
	return nil
}
