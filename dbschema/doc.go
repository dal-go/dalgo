// Package dbschema provides a portable schema-description vocabulary
// and read-side (introspection) capability for DALgo.
//
// The package contains Tier-1 engine-neutral types — FieldDef,
// CollectionDef, IndexDef, Type, Precision, DefaultExpr and concretes —
// designed for three-tier composition: engine-specific extensions in
// each driver repo (Tier 2) embed Tier 1; application-specific
// wrappers in consumer repos (Tier 3) embed Tier 2.
//
// The package also defines the SchemaReader capability interface for
// schema introspection (the read-side mirror of [ddl.SchemaModifier])
// and the shared NotSupportedError typed error used by both the read
// and write sides.
//
// dbschema does NOT contain operations. CREATE / DROP / ALTER live in
// the sibling [ddl] sub-package, which imports dbschema for the types
// it operates on.
package dbschema
