package dbschema

import "github.com/dal-go/dalgo/dal"

// FieldDef is the portable description of one field (a.k.a. column)
// of a [CollectionDef].
//
// FieldDef is the schema-DEFINITION shape. It is distinct from the
// unrelated runtime types in the dal package: [dal.Column] (a
// SELECT-clause expression+alias), [dal.FieldRef] (a query field
// reference), [dal.FieldVal] (a runtime name+value pair). Those exist
// for query and runtime concerns; FieldDef exists to describe a
// column's structure in a portable way.
//
// AutoIncrement is advisory: drivers MAY restrict it to integer types
// in the primary key and return *NotSupportedError if a caller passes
// AutoIncrement on a non-integer field or a field not in the primary
// key. dbschema itself does NOT enforce the restriction.
type FieldDef struct {
	// Name is the field's identifier.
	Name dal.FieldName
	// Type is the portable column type.
	Type Type
	// Length is an optional length hint for String / Bytes types.
	// nil means "driver default."
	Length *int
	// Precision is an optional precision hint for Decimal types.
	// nil means "driver default."
	Precision *Precision
	// Nullable is true if the field permits NULL values. Default
	// (zero value) is false = NOT NULL.
	Nullable bool
	// Default is an optional default expression. nil means "no
	// default."
	Default DefaultExpr
	// AutoIncrement is true if the field should auto-generate values.
	// Typically restricted by drivers to integer primary-key fields.
	AutoIncrement bool
}
