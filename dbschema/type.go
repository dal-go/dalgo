package dbschema

// Type is the portable column-type enumeration. Drivers translate
// these to engine-specific column types when emitting DDL.
//
// The zero value is [Null] so an unset FieldDef.Type is meaningful
// rather than silently Bool.
type Type int8

const (
	// Null indicates an unset or null-typed field. Also the zero value of Type.
	Null Type = iota
	// Bool is a boolean column.
	Bool
	// Int is an integer column. Drivers pick an appropriate width
	// (e.g. INT, BIGINT) based on optional Length hints.
	Int
	// Float is a floating-point column.
	Float
	// String is a textual column. Drivers honor optional Length
	// hints (e.g. VARCHAR(N) on PostgreSQL).
	String
	// Bytes is a binary blob column.
	Bytes
	// Time is a date-time column.
	Time
	// Decimal is a fixed-point numeric column. Drivers honor optional
	// Precision hints (e.g. NUMERIC(total, scale) on PostgreSQL).
	Decimal
)

// String returns a non-empty lowercase identifier suitable for
// diagnostic output. The returned strings are pairwise distinct.
func (t Type) String() string {
	switch t {
	case Null:
		return "null"
	case Bool:
		return "bool"
	case Int:
		return "int"
	case Float:
		return "float"
	case String:
		return "string"
	case Bytes:
		return "bytes"
	case Time:
		return "time"
	case Decimal:
		return "decimal"
	default:
		return "unknown"
	}
}

// Precision describes decimal precision. Total is the total number of
// significant digits; Scale is the number of digits to the right of
// the decimal point. Both are non-negative; the package does NOT
// enforce Scale <= Total.
type Precision struct {
	Total int
	Scale int
}
