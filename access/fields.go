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
	mask     *CompiledMask
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
	if s.mask != nil {
		return s.mask.Allows(path)
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
	if s == nil || s.mask != nil {
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

type queryProjectionStatus uint8

const (
	// queryProjectionApplied has an explicit, safe column list.
	queryProjectionApplied queryProjectionStatus = iota
	// queryProjectionUnavailable preserves the records-reader redaction fallback.
	queryProjectionUnavailable
	// queryProjectionEmpty would be interpreted as SELECT * by DALgo and is denied.
	queryProjectionEmpty
)

type queryProjection struct {
	query  dal.StructuredQuery
	status queryProjectionStatus
}

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
		if !sets.allowsValue(prefix, value) {
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
		if !sets.allowsValue(path, value) {
			delete(data, key)
		}
	}
}

// redactRecord removes the refused fields from a loaded record: map data is
// pruned in place; pointer data is round-tripped through JSON so nested
// refusals apply, then written back into the zeroed target.
func (sets fieldSets) redactRecord(rec record.Record) error {
	return sets.redactRecordKeeping(rec, nil)
}

// redactRecordKeeping is redactRecord for the row of a query whose columns the
// caller named: the top-level keys in keep are the names those columns come
// back under, and each was already checked against the field list by its
// source expression, so they stay whatever the list says of the name itself.
func (sets fieldSets) redactRecordKeeping(rec record.Record, keep []string) error {
	if !sets.restrictive() || !rec.Exists() {
		return nil
	}
	value := reflect.ValueOf(rec.Data())
	switch {
	case value.Kind() == reflect.Map:
		if data, ok := rec.Data().(map[string]any); ok {
			sets.redactMapKeeping(data, keep)
			return nil
		}
		return fmt.Errorf("access: cannot redact map data of type %T", rec.Data())
	case value.Kind() == reflect.Pointer && !value.IsNil():
		if data, ok := rec.Data().(*map[string]any); ok {
			sets.redactMapKeeping(*data, keep)
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

// redactMapKeeping is redactMap for a row whose top-level keys in keep are
// exempt from redaction.
func (sets fieldSets) redactMapKeeping(data map[string]any, keep []string) {
	kept := make(map[string]any, len(keep))
	for _, name := range keep {
		if value, ok := data[name]; ok {
			kept[name] = value
			delete(data, name)
		}
	}
	sets.redactMap("", data)
	for name, value := range kept {
		data[name] = value
	}
}

// redactingReader applies field redaction to every record a query returns.
// outputs are the names the query's explicitly selected columns come back
// under (see outputNames).
type redactingReader struct {
	dal.RecordsReader
	sets    fieldSets
	outputs []string
}

func (r redactingReader) Next() (record.Record, error) {
	rec, err := r.RecordsReader.Next()
	if err != nil || rec == nil {
		return rec, err
	}
	if err := r.sets.redactRecordKeeping(rec, r.outputs); err != nil {
		return nil, err
	}
	return rec, nil
}

// outputNames lists the names the explicitly selected columns of a query come
// back under, by DALgo's rule: the alias when there is one, otherwise the
// field's own name, otherwise the text of the expression. A field under its own
// name needs no entry; it is allowed by the list that allowed the column. A
// wildcard names no output of its own.
func outputNames(query dal.StructuredQuery) []string {
	if query == nil {
		return nil
	}
	var names []string
	for _, column := range query.Columns() {
		if column.Wildcard != nil || column.Expression == nil {
			continue
		}
		if column.Alias != "" {
			names = append(names, column.Alias)
			continue
		}
		if _, plain := column.Expression.(dal.FieldRef); !plain {
			names = append(names, column.Expression.String())
		}
	}
	return names
}

// projectQuery narrows a structured query's columns to the enumerable
// intersection of the allowed fields, keeping any columns the caller already
// selected that are allowed. It returns ok=false when the sets are
// restrictive but cannot be enumerated, so the caller must redact instead.
// projectQuery rewrites an enumerable restrictive policy into explicit columns.
// Its status distinguishes a redaction-safe unavailable fallback from an empty
// projection that must be denied before DALgo could interpret it as SELECT *.
func projectQuery(query dal.StructuredQuery, sets fieldSets) queryProjection {
	if !sets.restrictive() {
		return queryProjection{query: query, status: queryProjectionApplied}
	}
	if selected := query.Columns(); len(selected) > 0 {
		hasWildcard := false
		for _, column := range selected {
			if column.Wildcard != nil {
				hasWildcard = true
				continue
			}
			if !sets.readsOnlyAllowedFields(column.Expression) {
				return queryProjection{query: query, status: queryProjectionUnavailable}
			}
		}
		if hasWildcard {
			allowed, ok := sets.enumerable()
			if !ok {
				return queryProjection{query: query, status: queryProjectionUnavailable}
			}
			columns := make([]dal.Column, 0, len(allowed)+len(selected)-1)
			for _, column := range selected {
				if column.Wildcard == nil {
					columns = append(columns, column)
					continue
				}
				for _, name := range allowed {
					if !column.Wildcard.Excludes(name) {
						columns = append(columns, dal.Column{Expression: dal.Field(name)})
					}
				}
			}
			if len(columns) == 0 {
				return queryProjection{query: query, status: queryProjectionEmpty}
			}
			return queryProjection{query: dal.WithColumns(query, columns), status: queryProjectionApplied}
		}
		return queryProjection{query: query, status: queryProjectionApplied}
	}
	allowed, ok := sets.enumerable()
	if !ok {
		return queryProjection{query: query, status: queryProjectionUnavailable}
	}
	columns := make([]dal.Column, 0, len(allowed))
	for _, name := range allowed {
		columns = append(columns, dal.Column{Expression: dal.Field(name)})
	}
	if len(columns) == 0 {
		return queryProjection{query: query, status: queryProjectionEmpty}
	}
	return queryProjection{query: dal.WithColumns(query, columns), status: queryProjectionApplied}
}

var _ = context.Background

// readsOnlyAllowedFields reports whether a selected column is a plain field, or
// an aggregate whose operands name only fields, that every set allows.
func (sets fieldSets) readsOnlyAllowedFields(expression dal.Expression) bool {
	switch expression := expression.(type) {
	case dal.FieldRef:
		return sets.allowsWhole(expression.Name())
	case dal.AggregateFunc:
		fields, checkable := aggregateFields(expression, 0)
		for _, name := range fields {
			checkable = checkable && sets.allowsWhole(name)
		}
		return checkable
	}
	return false
}

// allowsWhole prevents a whole-object query probe from observing masked leaves.
// Masks may conservatively reject an object when subtree coverage is uncertain.
func (sets fieldSets) allowsWhole(path string) bool {
	for _, set := range sets {
		if set != nil && set.mask != nil {
			if !set.mask.CompleteSubtree(path) {
				return false
			}
		} else if !set.allows(path) {
			return false
		}
	}
	return true
}
func (sets fieldSets) allowsValue(path string, value any) bool {
	v := reflect.ValueOf(value)
	if v.IsValid() && (v.Kind() == reflect.Array || v.Kind() == reflect.Slice || v.Kind() == reflect.Map || v.Kind() == reflect.Struct || v.Kind() == reflect.Pointer) {
		return sets.allowsWhole(path)
	}
	return sets.allows(path)
}

// Masked parent mutations must authorize all affected descendants, including
// removed pre-image fields. A leaf update does not acquire an ancestor veto.
func (sets fieldSets) disallowedMaskedMutation(images writeImages, operation Operations) (refused []string, unsupported bool) {
	masked := false
	for _, set := range sets {
		if set != nil && set.mask != nil {
			masked = true
		}
	}
	if !masked {
		return nil, false
	}
	var walk func(string, any)
	walk = func(path string, value any) {
		nested, knownObject := value.(map[string]any)
		allowed := sets.allowsValue(path, value)
		if knownObject {
			allowed = sets.allows(path)
		}
		if !allowed {
			refused = append(refused, path)
			kind := reflect.ValueOf(value)
			if !knownObject && kind.IsValid() && (kind.Kind() == reflect.Array || kind.Kind() == reflect.Slice || kind.Kind() == reflect.Map || kind.Kind() == reflect.Struct || kind.Kind() == reflect.Pointer) {
				unsupported = true
			}
		}
		if knownObject {
			for key, child := range nested {
				walk(path+"."+key, child)
			}
		}
	}
	if operation == Update {
		for _, item := range images.updates {
			path := item.FieldName()
			if len(item.FieldPath()) > 0 {
				path = strings.Join(item.FieldPath(), ".")
			}
			for _, data := range []map[string]any{images.pre, images.post} {
				var value any = data
				found := true
				for _, segment := range strings.Split(path, ".") {
					m, ok := value.(map[string]any)
					if !ok {
						found = false
						break
					}
					value, ok = m[segment]
					if !ok {
						found = false
						break
					}
				}
				if found {
					walk(path, value)
				}
			}
		}
	} else {
		for _, data := range []map[string]any{images.pre, images.post} {
			for path, value := range data {
				walk(path, value)
			}
		}
	}
	sort.Strings(refused)
	return refused, unsupported
}
