package record

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"strings"
)

// DataToMap converts record data into a map[string]any keyed by field name.
//
// A map[string]any is returned unchanged. Any other value (typically a struct
// or a pointer to a struct) is converted via encoding/json, after which the
// top-level keys are remapped so a field's `db` tag takes precedence over its
// `json` tag. This lets file/document adapters that lack a native serializer
// (unlike the Firestore SDK or scany for SQL) reuse one consistent mapping.
//
// Tag precedence for a field key is: `db` tag, then `json` tag, then field name.
// `json:"-"` fields are omitted and `,omitempty` is honoured (both via the
// json round-trip). Nested struct fields are serialized by encoding/json and
// therefore follow their `json` tags.
func DataToMap(data any) (map[string]any, error) {
	if data == nil {
		return nil, nil
	}
	if m, ok := data.(map[string]any); ok {
		return m, nil
	}
	b, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("record: marshal data of type %T: %w", data, err)
	}
	m := map[string]any{}
	if err = json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("record: convert data of type %T to map: %w", data, err)
	}
	for _, r := range fieldTagRenames(reflect.TypeOf(data)) {
		if r.from == r.to {
			continue
		}
		if v, ok := m[r.from]; ok {
			m[r.to] = v
			delete(m, r.from)
		}
	}
	return m, nil
}

// MapToData populates target from src. If target is a map[string]any, src is
// copied into it. Otherwise target must be a pointer (typically to a struct):
// the `db`-keyed entries in src are remapped back to `json` keys and applied via
// encoding/json, so values, nested structs and type coercion are handled by the
// standard library. Tag precedence matches DataToMap: `db`, then `json`, then
// field name.
func MapToData(target any, src map[string]any) error {
	if m, ok := target.(map[string]any); ok {
		maps.Copy(m, src)
		return nil
	}
	remapped := maps.Clone(src)
	if remapped == nil {
		remapped = map[string]any{}
	}
	for _, r := range fieldTagRenames(reflect.TypeOf(target)) {
		if r.from == r.to {
			continue
		}
		if v, ok := remapped[r.to]; ok {
			remapped[r.from] = v
			delete(remapped, r.to)
		}
	}
	b, err := json.Marshal(remapped)
	if err != nil {
		return fmt.Errorf("record: marshal data map: %w", err)
	}
	if err = json.Unmarshal(b, target); err != nil {
		return fmt.Errorf("record: unmarshal data into %T: %w", target, err)
	}
	return nil
}

// tagRename records that the json key `from` should be exposed as the db key `to`.
type tagRename struct{ from, to string }

// fieldTagRenames walks the struct fields of t (dereferencing pointers) and
// returns the renames needed to make a field's `db` tag take precedence over its
// `json` tag. Anonymous (embedded) struct fields are recursed into so their
// fields participate too.
func fieldTagRenames(t reflect.Type) []tagRename {
	for t != nil && (t.Kind() == reflect.Pointer || t.Kind() == reflect.Interface) {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	var renames []tagRename
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous {
			renames = append(renames, fieldTagRenames(f.Type)...)
			continue
		}
		if f.PkgPath != "" { // unexported
			continue
		}
		jsonKey := tagName(f.Tag.Get("json"), f.Name)
		if jsonKey == "" { // json:"-"
			continue
		}
		dbKey := tagName(f.Tag.Get("db"), "")
		if dbKey == "" || dbKey == jsonKey {
			continue
		}
		renames = append(renames, tagRename{from: jsonKey, to: dbKey})
	}
	return renames
}

// tagName extracts the name portion of a struct tag value (the part before the
// first comma). A tag of "-" yields "" (skip). An empty tag yields fallback.
func tagName(tag, fallback string) string {
	if tag == "" {
		return fallback
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "-" {
		return ""
	}
	if name == "" {
		return fallback
	}
	return name
}
