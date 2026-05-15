// specscore: feat-recordops/diff
package recordops

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/dal-go/dalgo/dal"
)

// compareRecords returns the per-field deltas between baseline's Data() and
// candidate's Data(). The returned slice is sorted by Name ascending. A
// candidate field that is absent from the candidate's record (but present in
// baseline) is encoded as FieldValue{Name, Absent: true} — structurally
// distinct from FieldValue{Name, Value: nil, Absent: false} (a real Go-nil
// value the candidate explicitly holds).
//
// On panic during reflect.DeepEqual (e.g., a func/chan field), returns
// (nil, err wrapping ErrIncomparableField).
func compareRecords(baseID any, baseRec, candRec dal.Record, cfg options) (deltas []FieldValue, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recordops: incomparable field in record %v: %v: %w", baseID, r, ErrIncomparableField)
			deltas = nil
		}
	}()

	bv := deref(reflect.ValueOf(baseRec.Data()))
	cv := deref(reflect.ValueOf(candRec.Data()))

	bk, ck := bucket(bv), bucket(cv)
	if bk != ck {
		return []FieldValue{{Name: "_value", Value: candRec.Data()}}, nil
	}

	switch bk {
	case bucketStruct:
		return compareStruct(bv, cv, cfg), nil
	case bucketMap:
		return compareMap(bv, cv, cfg), nil
	default:
		if !reflect.DeepEqual(baseRec.Data(), candRec.Data()) {
			return []FieldValue{{Name: "_value", Value: candRec.Data()}}, nil
		}
		return nil, nil
	}
}

// baselineFields extracts baseline's field values for the snapshot. By
// default, returns every field. With cfg.onlyChangedFields, returns only
// fields that have a delta on at least one candidate; returns nil if no
// candidate has any delta.
func baselineFields(baseRec dal.Record, perCandidateDeltas [][]FieldValue, cfg options) []FieldValue {
	all := extractAllFields(baseRec)
	if !cfg.onlyChangedFields {
		return all
	}
	deltaNames := make(map[string]struct{})
	for _, deltas := range perCandidateDeltas {
		for _, d := range deltas {
			deltaNames[d.Name] = struct{}{}
		}
	}
	if len(deltaNames) == 0 {
		return nil
	}
	trimmed := make([]FieldValue, 0, len(deltaNames))
	for _, fv := range all {
		if _, ok := deltaNames[fv.Name]; ok {
			trimmed = append(trimmed, fv)
		}
	}
	return trimmed
}

// extractAllFields returns the full field list for a record. Used for the
// baseline snapshot (default mode) and for the Extra-candidate Fields list.
// Called by both baselineFields and classify (for the Extra branch in
// diff.go) — do not add trimming logic here; trimming belongs in callers.
func extractAllFields(rec dal.Record) []FieldValue {
	v := deref(reflect.ValueOf(rec.Data()))
	switch bucket(v) {
	case bucketStruct:
		t := v.Type()
		out := make([]FieldValue, 0, t.NumField())
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			out = append(out, FieldValue{Name: f.Name, Value: v.Field(i).Interface()})
		}
		return out
	case bucketMap:
		keys := v.MapKeys()
		strs := make([]string, len(keys))
		for i, k := range keys {
			strs[i] = k.String()
		}
		sort.Strings(strs)
		out := make([]FieldValue, 0, len(strs))
		for _, k := range strs {
			out = append(out, FieldValue{Name: k, Value: v.MapIndex(reflect.ValueOf(k)).Interface()})
		}
		return out
	default:
		return []FieldValue{{Name: "_value", Value: rec.Data()}}
	}
}

type kindBucket int8

const (
	bucketStruct kindBucket = iota
	bucketMap
	bucketOther
)

func bucket(v reflect.Value) kindBucket {
	if !v.IsValid() {
		return bucketOther
	}
	switch v.Kind() {
	case reflect.Struct:
		return bucketStruct
	case reflect.Map:
		if v.Type().Key().Kind() == reflect.String {
			return bucketMap
		}
	}
	return bucketOther
}

func deref(v reflect.Value) reflect.Value {
	if v.IsValid() && v.Kind() == reflect.Pointer && !v.IsNil() {
		return v.Elem()
	}
	return v
}

func compareStruct(bv, cv reflect.Value, cfg options) []FieldValue {
	t := bv.Type()
	var deltas []FieldValue
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if _, ignored := cfg.ignoreFields[f.Name]; ignored {
			continue
		}
		bf := bv.Field(i).Interface()
		cf := cv.Field(i).Interface()
		if !reflect.DeepEqual(bf, cf) {
			deltas = append(deltas, FieldValue{Name: f.Name, Value: cf})
		}
	}
	// Sort by Name for deterministic renderer output (matches compareMap).
	sort.Slice(deltas, func(i, j int) bool { return deltas[i].Name < deltas[j].Name })
	return deltas
}

func compareMap(bv, cv reflect.Value, cfg options) []FieldValue {
	keys := unionKeys(bv, cv)
	sort.Strings(keys)
	var deltas []FieldValue
	for _, k := range keys {
		if _, ignored := cfg.ignoreFields[k]; ignored {
			continue
		}
		bk := bv.MapIndex(reflect.ValueOf(k))
		ck := cv.MapIndex(reflect.ValueOf(k))
		switch {
		case bk.IsValid() && ck.IsValid():
			if !reflect.DeepEqual(bk.Interface(), ck.Interface()) {
				deltas = append(deltas, FieldValue{Name: k, Value: ck.Interface()})
			}
		case bk.IsValid() && !ck.IsValid():
			// Present in baseline, absent from candidate.
			// Under WithAbsentEqualsNil, a nil-like baseline value collapses to no delta.
			if cfg.absentEqualsNil && isNilLike(bk) {
				continue
			}
			deltas = append(deltas, FieldValue{Name: k, Absent: true})
		case !bk.IsValid() && ck.IsValid():
			// Absent from baseline, present in candidate.
			// Under WithAbsentEqualsNil, a nil-like candidate value collapses to no delta.
			if cfg.absentEqualsNil && isNilLike(ck) {
				continue
			}
			deltas = append(deltas, FieldValue{Name: k, Value: ck.Interface()})
		}
	}
	return deltas
}

// isNilLike returns true for values that should be treated as "nil" under
// WithAbsentEqualsNil: invalid (untyped nil), interface holding nil, or a
// nilable kind (pointer/interface/map/slice/chan/func) whose IsNil() is true.
// Untyped non-nil scalars (0, "", false) are NOT nil-like.
func isNilLike(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	iv := v.Interface()
	if iv == nil {
		return true
	}
	rv := reflect.ValueOf(iv)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return rv.IsNil()
	}
	return false
}

func unionKeys(a, b reflect.Value) []string {
	seen := make(map[string]struct{})
	for _, k := range a.MapKeys() {
		seen[k.String()] = struct{}{}
	}
	for _, k := range b.MapKeys() {
		seen[k.String()] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	return out
}
