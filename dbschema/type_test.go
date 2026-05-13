package dbschema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestType_ConstantsExist(t *testing.T) {
	// Per REQ:type-enum AC-1 — all eight constants exist and are distinct.
	types := []Type{Null, Bool, Int, Float, String, Bytes, Time, Decimal}
	seen := make(map[Type]bool)
	for _, ty := range types {
		assert.False(t, seen[ty], "duplicate Type constant value: %d", ty)
		seen[ty] = true
	}
	assert.Len(t, seen, 8)
}

func TestType_NullIsZeroValue(t *testing.T) {
	// Per REQ:type-enum AC-2 — Null is the zero value.
	var t0 Type
	assert.Equal(t, Null, t0)
}

func TestType_String(t *testing.T) {
	// Per REQ:type-string AC-1 and AC-2 — non-empty lowercase strings, distinct.
	cases := map[Type]string{
		Null:    "null",
		Bool:    "bool",
		Int:     "int",
		Float:   "float",
		String:  "string",
		Bytes:   "bytes",
		Time:    "time",
		Decimal: "decimal",
	}
	seen := make(map[string]bool)
	for ty, want := range cases {
		got := ty.String()
		assert.Equal(t, want, got, "Type(%d).String()", ty)
		assert.False(t, seen[got], "duplicate string: %q", got)
		seen[got] = true
	}
}

func TestPrecision_Structure(t *testing.T) {
	// Per REQ:precision-struct AC-1.
	p := Precision{Total: 18, Scale: 4}
	assert.Equal(t, 18, p.Total)
	assert.Equal(t, 4, p.Scale)
}
