package dalgo2memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
)

// internalGenericData is the unexported carrier for decoding engine data into
// generic transaction maps without losing integer precision.
type internalGenericData map[string]any

// UnmarshalJSON implements json.Unmarshaler using unmarshalGeneric, ensuring
// that generic JSON unmarshaling preserves numeric tokens as json.Number.
func (d *internalGenericData) UnmarshalJSON(b []byte) error {
	var m map[string]any
	if err := unmarshalGeneric(b, &m); err != nil {
		return err
	}
	*d = m
	return nil
}

// unmarshalGeneric decodes JSON bytes into target with UseNumber enabled so that
// large integer tokens are retained without lossy float64 conversion.
func unmarshalGeneric(data []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	return d.Decode(target)
}

type numKind int

const (
	numInvalid numKind = iota
	numInt
	numUint
	numFloat
)

type parsedNumber struct {
	kind numKind
	i    int64
	u    uint64
	f    float64
}

func (n parsedNumber) toFloat() float64 {
	switch n.kind {
	case numInt:
		return float64(n.i)
	case numUint:
		return float64(n.u)
	case numFloat:
		return n.f
	default:
		return 0
	}
}

// parseNumber extracts a numeric value from v into a parsedNumber.
// A plain string is never parsed as a number, preserving numeric-vs-string distinction.
func parseNumber(v any) (parsedNumber, bool) {
	if v == nil {
		return parsedNumber{}, false
	}
	if n, ok := v.(json.Number); ok {
		s := string(n)
		if !strings.ContainsAny(s, ".eE") {
			if i, err := strconv.ParseInt(s, 10, 64); err == nil {
				return parsedNumber{kind: numInt, i: i}, true
			}
			if u, err := strconv.ParseUint(s, 10, 64); err == nil {
				// ParseInt failed above, so non-negative s cannot be <= MaxInt64.
				return parsedNumber{kind: numUint, u: u}, true
			}
		}
		if f, err := n.Float64(); err == nil {
			return parsedNumber{kind: numFloat, f: f}, true
		}
		return parsedNumber{}, false
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return parsedNumber{kind: numInt, i: rv.Int()}, true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u := rv.Uint()
		if u <= math.MaxInt64 {
			return parsedNumber{kind: numInt, i: int64(u)}, true
		}
		return parsedNumber{kind: numUint, u: u}, true
	case reflect.Float32, reflect.Float64:
		return parsedNumber{kind: numFloat, f: rv.Float()}, true
	default:
		return parsedNumber{}, false
	}
}

// compareIntToFloat compares an integer parsedNumber (numInt or numUint) against a float64
// with exact 128-bit precision using math/big, avoiding any rounding of the integer.
func compareIntToFloat(a parsedNumber, f float64) int {
	if math.IsInf(f, 1) {
		return -1
	}
	if math.IsInf(f, -1) {
		return 1
	}
	bf := new(big.Float).SetPrec(128).SetFloat64(f)
	bi := new(big.Float).SetPrec(128)
	if a.kind == numInt {
		bi.SetInt64(a.i)
	} else {
		bi.SetUint64(a.u)
	}
	return bi.Cmp(bf)
}

func compareParsed(a, b parsedNumber) int {
	aNaN := a.kind == numFloat && math.IsNaN(a.f)
	bNaN := b.kind == numFloat && math.IsNaN(b.f)
	if aNaN || bNaN {
		switch {
		case aNaN && bNaN:
			return 0
		case aNaN:
			return -1
		default:
			return 1
		}
	}

	if a.kind == numInt && b.kind == numInt {
		switch {
		case a.i < b.i:
			return -1
		case a.i > b.i:
			return 1
		default:
			return 0
		}
	}
	if a.kind == numUint && b.kind == numUint {
		switch {
		case a.u < b.u:
			return -1
		case a.u > b.u:
			return 1
		default:
			return 0
		}
	}
	if a.kind == numInt && b.kind == numUint {
		// By parseNumber invariant, any parsed numInt satisfies a.i <= math.MaxInt64,
		// while any parsed numUint satisfies b.u > math.MaxInt64.
		// Therefore, canonical a is strictly less than b.
		return -1
	}
	if a.kind == numUint && b.kind == numInt {
		// By parseNumber invariant, any parsed numUint satisfies a.u > math.MaxInt64,
		// while any parsed numInt satisfies b.i <= math.MaxInt64.
		// Therefore, canonical a is strictly greater than b.
		return 1
	}
	if a.kind == numFloat && b.kind == numFloat {
		switch {
		case a.f < b.f:
			return -1
		case a.f > b.f:
			return 1
		default:
			return 0
		}
	}
	if a.kind == numFloat {
		return -compareIntToFloat(b, a.f)
	}
	return compareIntToFloat(a, b.f)
}

// numbersEqual reports whether a and b are both numbers and numerically equal.
// If either operand is NaN, numbersEqual returns false, true so that NaN never compares equal.
func numbersEqual(a, b any) (bool, bool) {
	na, okA := parseNumber(a)
	if !okA {
		return false, false
	}
	nb, okB := parseNumber(b)
	if !okB {
		return false, false
	}
	if (na.kind == numFloat && math.IsNaN(na.f)) || (nb.kind == numFloat && math.IsNaN(nb.f)) {
		return false, true
	}
	return compareParsed(na, nb) == 0, true
}

// isNaNValue reports whether v parses as a floating-point NaN.
func isNaNValue(v any) bool {
	pn, ok := parseNumber(v)
	return ok && pn.kind == numFloat && math.IsNaN(pn.f)
}

// canonicalGroupValue returns a canonical string token for GROUP BY partitioning.
// Exact integers (int, uint, json.Number, and floats without fractional parts)
// map to canonical integer tokens ("num:int:<val>") so that 1 and 1.0 group together
// while distinct large integers (such as MaxInt64 and MaxInt64 - 1) produce distinct keys.
// Strings, booleans, and non-numeric types maintain their distinct type identity.
func canonicalGroupValue(v any) string {
	if v == nil {
		return "nil"
	}
	if s, ok := v.(string); ok {
		return "str:" + s
	}
	if b, ok := v.(bool); ok {
		if b {
			return "bool:true"
		}
		return "bool:false"
	}
	if pn, ok := parseNumber(v); ok {
		switch pn.kind {
		case numInt:
			return fmt.Sprintf("num:int:%d", pn.i)
		case numUint:
			return fmt.Sprintf("num:uint:%d", pn.u)
		case numFloat:
			switch {
			case math.IsNaN(pn.f):
				return "num:nan"
			case math.IsInf(pn.f, 1):
				return "num:+inf"
			case math.IsInf(pn.f, -1):
				return "num:-inf"
			default:
				if math.Trunc(pn.f) == pn.f {
					if pn.f >= -9223372036854775808.0 && pn.f < 9223372036854775808.0 {
						i := int64(pn.f)
						if float64(i) == pn.f {
							return fmt.Sprintf("num:int:%d", i)
						}
					} else if pn.f >= 9223372036854775808.0 && pn.f < 18446744073709551616.0 {
						u := uint64(pn.f)
						if float64(u) == pn.f {
							return fmt.Sprintf("num:uint:%d", u)
						}
					}
				}
				return fmt.Sprintf("num:float:%s", strconv.FormatFloat(pn.f, 'g', -1, 64))
			}
		}
	}
	return fmt.Sprintf("%T:%v", v, v)
}

const maxSafeFloatInt = int64(1) << 53

// isUnsafeFloatNumber reports whether v is a numeric value whose representation
// exceeds the safe integer precision range of IEEE 754 double precision float64 [-2^53, 2^53]
// or is non-finite (NaN, +/-Inf).
func isUnsafeFloatNumber(v any) bool {
	pn, ok := parseNumber(v)
	if !ok {
		return false
	}
	switch pn.kind {
	case numInt:
		return pn.i > maxSafeFloatInt || pn.i < -maxSafeFloatInt
	case numUint:
		return pn.u > uint64(maxSafeFloatInt)
	default:
		// parseNumber guarantees that when ok is true, pn.kind is exclusively numInt, numUint, or numFloat.
		// Therefore default here handles numFloat values: non-finite (NaN, Inf) or magnitude > maxSafeFloatInt.
		return math.IsNaN(pn.f) || math.IsInf(pn.f, 0) || math.Abs(pn.f) > float64(maxSafeFloatInt)
	}
}

// toPublicValue normalizes internal numeric representations (like json.Number)
// to standard float64 for public query maps, keeping external map data types compatible.
func toPublicValue(v any) any {
	switch val := v.(type) {
	case json.Number:
		if f, err := val.Float64(); err == nil {
			return f
		}
		return string(val)
	case internalGenericData:
		return toPublicMap(map[string]any(val))
	case map[string]any:
		return toPublicMap(val)
	case []any:
		return toPublicSlice(val)
	default:
		return v
	}
}

// toPublicMap recursively normalizes json.Number in maps for public query results.
func toPublicMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = toPublicValue(v)
	}
	return out
}

// toPublicSlice recursively normalizes json.Number in slices for public query results.
func toPublicSlice(s []any) []any {
	if s == nil {
		return nil
	}
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = toPublicValue(v)
	}
	return out
}
