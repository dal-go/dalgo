package dalgo

import (
	"fmt"
)

// A FieldPath is a non-empty sequence of non-empty fields that reference a value.
//
// A FieldPath value should only be necessary if one of the field names contains
// one of the runes ".˜*/[]". Most methods accept a simpler form of field path
// as a string in which the individual fields are separated by dots.
// For example,
//   []string{"a", "b"}
// is equivalent to the string form
//   "a.b"
// but
//   []string{"*"}
// has no equivalent string form.
type FieldPath []string

type Update struct {
	Field     string
	FieldPath FieldPath
	Value     interface{}
}

func (v Update) Validate() error {
	if v.Field != "" && len(v.FieldPath) > 0 {
		return fmt.Errorf("both Field and FieldPath are provided: %v, %+v", v.Field, v.FieldPath)
	}
	return nil
}

type sentinel int

const (
	// DeleteField is used as a value in a call to Update or Set with merge to indicate
	// that the corresponding child should be deleted.
	DeleteField sentinel = iota

	// ServerTimestamp is used as a value in a call to Update to indicate that the
	// child's value should be set to the time at which the server processed
	// the request.
	//
	// ServerTimestamp must be the value of a field directly; it cannot appear in
	// array or struct values, or in any value that is itself inside an array or
	// struct.
	ServerTimestamp
)
