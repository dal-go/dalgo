---
format: https://specscore.md/feature-specification
status: Implemented
---

# Feature: dbschema CollectionDef

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/collection-def?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/collection-def?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/collection-def?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/collection-def?op=request-change) |

**Status:** Implemented
**Source Idea:** [`dalgo-schema-modification`](../../../ideas/dalgo-schema-modification.md)
**Parent Feature:** [`dbschema`](../README.md)

## Summary

Defines the `CollectionDef` struct — the portable description of one collection (a.k.a. table). Carries the collection name, ordered list of fields, primary-key fields (single or composite), and inline-declared secondary indexes.

## Behavior

### REQ: collection-def-struct

The `dbschema` package MUST export the struct:

```go
type CollectionDef struct {
    Name       string           // collection / table name
    Fields     []FieldDef       // fields (columns) in declared order
    PrimaryKey []dal.FieldName  // names of fields composing the primary key
    Indexes    []IndexDef       // secondary indexes declared with the collection
}
```

`PrimaryKey` is a `[]dal.FieldName`. A single-field PK is a one-element slice; a composite PK has multiple entries; an empty slice means "no primary key declared" — driver-specific behavior applies (SQLite may auto-assign ROWID, PostgreSQL may reject, etc.).

`Indexes` declared here are inline with the collection definition. Indexes added or removed AFTER the collection exists are passed as `AlterOp` values to [`ddl.AlterCollection`](../../ddl/alter-ops/README.md) — specifically `ddl.AddIndex(idx)` and `ddl.DropIndex(name)`. There are no standalone `CreateIndex`/`DropIndex` methods on `SchemaModifier`.

The package does NOT validate that `PrimaryKey` or `Indexes` reference fields actually present in `Fields`. That's a driver concern — drivers MUST return a driver-specific error (or `*dbschema.NotSupportedError` if the operation itself isn't supported) when validating against the engine.

#### AC-1: collection-def-compiles

**Given** a Go program importing `dbschema`
**When** the program declares a `CollectionDef` with two fields and a single-field primary key
**Then** the program compiles.

#### AC-2: composite-pk

**Given** a `CollectionDef` whose `PrimaryKey` has two entries (e.g. `[]dal.FieldName{"tenant_id", "user_id"}`)
**When** the program compiles
**Then** the structure is well-formed; validation that the named fields exist is left to drivers.

#### AC-3: collection-with-indexes

**Given** a `CollectionDef` declaring two fields and one inline `IndexDef`
**When** the program compiles
**Then** `len(c.Fields) == 2 && len(c.Indexes) == 1`.

#### AC-4: collection-def-zero-value

**Given** a Go program importing `dbschema`
**When** the program declares `var c dbschema.CollectionDef`
**Then** `c.Name == ""`, `c.Fields == nil`, `c.PrimaryKey == nil`, `c.Indexes == nil`.

## Architecture

| File | Contents |
|---|---|
| `dbschema/collection_def.go` | `CollectionDef` struct + godoc. |
| `dbschema/collection_def_test.go` | Tests covering the ACs above. |

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
