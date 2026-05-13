// Package ddl provides the schema-modification execution surface for
// DALgo. It defines the SchemaModifier capability interface, the
// composable AlterOp model for collection alterations, the
// TransactionalDDL capability for atomicity advertisement, and
// top-level helper functions that wrap a type assertion on dal.DB.
//
// ddl imports [github.com/dal-go/dalgo/dbschema] for the structural
// types it operates on (CollectionDef, FieldDef, IndexDef, etc.) AND
// for the shared NotSupportedError typed error. Drivers that
// implement DDL satisfy ddl.SchemaModifier; drivers that don't
// cause helper functions to return *dbschema.NotSupportedError.
package ddl
