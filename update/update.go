package update

import (
	"errors"
	"fmt"
	"strings"
)

// A FieldPath is a non-empty sequence of non-empty fields that reference a value.
//
// A FieldPath value should only be necessary if one of the fieldName names contains
// one of the runes ".˜*/[]". Most methods accept a simpler form of fieldName path
// as a string in which the individual fields are separated by dots.
// For example,
//
//	[]string{"a", "b"}
//
// is equivalent to the string form
//
//	"a.b"
//
// but
//
//	[]string{"*"}
//
// has no equivalent string form.
type FieldPath []string

type Update interface {
	FieldName() string
	FieldPath() FieldPath
	Value() any
}

func ByFieldName(fieldName string, value any) Update {
	if fieldName == "" {
		panic("fieldName cannot be empty")
	}
	if strings.Contains(fieldName, ".") {
		v := update{fieldPath: strings.Split(fieldName, "."), value: value}
		if err := v.Validate(); err != nil {
			panic(err)
		}
		return v
	}
	return update{fieldName: fieldName, value: value}
}

func ByFieldPath(fieldPath FieldPath, value any) Update {
	if len(fieldPath) == 0 {
		panic("fieldPath cannot be empty")
	}
	v := update{fieldPath: fieldPath, value: value}
	if err := v.Validate(); err != nil {
		panic(err)
	}
	return v
}

func DeleteByFieldName(fieldName string) Update {
	if fieldName == "" {
		panic("fieldName cannot be empty")
	}
	return update{fieldName: fieldName, value: DeleteField}
}

func DeleteByFieldPath(path ...string) Update {
	if len(path) == 0 {
		panic("fieldPath cannot be empty")
	}
	return update{fieldPath: path, value: DeleteField}
}

// update defines an update of a single fieldName
type update struct {
	fieldName string
	fieldPath FieldPath
	value     any
}

func (v update) FieldName() string {
	return v.fieldName
}

func (v update) FieldPath() FieldPath {
	return v.fieldPath
}

func (v update) Value() any {
	return v.value
}

// Validate validates the update
func (v update) Validate() error {
	if strings.TrimSpace(v.fieldName) == "" && len(v.fieldPath) == 0 {
		return errors.New("either fieldName or fieldPath must be provided")
	}
	if strings.Contains(v.fieldName, ".") {
		return fmt.Errorf("fieldName contains '.' character: %q", v.fieldName)
	}
	if v.fieldName != "" && len(v.fieldPath) > 0 {
		return fmt.Errorf("both FieldVal and fieldPath are provided: %v, %+v", v.fieldName, v.fieldPath)
	}
	for i, fp := range v.fieldPath {
		if strings.TrimSpace(fp) == "" {
			return fmt.Errorf("empty field path component at index %d", i)
		}
	}
	return nil
}

type sentinel int

const (
	// DeleteField is used as a value in a call to update or Set with merge to indicate
	// that the corresponding child should be deleted.
	DeleteField sentinel = iota

	// ServerTimestamp is used as a value in a call to update to indicate that the
	// child's value should be set to the time at which the server processed
	// the request.
	//
	// ServerTimestamp must be the value of a fieldName directly; it cannot appear in
	// array or struct values, or in any value that is itself inside an array or
	// struct.
	ServerTimestamp
)
