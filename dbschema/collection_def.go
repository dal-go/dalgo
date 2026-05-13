package dbschema

import "github.com/dal-go/dalgo/dal"

// CollectionDef is the portable description of one collection (a.k.a.
// table) — its name, ordered fields, primary key, and inline
// declared secondary indexes.
//
// PrimaryKey is a slice of dal.FieldName. A single-field PK is a
// one-element slice; a composite PK has multiple entries; an empty
// slice means "no primary key declared" and driver-specific behavior
// applies (SQLite may auto-assign ROWID, PostgreSQL may reject, etc.).
//
// Indexes declared inline with CollectionDef are created together
// with the collection in a CreateCollection call. Indexes added or
// removed AFTER the collection exists are passed as ddl.AddIndex /
// ddl.DropIndex AlterOps to ddl.AlterCollection.
//
// The package does NOT validate that PrimaryKey or Indexes reference
// fields actually present in Fields. That's a driver concern —
// drivers MUST return a driver-specific error (or *NotSupportedError
// if the operation itself isn't supported) when validating against
// the engine.
type CollectionDef struct {
	// Name is the collection / table name.
	Name string
	// Fields lists the fields (columns) in declared order.
	Fields []FieldDef
	// PrimaryKey lists the names of fields composing the primary key.
	// Empty = no PK declared (driver-specific behavior applies).
	PrimaryKey []dal.FieldName
	// Indexes lists the secondary indexes declared inline with this
	// collection definition.
	Indexes []IndexDef
}
