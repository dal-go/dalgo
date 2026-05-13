# Feature: dbschema SchemaReader (Introspection Capability)

> [View in SpecStudio](https://specstudio.synchestra.io/project/features?id=dalgo@dal-go@github.com&path=spec%2Ffeatures%2Fdbschema%2Fschema-reader) — graph, discussions, approvals

**Status:** Implemented
**Source Idea:** [`dalgo-schema-modification`](../../../ideas/dalgo-schema-modification.md)
**Parent Feature:** [`dbschema`](../README.md)

## Summary

Defines the `SchemaReader` capability interface for schema introspection — the read-side mirror of [`ddl.SchemaModifier`](../../ddl/schema-modifier/README.md). Drivers that support introspection (`dalgo2sql` via SQL information_schema / SQLite pragmas, `dalgo2firestore` via the Firestore admin API, etc.) opt in by implementing `SchemaReader` on their `dal.DB` value. Drivers that don't (in-memory mocks, raw key-value stores) simply don't implement it; the helper functions return `*dbschema.NotSupportedError`.

This Feature consolidates what currently lives in `datatug-cli/pkg/datatug-core/schemer/` (the engine-neutral reader interfaces) and brings it upstream into dalgo. The per-engine implementations move to their respective driver repos as separate Features (`dalgo2sql/dbschema/`, `dalgo2firestore/dbschema/`).

## Behavior

### REQ: schema-reader-interface

The `dbschema` package MUST export the capability interface:

```go
type SchemaReader interface {
    // ListCollections returns the collections (tables) accessible to db. The optional
    // parent key narrows scope when the backend supports hierarchical addressing
    // (e.g. SQL catalog/schema). Pass nil for "everything visible."
    ListCollections(ctx context.Context, parent *dal.Key) ([]dal.CollectionRef, error)

    // DescribeCollection returns the structural definition of one collection,
    // including its fields, primary key, and inline indexes.
    DescribeCollection(ctx context.Context, ref *dal.CollectionRef) (*CollectionDef, error)

    // ListIndexes returns the indexes on a collection. The returned slice MAY include
    // indexes already reported inline via DescribeCollection's `Indexes` field.
    ListIndexes(ctx context.Context, ref *dal.CollectionRef) ([]IndexDef, error)

    // ListConstraints is OPTIONAL. Drivers that do not support constraint introspection
    // MUST return *NotSupportedError. Drivers that support it return all non-PK,
    // non-FK constraints (CHECK, UNIQUE constraints declared at the table level, etc.).
    ListConstraints(ctx context.Context, ref *dal.CollectionRef) ([]ConstraintDef, error)

    // ListReferrers is OPTIONAL. Drivers MAY return *NotSupportedError. Returns the
    // collections that reference `ref` via foreign keys.
    ListReferrers(ctx context.Context, ref *dal.CollectionRef) ([]Referrer, error)
}
```

Drivers MUST implement `ListCollections`, `DescribeCollection`, and `ListIndexes` if they implement `SchemaReader` at all. `ListConstraints` and `ListReferrers` MAY return `*NotSupportedError` for drivers whose backend lacks the concept (e.g., Firestore has no SQL-style constraints).

The Tier-1 reader interface returns Tier-1 types (`CollectionDef`, `IndexDef`, etc.). Drivers that produce engine-specific extension types (Tier 2) MAY also implement engine-specific reader interfaces in their own package — outside the scope of this Feature.

#### AC-1: interface-exists

**Given** a Go program importing `dbschema`
**When** the program references `dbschema.SchemaReader`
**Then** the program compiles.

#### AC-2: method-signatures

**Given** a stub type that declares all five methods with the exact signatures above
**When** the stub attempts `var _ dbschema.SchemaReader = (*stub)(nil)`
**Then** the assertion compiles.

#### AC-3: partial-impl-rejected

**Given** a stub that declares only four of the five methods (omits any one of the five — required or optional, since the entire interface must be implemented to satisfy the type assertion)
**When** the stub attempts `var _ dbschema.SchemaReader = (*stub)(nil)`
**Then** the program fails to compile with a "missing method" error naming the absent method.

### REQ: helper-functions

The `dbschema` package MUST export top-level helper functions mirroring the methods on `SchemaReader`:

```go
func ListCollections(ctx context.Context, db dal.DB, parent *dal.Key) ([]dal.CollectionRef, error)
func DescribeCollection(ctx context.Context, db dal.DB, ref *dal.CollectionRef) (*CollectionDef, error)
func ListIndexes(ctx context.Context, db dal.DB, ref *dal.CollectionRef) ([]IndexDef, error)
func ListConstraints(ctx context.Context, db dal.DB, ref *dal.CollectionRef) ([]ConstraintDef, error)
func ListReferrers(ctx context.Context, db dal.DB, ref *dal.CollectionRef) ([]Referrer, error)
```

Each helper type-asserts `db.(dbschema.SchemaReader)`:
- Success → delegates to the corresponding method.
- Failure → returns `*dbschema.NotSupportedError` with `Op` set to the method name and `Backend` set to `db.Adapter().Name()` if `db.Adapter()` returns a non-nil `dal.Adapter`; otherwise `Backend` is left as the empty string.

This mirrors the helper-dispatch contract on the write side ([`ddl/operations/`](../../ddl/operations/README.md) REQ:not-supported-on-non-implementer).

#### AC-1: helpers-compile

**Given** a Go program importing `dbschema`
**When** the program references all five top-level helper functions
**Then** the program compiles.

#### AC-2: describe-collection-dispatches

**Given** a stub `dal.DB` that implements `dbschema.SchemaReader` and records its calls
**When** the program calls `dbschema.DescribeCollection(ctx, stub, ref)`
**Then** the stub's `DescribeCollection` method is invoked exactly once with the same `ctx` and `ref`.

#### AC-3: not-implementer-returns-typed-error

**Given** a `dal.DB` stub `db` whose concrete type does NOT implement `dbschema.SchemaReader`
**When** the program calls `dbschema.DescribeCollection(ctx, db, ref)`
**Then** the returned error is non-nil, `errors.Is(err, dal.ErrNotSupported)` is `true`, and `errors.As(err, &ue)` succeeds with `ue.Op == "DescribeCollection"`.

### REQ: supporting-types

The `dbschema` package MUST export two supporting types referenced by `ListConstraints` and `ListReferrers`:

```go
type ConstraintDef struct {
    Name string
    Type string  // engine-neutral kind: "check", "unique", "primary-key", "foreign-key"
}

type Referrer struct {
    Collection dal.CollectionRef
    Fields     []dal.FieldName  // fields in `Collection` that reference back to the queried collection
}
```

These types are minimal at Tier 1. The richer shape required for specific constraint kinds (check expression, unique field list, foreign-key target + cascade actions, etc.) is intentionally deferred and tracked under Outstanding Questions. Engine-specific reader extensions (Tier 2) MAY define richer constraint/referrer types in their own packages without waiting for Tier 1 to grow.

#### AC-1: types-compile

**Given** a Go program importing `dbschema`
**When** the program declares `var c dbschema.ConstraintDef; var r dbschema.Referrer`
**Then** the program compiles.

## Architecture

| File | Contents |
|---|---|
| `dbschema/reader.go` | `SchemaReader` interface declaration + godoc. |
| `dbschema/reader_helpers.go` | Top-level helper functions. |
| `dbschema/constraint.go` | `ConstraintDef` struct + godoc. |
| `dbschema/referrer.go` | `Referrer` struct + godoc. |
| `dbschema/reader_test.go` | Tests covering the ACs above with stub `dal.DB` types. |

## Testing Strategy

In-package Go tests. Two stub `dal.DB` types in the test file: one implements `SchemaReader` and records calls; one does not. Helper-dispatch ACs verify both paths.

## Migration Note

This Feature consolidates the existing `datatug-cli/pkg/datatug-core/schemer/` interfaces (`ColumnsProvider`, `IndexesProvider`, `IndexColumnsProvider`, `ConstraintsProvider`, `ReferrersProvider`, `ForeignKeysProvider`) into the unified `SchemaReader` interface. The migration is intentional: the per-aspect interfaces produced N capability checks per call site; a single capability interface is simpler and matches the dalgo pattern (cf. `ConcurrencyAware`, `SchemaModifier`).

Per-engine introspection logic that today lives in `datatug-cli/pkg/schemers/sqliteschema/`, `mssqlschema/`, `sqlinfoschema/`, `firestoreschema/` moves to the respective driver repos as separate Features:

- `dalgo2sql/dbschema/` — SQLite, MSSQL, information-schema readers
- `dalgo2firestore/dbschema/` — Firestore reader

Each driver's reader implements `dbschema.SchemaReader` (Tier 1) and MAY return engine-specific extension types via additional reader methods (Tier 2).

## Outstanding Questions

- **`ConstraintDef` shape.** The Tier-1 struct is minimal — three fields. At Feature-spec time it's unclear whether all required constraint metadata is captured. Likely needs refinement once the SQL Tier-2 extension is drafted. Defer.
- **`ListCollections` parent-key semantics.** SQL catalog/schema vs Firestore parent document — both fit the `parent *dal.Key` shape but with different semantics. The interface is generic; godoc must spell out each backend's interpretation.

---
*This document follows the https://specscore.md/feature-specification*
