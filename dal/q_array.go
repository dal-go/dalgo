package dal

import (
	"fmt"
	"reflect"
	"strings"
)

var _ Expression = Array{}
var _ Expression = (*Array)(nil)

type Array struct {
	Value any `json:"value"`
}

func NewArray(v any) Array {
	switch value := v.(type) {
	case []string, []int, []int64, []uint, []uint8, []uint16, []uint32, []uint64, []float32, []float64:
		return Array{Value: value}
	default:
		panic(fmt.Sprintf("unsupported type %T", v))
	}
}

func (v Array) Equal(b Array) bool {
	// Handle nil cases
	if v.Value == nil && b.Value == nil {
		return true
	}
	if v.Value == nil || b.Value == nil {
		return false
	}

	vVal := reflect.ValueOf(v.Value)
	bVal := reflect.ValueOf(b.Value)

	// Check if both are slices
	if vVal.Kind() != reflect.Slice || bVal.Kind() != reflect.Slice {
		// For non-slice values, use direct comparison
		return v.Value == b.Value
	}

	// Check if at least one is []any slice (requirement is satisfied)
	vType := vVal.Type()
	bType := bVal.Type()
	_ = (vType.Elem().Kind() == reflect.Interface) || (bType.Elem().Kind() == reflect.Interface)

	// For all slices (whether []any or not), we need to compare elements
	// If lengths are different, slices are not equal
	if vVal.Len() != bVal.Len() {
		return false
	}

	// Compare elements
	for i := 0; i < vVal.Len(); i++ {
		vElem := vVal.Index(i).Interface()
		bElem := bVal.Index(i).Interface()
		if vElem != bElem {
			return false
		}
	}

	return true
}

// String returns string representation of a Constant
func (v Array) String() string {
	// Handle the nil case
	if v.Value == nil {
		return "()"
	}

	switch value := v.Value.(type) {
	case []string:
		if len(value) == 0 {
			return "()"
		}
		return "('" + strings.Join(value, "','") + "')"
	default:
		// Check if value is a slice
		val := reflect.ValueOf(v.Value)
		if val.Kind() == reflect.Slice {
			// Convert slice elements to strings and join them
			var elements []string
			for i := 0; i < val.Len(); i++ {
				elem := val.Index(i).Interface()
				elements = append(elements, fmt.Sprintf("%v", elem))
			}
			return "(" + strings.Join(elements, ",") + ")"
		}
		// Panic for non-slice types
		panic(fmt.Sprintf("unsupported type %T", v.Value))
	}
}
