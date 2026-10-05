package access

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	return sets.redactRecordRenaming(rec, nil)
}

// redactRecordRenaming is redactRecord for the row of a query whose columns the
// access layer renamed: in map data, the value under each generated key is kept
// whatever the list says of the key and is moved to the name the caller gave
// the column; every other top-level key is redacted by its own name.
func (sets fieldSets) redactRecordRenaming(rec record.Record, renames []outputRename) error {
	if !sets.restrictive() || !rec.Exists() {
		return nil
	}
	value := reflect.ValueOf(rec.Data())
	switch {
	case value.Kind() == reflect.Map:
		if data, ok := rec.Data().(map[string]any); ok {
			sets.redactMapRenaming(data, renames)
			return nil
		}
		return fmt.Errorf("access: cannot redact map data of type %T", rec.Data())
	case value.Kind() == reflect.Pointer && !value.IsNil():
		if data, ok := rec.Data().(*map[string]any); ok {
			sets.redactMapRenaming(*data, renames)
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

// redactMapRenaming is redactMap for a row some of whose columns were sent
// under generated aliases. Only a key the access layer generated for this query
// is exempt, and it is judged by what produced it, not by its name: it was given
// only to a column whose expression this session's field list allows (see
// aliasRefusedOutputs). Whatever else the session returns under any other key,
// a name the caller chose included, is redacted by that key.
func (sets fieldSets) redactMapRenaming(data map[string]any, renames []outputRename) {
	kept := make([]any, len(renames))
	found := make([]bool, len(renames))
	for i, rename := range renames {
		kept[i], found[i] = data[rename.generated]
		delete(data, rename.generated)
	}
	sets.redactMap("", data)
	for i, rename := range renames {
		if found[i] {
			data[rename.name] = kept[i]
		}
	}
}

// redactingReader applies field redaction to every record a query returns.
// renames are the columns the access layer sent under generated aliases (see
// aliasRefusedOutputs).
type redactingReader struct {
	dal.RecordsReader
	sets    fieldSets
	renames []outputRename
}

func (r redactingReader) Next() (record.Record, error) {
	rec, err := r.RecordsReader.Next()
	if err != nil || rec == nil {
		return rec, err
	}
	if err := r.sets.redactRecordRenaming(rec, r.renames); err != nil {
		return nil, err
	}
	return rec, nil
}

// outputRename records one column sent to the wrapped session under an alias the
// access layer generated: the key the column comes back under, and the name the
// caller gave it.
type outputRename struct{ generated, name string }

// outputName is the name a selected column comes back under, by DALgo's rule:
// the alias when there is one, otherwise the field's own name, otherwise the
// text of the expression. ok is false for a column that names no output: a
// wildcard, or a column with no expression.
func outputName(column dal.Column) (name string, ok bool) {
	if column.Wildcard != nil || column.Expression == nil {
		return "", false
	}
	if column.Alias != "" {
		return column.Alias, true
	}
	switch expression := column.Expression.(type) {
	case dal.FieldRef:
		return expression.Name(), true
	case *dal.FieldRef:
		return expression.Name(), true
	}
	return column.Expression.String(), true
}

// aliasRefusedOutputs sends every explicitly selected column whose expression
// the field list allows, and whose output name the list does not, under an alias
// of the access layer's own, and returns the query to send with the renames that
// undo it. The alias carries a random token drawn for this query, so neither the
// caller nor the wrapped session can know it. Every other column is sent as it
// is: one that comes back under a name the list allows, and one whose expression
// the list does not allow, which this session did not hold to the list (it can
// be a column an outer secured session's projection added) and which redaction
// therefore judges by its name. The alias exists so that redaction can tell the
// value of an allowed expression from a stored field of the same name: redaction
// keeps exactly the generated keys, and a session that returns more than the
// query projected, or none of it, brings back no generated key and so exempts
// nothing.
func aliasRefusedOutputs(query dal.StructuredQuery, sets fieldSets) (dal.StructuredQuery, []outputRename) {
	columns := query.Columns()
	var renames []outputRename
	var rewritten []dal.Column
	token := ""
	for i, column := range columns {
		if !sets.readsOnlyAllowedFields(column.Expression) {
			continue
		}
		name, named := outputName(column)
		if !named || sets.allowsWhole(name) {
			continue
		}
		if rewritten == nil {
			rewritten = append([]dal.Column(nil), columns...)
			token = outputToken()
		}
		generated := fmt.Sprintf("access_%s_%d", token, i)
		rewritten[i].Alias = generated
		renames = append(renames, outputRename{generated: generated, name: name})
	}
	if renames == nil {
		return query, nil
	}
	return dal.WithColumns(query, rewritten), renames
}

// outputToken returns 128 random bits as lower-case hexadecimal, so an alias
// built from it survives an engine that folds identifier case.
func outputToken() string {
	var raw [16]byte
	_, _ = rand.Read(raw[:]) // never fails: a failure of the system source ends the process
	return hex.EncodeToString(raw[:])
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
	if len(query.GroupBy()) > 0 {
		// A grouped query with no columns selects its group keys, so that is what
		// it is projected to, not the allowed fields: a column that is neither
		// aggregated nor a group key makes the aggregation invalid.
		columns := dal.EffectiveAggregationColumns(query)
		for _, column := range columns {
			if !sets.readsOnlyAllowedFields(column.Expression) {
				return queryProjection{query: query, status: queryProjectionUnavailable}
			}
		}
		return queryProjection{query: dal.WithColumns(query, columns), status: queryProjectionApplied}
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

// readsOnlyAllowedFields reports whether a selected column is a plain field
// (given by value or by non-nil pointer), or an aggregate whose operands name
// only fields, that every set allows.
func (sets fieldSets) readsOnlyAllowedFields(expression dal.Expression) bool {
	switch expression := expression.(type) {
	case dal.FieldRef:
		return sets.allowsWhole(expression.Name())
	case *dal.FieldRef:
		return expression != nil && sets.allowsWhole(expression.Name())
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
