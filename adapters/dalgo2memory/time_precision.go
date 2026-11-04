package dalgo2memory

import (
	"reflect"
	"time"

	"github.com/dal-go/dalgo/dal"
)

var timeType = reflect.TypeOf(time.Time{})

// maxTimeTruncationDepth bounds the walk in truncateTimes. A value nested this
// deep is almost certainly self-referencing; it is left as it is, and the JSON
// encoder that follows reports the cycle as an error instead of this walk
// overflowing the stack.
const maxTimeTruncationDepth = 1000

// truncateTimes returns data with every time.Time it holds truncated to
// dal.TimePrecision, the precision real storage keeps (see dal.TimePrecision).
// The memory adapter calls it on every value it is about to store, so a read
// returns exactly what a production adapter would return.
//
// It reaches time.Time values (and *time.Time) in exported struct fields,
// nested and embedded structs, pointers, slices, arrays, map values and
// interface values (so map[string]any and []any are covered). It never
// mutates its input: when nothing changes, data itself is returned; otherwise
// only the containers on the path to a changed time are copied, and
// everything else is shared exactly as before.
func truncateTimes(data any) any {
	if data == nil {
		return nil
	}
	v, changed := truncateTimesIn(reflect.ValueOf(data), 0)
	if !changed {
		return data
	}
	return v.Interface()
}

// truncateTimesIn returns v with its times truncated and whether anything
// changed. When changed is false the returned value is v itself.
func truncateTimesIn(v reflect.Value, depth int) (reflect.Value, bool) {
	if depth > maxTimeTruncationDepth {
		return v, false
	}
	t := v.Type()
	if t == timeType {
		original := v.Interface().(time.Time)
		normalized := dal.NormalizeTime(original)
		if normalized.Equal(original) { // already at storage precision
			return v, false
		}
		return reflect.ValueOf(normalized), true
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return v, false
		}
		elem, changed := truncateTimesIn(v.Elem(), depth+1)
		if !changed {
			return v, false
		}
		p := reflect.New(t.Elem())
		p.Elem().Set(elem)
		return p, true
	case reflect.Interface:
		if v.IsNil() {
			return v, false
		}
		elem, changed := truncateTimesIn(v.Elem(), depth+1)
		if !changed {
			return v, false
		}
		out := reflect.New(t).Elem()
		out.Set(elem)
		return out, true
	case reflect.Struct:
		var out reflect.Value
		for i := 0; i < v.NumField(); i++ {
			if !t.Field(i).IsExported() {
				continue // unexported fields are not persisted (JSON skips them)
			}
			field, changed := truncateTimesIn(v.Field(i), depth+1)
			if !changed {
				continue
			}
			if !out.IsValid() {
				out = reflect.New(t).Elem()
				out.Set(v)
			}
			out.Field(i).Set(field)
		}
		if !out.IsValid() {
			return v, false
		}
		return out, true
	case reflect.Slice, reflect.Array:
		if holdsNoTime(t.Elem()) || (v.Kind() == reflect.Slice && v.IsNil()) {
			return v, false
		}
		var out reflect.Value
		for i := 0; i < v.Len(); i++ {
			elem, changed := truncateTimesIn(v.Index(i), depth+1)
			if !changed {
				continue
			}
			if !out.IsValid() {
				if v.Kind() == reflect.Slice {
					out = reflect.MakeSlice(t, v.Len(), v.Len())
					reflect.Copy(out, v)
				} else {
					out = reflect.New(t).Elem()
					out.Set(v)
				}
			}
			out.Index(i).Set(elem)
		}
		if !out.IsValid() {
			return v, false
		}
		return out, true
	case reflect.Map:
		if holdsNoTime(t.Elem()) || v.IsNil() {
			return v, false
		}
		var out reflect.Value
		iter := v.MapRange()
		for iter.Next() {
			elem, changed := truncateTimesIn(iter.Value(), depth+1)
			if !changed {
				continue
			}
			if !out.IsValid() {
				out = reflect.MakeMapWithSize(t, v.Len())
				copyIter := v.MapRange()
				for copyIter.Next() {
					out.SetMapIndex(copyIter.Key(), copyIter.Value())
				}
			}
			out.SetMapIndex(iter.Key(), elem)
		}
		if !out.IsValid() {
			return v, false
		}
		return out, true
	default:
		return v, false
	}
}

// holdsNoTime reports whether values of t can never hold a time.Time: the
// scalar kinds. It lets truncateTimes skip []byte, []string, map[string]int
// and the like without visiting every element.
func holdsNoTime(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return true
	}
	return false
}
