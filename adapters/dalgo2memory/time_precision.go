package dalgo2memory

import (
	"reflect"
	"sync"
	"time"
	"unsafe"

	"github.com/dal-go/dalgo/dal"
)

var timeType = reflect.TypeOf(time.Time{})

// truncateTimes returns data with every time.Time it holds truncated to
// dal.TimePrecision, the finest precision production storage keeps (see
// dal.TimePrecision). The memory adapter calls it on every value it is about to
// store, so a read returns what a production adapter would return.
//
// It reaches every time.Time (and *time.Time) that encoding/json persists:
// exported struct fields, nested structs, exported fields promoted from
// embedded structs (exported or not, by value or by pointer), pointers,
// slices, arrays, map values and interface values (so map[string]any and []any
// are covered). It never mutates its input: when nothing changes, data itself
// is returned; otherwise only the containers on the path to a changed time are
// copied, and everything else is shared exactly as before.
//
// A type that cannot hold a time (see mayHoldTime) is skipped without a walk.
// A value that refers to itself is walked once per pointer, map or slice: the
// back reference is left as it is, so the JSON encoder that follows still
// reports the cycle as an error, as it did before truncation existed.
func truncateTimes(data any) any {
	if data == nil {
		return nil
	}
	w := timeWalker{}
	v, changed := w.walk(reflect.ValueOf(data))
	if !changed {
		return data
	}
	return v.Interface()
}

// visitKey identifies one pointer, map or slice the walk has entered.
type visitKey struct {
	ptr uintptr
	typ reflect.Type
	len int
}

// visitResult is the walk's result for a visitKey; done is false while the
// walk is still inside it, which is how a cycle is recognised.
type visitResult struct {
	value   reflect.Value
	changed bool
	done    bool
}

type timeWalker struct {
	visited map[visitKey]*visitResult
}

// walk returns v with its times truncated and whether anything changed. When
// changed is false the returned value is v itself.
func (w *timeWalker) walk(v reflect.Value) (reflect.Value, bool) {
	t := v.Type()
	if !mayHoldTime(t) {
		return v, false
	}
	if t == timeType {
		original := v.Interface().(time.Time)
		normalized := dal.NormalizeTime(original)
		if normalized.Equal(original) { // already at storage precision
			return v, false
		}
		return reflect.ValueOf(normalized), true
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice:
		if v.IsNil() || v.Kind() == reflect.Slice && v.Len() == 0 {
			return v, false
		}
		key := visitKey{ptr: v.Pointer(), typ: t}
		if v.Kind() == reflect.Slice {
			key.len = v.Len()
		}
		if w.visited == nil {
			w.visited = make(map[visitKey]*visitResult)
		}
		if seen, ok := w.visited[key]; ok {
			if !seen.done {
				return v, false // a cycle: leave the back reference for the encoder to reject
			}
			return seen.value, seen.changed
		}
		result := &visitResult{}
		w.visited[key] = result
		result.value, result.changed = w.walkReference(v)
		result.done = true
		return result.value, result.changed
	case reflect.Interface:
		if v.IsNil() {
			return v, false
		}
		elem, changed := w.walk(v.Elem())
		if !changed {
			return v, false
		}
		out := reflect.New(t).Elem()
		out.Set(elem)
		return out, true
	case reflect.Struct:
		return w.walkStruct(v)
	default: // reflect.Array: mayHoldTime admits no other kind
		return w.walkElements(v, func() reflect.Value {
			out := reflect.New(t).Elem()
			out.Set(v)
			return out
		})
	}
}

// walkReference walks a non-nil pointer, map or non-empty slice.
func (w *timeWalker) walkReference(v reflect.Value) (reflect.Value, bool) {
	t := v.Type()
	switch v.Kind() {
	case reflect.Pointer:
		elem, changed := w.walk(v.Elem())
		if !changed {
			return v, false
		}
		p := reflect.New(t.Elem())
		p.Elem().Set(elem)
		return p, true
	case reflect.Slice:
		return w.walkElements(v, func() reflect.Value {
			out := reflect.MakeSlice(t, v.Len(), v.Len())
			reflect.Copy(out, v)
			return out
		})
	default: // reflect.Map
		var out reflect.Value
		iter := v.MapRange()
		for iter.Next() {
			elem, changed := w.walk(iter.Value())
			if !changed {
				continue
			}
			if !out.IsValid() {
				out = reflect.MakeMapWithSize(t, v.Len())
				all := v.MapRange()
				for all.Next() {
					out.SetMapIndex(all.Key(), all.Value())
				}
			}
			out.SetMapIndex(iter.Key(), elem)
		}
		if !out.IsValid() {
			return v, false
		}
		return out, true
	}
}

// walkElements walks a slice or an array; copyOf makes the settable copy the
// first changed element is written to.
func (w *timeWalker) walkElements(v reflect.Value, copyOf func() reflect.Value) (reflect.Value, bool) {
	var out reflect.Value
	for i := 0; i < v.Len(); i++ {
		elem, changed := w.walk(v.Index(i))
		if !changed {
			continue
		}
		if !out.IsValid() {
			out = copyOf()
		}
		out.Index(i).Set(elem)
	}
	if !out.IsValid() {
		return v, false
	}
	return out, true
}

// walkStruct walks the fields encoding/json persists (see persistedField). A
// field of an unexported embedded struct is read and written through an
// addressable copy, as reflection refuses to touch unexported fields directly.
func (w *timeWalker) walkStruct(v reflect.Value) (reflect.Value, bool) {
	t := v.Type()
	src, out := v, reflect.Value{}
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !persistedField(sf) || !mayHoldTime(sf.Type) {
			continue
		}
		if !sf.IsExported() && !src.CanAddr() {
			src = addressableCopy(v)
		}
		field, changed := w.walk(structField(src, i, sf))
		if !changed {
			continue
		}
		if !out.IsValid() {
			out = addressableCopy(v)
		}
		structField(out, i, sf).Set(field)
	}
	if !out.IsValid() {
		return v, false
	}
	return out, true
}

func addressableCopy(v reflect.Value) reflect.Value {
	out := reflect.New(v.Type()).Elem()
	out.Set(v)
	return out
}

// structField returns field i of s; an unexported field must be addressable
// and is returned readable and settable.
func structField(s reflect.Value, i int, sf reflect.StructField) reflect.Value {
	if sf.IsExported() {
		return s.Field(i)
	}
	return reflect.NewAt(sf.Type, unsafe.Pointer(s.Field(i).UnsafeAddr())).Elem()
}

// persistedField reports whether encoding/json persists sf or fields promoted
// from it: an exported field, or an unexported embedded struct or pointer to
// a struct, whose exported fields are promoted.
func persistedField(sf reflect.StructField) bool {
	if sf.IsExported() {
		return true
	}
	if !sf.Anonymous {
		return false
	}
	t := sf.Type
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Kind() == reflect.Struct
}

// mayHoldTimeCache memoises mayHoldTime per reflect.Type.
var mayHoldTimeCache sync.Map

// mayHoldTime reports whether a value of type t can hold a time.Time that
// encoding/json persists. An interface type answers yes, as its dynamic value
// may. Typed records whose fields hold no time are thus skipped entirely.
func mayHoldTime(t reflect.Type) bool {
	if cached, ok := mayHoldTimeCache.Load(t); ok {
		return cached.(bool)
	}
	result := typeReachesTime(t, map[reflect.Type]bool{})
	mayHoldTimeCache.Store(t, result)
	return result
}

// typeReachesTime is the uncached search behind mayHoldTime. seen stops a
// recursive type: a type already on the path adds nothing new.
func typeReachesTime(t reflect.Type, seen map[reflect.Type]bool) bool {
	if t == timeType {
		return true
	}
	if seen[t] {
		return false
	}
	seen[t] = true
	switch t.Kind() {
	case reflect.Interface:
		return true
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		return typeReachesTime(t.Elem(), seen)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if sf := t.Field(i); persistedField(sf) && typeReachesTime(sf.Type, seen) {
				return true
			}
		}
	}
	return false
}

// truncateConditionTimes returns cond with every time.Time in its constant
// operands truncated to dal.TimePrecision. A query applies it once, before
// planning, because production storage encodes a query bound at the precision
// it stores; every later step (an engine's candidate lookup, the per-row
// match) then compares like with like and needs no knowledge of precision.
func truncateConditionTimes(cond dal.Condition) dal.Condition {
	switch c := cond.(type) {
	case dal.Comparison:
		return dal.NewComparison(truncateExpressionTimes(c.Left), c.Operator, truncateExpressionTimes(c.Right))
	case dal.GroupCondition:
		children := c.Conditions()
		truncated := make([]dal.Condition, len(children))
		for i, child := range children {
			truncated[i] = truncateConditionTimes(child)
		}
		return dal.NewGroupCondition(c.Operator(), truncated...)
	default: // nil, IS [NOT] NULL, and shapes the evaluator rejects anyway
		return cond
	}
}

// truncateExpressionTimes truncates the times in a constant or array operand.
func truncateExpressionTimes(expr dal.Expression) dal.Expression {
	switch e := expr.(type) {
	case dal.Constant:
		return dal.Constant{Value: truncateTimes(e.Value)}
	case dal.Array:
		return dal.Array{Value: truncateTimes(e.Value)}
	default:
		return expr
	}
}
