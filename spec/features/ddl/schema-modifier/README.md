# Feature: ddl SchemaModifier Interface

> [View in SpecStudio](https://specstudio.synchestra.io/project/features?id=dalgo@dal-go@github.com&path=spec%2Ffeatures%2Fddl%2Fschema-modifier) — graph, discussions, approvals

**Status:** Implemented
**Source Idea:** [`dalgo-schema-modification`](../../../ideas/dalgo-schema-modification.md)
**Parent Feature:** [`ddl`](../README.md)

## Summary

Defines the `SchemaModifier` capability interface — the three-method contract drivers implement to support DDL operations. The interface is NOT embedded in `dal.DB` (DDL is genuinely optional for some backends — read-only wrappers, analytics drivers, mocks). Drivers opt in by implementing `SchemaModifier` on their `dal.DB` value or a related type reachable via type assertion.

Index-level operations after initial collection creation are NOT separate methods — they are `AlterOp` values passed to `AlterCollection`. See [`alter-ops/`](../alter-ops/README.md) for the `AddIndex` and `DropIndex` constructors.

## Behavior

### REQ: schema-modifier-interface

The `ddl` package MUST export:

```go
type SchemaModifier interface {
    CreateCollection(ctx context.Context, c dbschema.CollectionDef, opts ...Option) error
    DropCollection(ctx context.Context, name string, opts ...Option) error
    AlterCollection(ctx context.Context, name string, ops ...AlterOp) error
}
```

The interface has exactly three methods with the signatures above. Drivers implement all three; partial implementations cannot satisfy the interface. Drivers that physically cannot perform a given operation (e.g. an older SQLite that lacks `DROP COLUMN`) return `*dbschema.NotSupportedError` at runtime — the interface satisfaction is structural.

`CreateCollection` creates the table along with any inline indexes declared in `CollectionDef.Indexes`. `DropCollection` drops the table along with its indexes. Adding or removing indexes AFTER initial creation goes through `AlterCollection` using the `AddIndex` / `DropIndex` AlterOps.

#### AC-1: interface-exists

**Given** a Go program importing `ddl`
**When** the program references `ddl.SchemaModifier`
**Then** the program compiles.

#### AC-2: method-signatures

**Given** a Go program that declares an empty stub type and attempts to satisfy `ddl.SchemaModifier`
**When** the stub has all three methods with the exact signatures above
**Then** the assertion `var _ ddl.SchemaModifier = (*stub)(nil)` compiles.

#### AC-3: partial-impl-rejected

**Given** a Go program that declares a stub type with only two of the three methods (any one omitted)
**When** the stub attempts `var _ ddl.SchemaModifier = (*stub)(nil)`
**Then** the program fails to compile with a "missing method" error naming the absent method.

### REQ: opt-in-not-embedded

`SchemaModifier` MUST NOT be embedded in `dal.DB`. Drivers that do not implement it remain valid `dal.DB` implementers.

#### AC-1: db-need-not-satisfy

**Given** a `dal.DB` value `db` whose concrete type does NOT implement `ddl.SchemaModifier`
**When** the program performs `_, ok := db.(ddl.SchemaModifier)`
**Then** `ok` is `false` (no panic; the assertion simply fails).

#### AC-2: db-still-valid

**Given** a `dal.DB` implementation that satisfies all existing `dal.DB` requirements but does NOT implement `ddl.SchemaModifier`
**When** the program assigns it to a `var _ dal.DB`
**Then** the assignment compiles.

## Architecture

| File | Contents |
|---|---|
| `ddl/modifier.go` | `SchemaModifier` interface declaration + godoc. |
| `ddl/modifier_test.go` | Compile-time-style tests verifying the interface shape. |

## Outstanding Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
