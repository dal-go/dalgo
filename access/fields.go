package access

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/dal-go/dalgo/condeval"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

// fieldPattern is one parsed entry of a rule's fields allow-list: dotted
// segments, each a literal, a lone "*" (any one segment) or a glob with one
// "*" at the start or end ("public_*", "*_id"); a trailing ".*" matches the
// whole subtree below the preceding path.
type fieldPattern struct {
	source   string
	segments []string
	subtree  bool
}

// fieldSet is a rule's allow-list. A nil *fieldSet means every field.
type fieldSet struct {
	patterns []fieldPattern
	sources  []string
}

func parseFieldPatterns(sources []string) (*fieldSet, error) {
	if len(sources) == 0 {
		return nil, fmt.Errorf("access: a fields list must name at least one pattern")
	}
	set := &fieldSet{}
	for _, source := range sources {
		text := strings.TrimSpace(source)
		if text == "" {
			return nil, fmt.Errorf("access: fields contains an empty pattern")
		}
		pattern := fieldPattern{source: text}
		if strings.HasSuffix(text, ".*") {
			pattern.subtree = true
			text = strings.TrimSuffix(text, ".*")
		}
		for _, segment := range strings.Split(text, ".") {
			if segment == "" {
				return nil, fmt.Errorf("access: fields pattern %q has an empty segment", source)
			}
			if stars := strings.Count(segment, "*"); stars > 1 || (stars == 1 && segment != "*" && !strings.HasPrefix(segment, "*") && !strings.HasSuffix(segment, "*")) {
				return nil, fmt.Errorf("access: fields pattern %q: a segment may be *, a prefix* or a *suffix", source)
			}
			pattern.segments = append(pattern.segments, segment)
		}
		set.patterns = append(set.patterns, pattern)
		set.sources = append(set.sources, pattern.source)
	}
	return set, nil
}

func segmentMatches(pattern, segment string) bool {
	switch {
	case pattern == "*":
		return true
	case strings.HasPrefix(pattern, "*"):
		return strings.HasSuffix(segment, pattern[1:])
	case strings.HasSuffix(pattern, "*"):
		return strings.HasPrefix(segment, pattern[:len(pattern)-1])
	default:
		return pattern == segment
	}
}

// allows reports whether a dotted field path is readable or writable under
// the set: a pattern matches the path exactly, matches a prefix of it (an
// allowed parent covers its children), or is a subtree pattern over a prefix.
func (s *fieldSet) allows(path string) bool {
	if s == nil {
		return true
	}
	segments := strings.Split(path, ".")
	for _, pattern := range s.patterns {
		if len(pattern.segments) > len(segments) {
			continue
		}
		matched := true
		for i, want := range pattern.segments {
			if !segmentMatches(want, segments[i]) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// enumerable reports the top-level field names the set can be projected to
// when no pattern uses a wildcard in its first segment; otherwise ok is false
// and callers must fall back to redaction.
func (s *fieldSet) enumerable() (names []string, ok bool) {
	if s == nil {
		return nil, false
	}
	seen := map[string]struct{}{}
	for _, pattern := range s.patterns {
		first := pattern.segments[0]
		if strings.Contains(first, "*") {
			return nil, false
		}
		seen[first] = struct{}{}
	}
	names = make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, true
}

// fieldSets is the intersection of the allow-lists every applicable policy
// imposes on one resource; a field is allowed only when every set allows it.
type fieldSets []*fieldSet

func (sets fieldSets) allows(path string) bool {
	for _, set := range sets {
		if !set.allows(path) {
			return false
		}
	}
	return true
}

func (sets fieldSets) restrictive() bool {
	for _, set := range sets {
		if set != nil {
			return true
		}
	}
	return false
}

func (sets fieldSets) sources() string {
	var all []string
	for _, set := range sets {
		if set != nil {
			all = append(all, "["+strings.Join(set.sources, ", ")+"]")
		}
	}
	return strings.Join(all, " ∩ ")
}

// enumerable intersects the projectable top-level names of every set; ok is
// false when any restrictive set cannot be enumerated.
func (sets fieldSets) enumerable() ([]string, bool) {
	var names []string
	first := true
	for _, set := range sets {
		if set == nil {
			continue
		}
		own, ok := set.enumerable()
		if !ok {
			return nil, false
		}
		if first {
			names, first = own, false
			continue
		}
		keep := names[:0]
		for _, name := range names {
			for _, candidate := range own {
				if name == candidate {
					keep = append(keep, name)
					break
				}
			}
		}
		names = keep
	}
	return names, !first
}

// disallowedPaths lists the leaf paths of JSON-shaped data the sets refuse.
func (sets fieldSets) disallowedPaths(data map[string]any) []string {
	var refused []string
	var walk func(prefix string, value any)
	walk = func(prefix string, value any) {
		if nested, ok := value.(map[string]any); ok && len(nested) > 0 {
			for key, child := range nested {
				path := key
				if prefix != "" {
					path = prefix + "." + key
				}
				walk(path, child)
			}
			return
		}
		if !sets.allows(prefix) {
			refused = append(refused, prefix)
		}
	}
	for key, value := range data {
		walk(key, value)
	}
	sort.Strings(refused)
	return refused
}

// disallowedUpdates lists the update paths the sets refuse.
func (sets fieldSets) disallowedUpdates(updates []update.Update) []string {
	var refused []string
	for _, item := range updates {
		path := item.FieldName()
		if fieldPath := item.FieldPath(); len(fieldPath) > 0 {
			path = strings.Join(fieldPath, ".")
		}
		if !sets.allows(path) {
			refused = append(refused, path)
		}
	}
	sort.Strings(refused)
	return refused
}

// redactMap removes every leaf the sets refuse, in place.
func (sets fieldSets) redactMap(prefix string, data map[string]any) {
	for key, value := range data {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		if nested, ok := value.(map[string]any); ok && len(nested) > 0 {
			sets.redactMap(path, nested)
			if len(nested) == 0 {
				delete(data, key)
			}
			continue
		}
		if !sets.allows(path) {
			delete(data, key)
		}
	}
}

// redactRecord removes the refused fields from a loaded record: map data is
// pruned in place; pointer data is round-tripped through JSON so nested
// refusals apply, then written back into the zeroed target.
func (sets fieldSets) redactRecord(rec record.Record) error {
	if !sets.restrictive() || !rec.Exists() {
		return nil
	}
	value := reflect.ValueOf(rec.Data())
	switch {
	case value.Kind() == reflect.Map:
		if data, ok := rec.Data().(map[string]any); ok {
			sets.redactMap("", data)
			return nil
		}
		return fmt.Errorf("access: cannot redact map data of type %T", rec.Data())
	case value.Kind() == reflect.Pointer && !value.IsNil():
		if data, ok := rec.Data().(*map[string]any); ok {
			sets.redactMap("", *data)
			return nil
		}
		data, err := condeval.ToMap(rec.Data())
		if err != nil {
			return err
		}
		sets.redactMap("", data)
		encoded, _ := json.Marshal(data) // JSON-shaped data always marshals
		value.Elem().Set(reflect.Zero(value.Elem().Type()))
		return json.Unmarshal(encoded, rec.Data())
	default:
		return nil
	}
}

// redactingReader applies field redaction to every record a query returns.
type redactingReader struct {
	dal.RecordsReader
	sets fieldSets
}

func (r redactingReader) Next() (record.Record, error) {
	rec, err := r.RecordsReader.Next()
	if err != nil || rec == nil {
		return rec, err
	}
	if err := r.sets.redactRecord(rec); err != nil {
		return nil, err
	}
	return rec, nil
}

// projectQuery narrows a structured query's columns to the enumerable
// intersection of the allowed fields, keeping any columns the caller already
// selected that are allowed. It returns ok=false when the sets are
// restrictive but cannot be enumerated, so the caller must redact instead.
func projectQuery(query dal.StructuredQuery, sets fieldSets) (dal.StructuredQuery, bool) {
	if !sets.restrictive() {
		return query, true
	}
	allowed, ok := sets.enumerable()
	if !ok {
		return query, false
	}
	var columns []dal.Column
	if selected := query.Columns(); len(selected) > 0 {
		for _, column := range selected {
			field, isField := column.Expression.(dal.FieldRef)
			if !isField {
				return query, false
			}
			if sets.allows(field.Name()) {
				columns = append(columns, column)
			}
		}
	} else {
		for _, name := range allowed {
			columns = append(columns, dal.Column{Expression: dal.Field(name)})
		}
	}
	return dal.WithColumns(query, columns), true
}

var _ = context.Background
