# Feature: dbschema IndexDef

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/index-def?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/index-def?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/index-def?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/index-def?op=request-change) |

**Status:** Implemented
**Source Idea:** [`dalgo-schema-modification`](../../../ideas/dalgo-schema-modification.md)
**Parent Feature:** [`dbschema`](../README.md)

## Summary

Defines the `IndexDef` struct — the portable description of one index on a collection. Carries the index name, the collection it belongs to, the ordered list of fields it covers, and a `Unique` flag.

## Behavior

### REQ: index-def-struct

The `dbschema` package MUST export the struct:

```go
type IndexDef struct {
    Name       string           // index name
    Collection string           // collection the index belongs to
    Fields     []dal.FieldName  // ordered list of fields the index covers
    Unique     bool             // true = UNIQUE INDEX
}
```

`Fields` is ordered — the order matters for composite indexes (field ordinality affects which queries the index can serve). `Collection` is a plain string (the simple name of the collection the index belongs to); richer collection addressing (catalog/schema/parent-key) lives in `dal.CollectionRef` and is the argument type passed to reader/writer methods. The godoc on `IndexDef.Collection` MUST note this asymmetry: the surrounding `SchemaReader` and `SchemaModifier` methods accept `*dal.CollectionRef`, but the stored value on `IndexDef` is the bare name. Tier-2 engine extensions MAY add a richer reference field if needed.

#### AC-1: index-def-compiles

**Given** a Go program importing `dbschema`
**When** the program declares `idx := dbschema.IndexDef{Name: "ix_users_email", Collection: "users", Fields: []dal.FieldName{"email"}, Unique: true}`
**Then** the program compiles.

#### AC-2: composite-index

**Given** a Go program importing `dbschema`
**When** the program declares `idx := dbschema.IndexDef{Name: "ix_orders_status_created", Collection: "orders", Fields: []dal.FieldName{"status", "created_at"}, Unique: false}`
**Then** the program compiles and `len(idx.Fields) == 2`.

#### AC-3: index-def-zero-value

**Given** a Go program importing `dbschema`
**When** the program declares `var idx dbschema.IndexDef`
**Then** `idx.Name == ""`, `idx.Collection == ""`, `idx.Fields == nil`, `idx.Unique == false`.

## Architecture

| File | Contents |
|---|---|
| `dbschema/index_def.go` | `IndexDef` struct + godoc. |
| `dbschema/index_def_test.go` | Tests covering the ACs above. |

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
