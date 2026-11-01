package dalgo2memory

import (
	"encoding"
	"encoding/json"
	"fmt"
	"github.com/dal-go/dalgo/dal"
	"maps"
	"reflect"
	"strings"
	"time"
)

// orderedTimestampSources prepares comparison-only values. The declared source
// schema wins over the result type. An unprojected IntoRecord may supply a type
// only when the source has no schema. Stored JSON and output projections stay
// untouched, and date-looking string fields retain lexical ordering.
func orderedTimestampSources(rows []memoryRow, q dal.StructuredQuery, factory func() any) (map[string]map[string]any, error) {
	if factory == nil && len(q.Columns()) == 0 {
		if template := q.IntoRecord(); template != nil && template.Data() != nil {
			typ := reflect.TypeOf(template.Data())
			if typ.Kind() == reflect.Pointer {
				typ = typ.Elem()
			}
			factory = func() any { return reflect.New(typ).Interface() }
		}
	}
	if factory == nil || len(q.OrderBy()) == 0 {
		return nil, nil
	}
	out := make(map[string]map[string]any, len(rows))
	for _, row := range rows {
		raw := factory()
		if raw == nil {
			continue
		}
		for _, order := range q.OrderBy() {
			field, ok := order.Expression().(dal.FieldRef)
			if !ok || field.IsID() {
				continue
			}
			stored, present := fieldValue(row.data, field.Name())
			if !present || stored == nil {
				continue
			}
			switch orderedTimestampField(raw, field.Name()).(type) {
			case time.Time, *time.Time:
			default:
				continue
			}
			var instant time.Time
			switch value := stored.(type) {
			case time.Time:
				instant = value
			case string:
				var err error
				instant, err = time.Parse(time.RFC3339Nano, value)
				if err != nil {
					return nil, fmt.Errorf("dalgo2memory: invalid ordered timestamp %q: %w", field.Name(), err)
				}
			default:
				return nil, fmt.Errorf("dalgo2memory: invalid ordered timestamp %q: %T", field.Name(), stored)
			}
			if out[row.id] == nil {
				out[row.id] = maps.Clone(row.data)
			}
			out[row.id][field.Name()] = instant
		}
	}
	return out, nil
}

var (
	jsonMarshalerType = reflect.TypeFor[json.Marshaler]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
)

// The following private JSON field resolution matches dal's ordered aggregate
// source-field policy. Keep custom marshalers and ambiguous embedded fields
// opaque; they do not establish a declared timestamp under the JSON field name.
func orderedTimestampField(data any, path string) any {
	current := reflect.ValueOf(data)
	for _, part := range strings.Split(path, ".") {
		for current.IsValid() && (current.Kind() == reflect.Interface || current.Kind() == reflect.Pointer) {
			if current.IsNil() {
				if current.Kind() != reflect.Pointer {
					return nil
				}
				current = reflect.New(current.Type().Elem()).Elem()
			} else {
				current = current.Elem()
			}
		}
		if !current.IsValid() {
			return nil
		}
		switch current.Kind() {
		case reflect.Map:
			if writesItself(current) {
				return nil
			}
			if current.Type().Key().Kind() != reflect.String {
				return nil
			}
			element := current.MapIndex(reflect.ValueOf(part).Convert(current.Type().Key()))
			if !element.IsValid() {
				element = reflect.New(current.Type().Elem()).Elem()
			}
			current = element
		case reflect.Struct:
			if writesItself(current) {
				return nil
			}
			field, found := promotedStructField(current, part)
			if !found {
				return nil
			}
			current = field
		default:
			return nil
		}
	}
	if !current.IsValid() || !current.CanInterface() {
		return nil
	}
	return current.Interface()
}

// writesItself reports whether encoding/json writes a struct or map value by calling a
// method of its type, MarshalJSON or MarshalText, and not field by field. The
// method of the pointer type is called for a value that can be addressed (a row
// given by pointer, a field of one) and not for another.
func writesItself(value reflect.Value) bool {
	valueType := value.Type()
	if valueType.Implements(jsonMarshalerType) || valueType.Implements(textMarshalerType) {
		return true
	}
	pointerType := reflect.PointerTo(valueType)
	return value.CanAddr() && (pointerType.Implements(jsonMarshalerType) || pointerType.Implements(textMarshalerType))
}

// promotedStructField finds the field encoding/json writes under name in a struct,
// among its own fields and those of the structs it embeds without a name of their
// own, depth by depth: a field of the struct shadows one of an embedded struct, and
// where several fields of one depth carry the name, a single one given the name by
// its tag wins and otherwise none does, as in encoding/json.
func promotedStructField(root reflect.Value, name string) (reflect.Value, bool) {
	level := []reflect.Value{root}
	explored := map[reflect.Type]bool{}
	for len(level) > 0 {
		var next, found, tagged []reflect.Value
		for _, current := range level {
			if explored[current.Type()] {
				continue
			}
			for i := 0; i < current.NumField(); i++ {
				field := current.Type().Field(i)
				fieldType := field.Type
				if fieldType.Kind() == reflect.Pointer {
					fieldType = fieldType.Elem()
				}
				embeddedStruct := field.Anonymous && fieldType.Kind() == reflect.Struct
				// An unexported field is not written, but the fields of an unexported embedded struct are.
				if field.Tag.Get("json") == "-" || !field.IsExported() && !embeddedStruct {
					continue
				}
				written := strings.Split(field.Tag.Get("json"), ",")[0]
				if written == "" && embeddedStruct {
					embedded := current.Field(i)
					if embedded.Kind() == reflect.Pointer {
						if embedded.IsNil() {
							embedded = reflect.New(embedded.Type().Elem()).Elem()
						} else {
							embedded = embedded.Elem()
						}
					}
					next = append(next, embedded)
					continue
				}
				isTagged := written != ""
				if !isTagged {
					written = field.Name
				}
				if written != name {
					continue
				}
				found = append(found, current.Field(i))
				if isTagged {
					tagged = append(tagged, current.Field(i))
				}
			}
		}
		switch {
		case len(found) == 1:
			return found[0], true
		case len(tagged) == 1:
			return tagged[0], true
		case len(found) > 1:
			return reflect.Value{}, false
		}
		for _, current := range level {
			explored[current.Type()] = true
		}
		level = next
	}
	return reflect.Value{}, false
}
