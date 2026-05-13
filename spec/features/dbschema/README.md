# Feature: Schema Description Vocabulary & Read Capability (`dbschema`)

> [View in SpecStudio](https://specstudio.synchestra.io/project/features?id=dalgo@dal-go@github.com&path=spec%2Ffeatures%2Fdbschema) — graph, discussions, approvals

**Status:** Implemented
**Source Idea:** [`dalgo-schema-modification`](../../ideas/dalgo-schema-modification.md)

## Summary

This umbrella Feature introduces the portable schema-description vocabulary AND the read-side (introspection) capability as a new top-level dalgo sub-package `dbschema`. It contains:

- **Tier-1 types** (engine-neutral core): `FieldDef`, `CollectionDef`, `IndexDef`, `Type`, `Precision`, `DefaultExpr` + concretes, `ConstraintDef`, `Referrer`.
- **`SchemaReader` capability interface** + top-level helper functions for schema introspection — drivers opt in by implementing.
- **`NotSupportedError` typed error** shared with the [`ddl`](../ddl/README.md) write side.

This consolidates what currently lives in `datatug-cli/pkg/datatug-core/schemer/` (the engine-neutral reader interfaces) and brings it upstream. The per-engine reader implementations move to their respective driver repos as separate Features (planned: `dalgo2sql/dbschema/`, `dalgo2firestore/dbschema/`).

## Problem

DALgo today has no portable way to describe "this collection has these fields, with these types, and these indexes" — and no portable way to ASK a backend what's there. `dal.Schema` is an ORM-shape interface (key↔field mapping at runtime), not a structural description. Consumers that need to reason about schema — DDL execution, schema browsers, query builders, the cross-engine `db copy` command — must invent their own per-engine vocabularies, defeating DALgo's portability promise. This Feature provides the shared vocabulary AND the engine-neutral introspection contract.

## Three-Tier Composition

The Tier-1 types in this package are designed to be embedded by engine-specific extensions (Tier 2, in each driver repo) and by application-specific wrappers (Tier 3, in consumer repos):

```
Tier 1 (engine-neutral, this Feature):  dbschema.FieldDef { Name, Type, Length, Precision, Nullable, Default, AutoIncrement }
Tier 2 (engine extensions, follow-up):  dalgo2sql/dbschema.FieldDef { dbschema.FieldDef; DbType string; CharMaxLength *int; CharacterSet *string; ... }
Tier 3 (app wrappers, in consumers):    datatug.ColumnModel { sqlext.FieldDef; ByEnv; Checks; ... }
```

Each tier ADDS fields via Go struct embedding. No field is declared twice. Same pattern applies to `IndexDef` (Tier 2 SQL adds `IsClustered`, `IsXML`, `IsColumnStore`, `IsHash`, `IsPartial`, `Origin`, …) and `CollectionDef` (Tier 3 wraps for project-file storage and per-environment state).

## Children

Listed in **dependency order** (implement bottom-up):

| Feature | Tier | Summary |
|---|---|---|
| [types/](types/README.md) | 1 | `Type` (int8 enum: Null, Bool, Int, Float, String, Bytes, Time, Decimal) and `Precision` (Total, Scale). No dependencies. |
| [default-expr/](default-expr/README.md) | 1 | Sealed `DefaultExpr` interface + `DefaultLiteral{Value any}` + `DefaultCurrentTimestamp{}`. No dependencies. |
| [field-def/](field-def/README.md) | 1 | `FieldDef` struct — Name, Type, Length, Precision, Nullable, Default, AutoIncrement. Depends on `types`, `default-expr`. |
| [index-def/](index-def/README.md) | 1 | `IndexDef` struct — Name, Collection, Fields, Unique. Depends on `dal.FieldName`. |
| [collection-def/](collection-def/README.md) | 1 | `CollectionDef` struct — Name, Fields, PrimaryKey, Indexes. Depends on `field-def`, `index-def`. |
| [errors/](errors/README.md) | 1 | `NotSupportedError` typed error wrapping `dal.ErrNotSupported`. Shared with [`ddl`](../ddl/README.md). |
| [schema-reader/](schema-reader/README.md) | 1 | `SchemaReader` capability interface (`ListCollections`, `DescribeCollection`, `ListIndexes`, optional `ListConstraints`/`ListReferrers`) + top-level helpers + `ConstraintDef` + `Referrer` types. |

## Non-Goals

- **No `ALTER`-style mutation primitives.** Writes live in [`ddl`](../ddl/README.md).
- **No schema migration / diffing.** That's a consumer concern (e.g. datatug's comparator).
- **No engine-specific fields on Tier 1.** Things like `CharMaxLength`, `CharacterSet`, `IsClustered`, `IsXML` belong in Tier-2 extensions in their respective driver repos. Tier 1 stays engine-neutral.
- **No serialization format prescribed.** Types are pure data; on-disk representation is each consumer's choice (e.g. datatug's project-file format).
- **No validation logic.** A `CollectionDef` with a `PrimaryKey` naming a non-existent `FieldDef` compiles. Validation is the consumer's responsibility (typically `ddl`, which surfaces driver-side errors).
- **No per-engine reader implementations.** Concrete `SchemaReader` impls live in driver repos.
- **No app-specific concerns.** `ByEnv`, `Checks`, project-item metadata stay in datatug-cli's Tier-3 wrappers.

## Architecture

```
dbschema/                            ← this umbrella Feature's package (Tier 1)
├── doc.go                           ← package godoc, three-tier composition explainer
├── type.go                          ← Type, Precision
├── default_expr.go                  ← DefaultExpr, DefaultLiteral, DefaultCurrentTimestamp
├── field_def.go                     ← FieldDef
├── index_def.go                     ← IndexDef
├── collection_def.go                ← CollectionDef
├── errors.go                        ← NotSupportedError
├── reader.go                        ← SchemaReader interface
├── reader_helpers.go                ← top-level helper functions
├── constraint.go                    ← ConstraintDef
├── referrer.go                      ← Referrer
└── *_test.go                        ← in-package tests
```

Imports: `dal` (for `dal.FieldName`, `dal.DB`, `dal.Adapter`, `dal.Key`, `dal.CollectionRef`, `dal.ErrNotSupported`). Nothing else.

## Testing Strategy

In-package Go tests in `*_test.go` files. Each child Feature scopes its own tests. Type/struct tests are mostly structural — confirm zero values are sensible, public fields accessible, `Type.String()` returns expected values, the sealed `DefaultExpr` interface rejects unauthorized implementations. `SchemaReader` tests use two stub `dal.DB` types (one implements, one doesn't) to verify helper dispatch and the `*NotSupportedError` fall-through.

## Rehearse Integration

All ACs are testable via Go's built-in test runner. No external test scaffolding needed. Rehearse stubs intentionally skipped.

## Assumption Carryover

From the source Idea (now scope-expanded to cover reader + writer + three-tier composition):

| Idea assumption | Status |
|---|---|
| Small portable type-set is sufficient for SQLite + PostgreSQL + inGitDB | Carried; encoded in `types/` Feature. |
| Optional length/precision hints from day 1 (no public-API break later) | Carried; encoded in `field-def/`. |
| `Unique bool` on indexes from day 1 | Carried; encoded in `index-def/`. |
| `Def` suffix distinguishes schema/runtime types | Carried uniformly. |
| Schema-info types have multiple consumers (DDL, browsers, query builders) | Validated; supports the engine-neutral Tier 1 + reader interface design. |
| Existing datatug schemer is battle-tested and worth upstream-ing | Validated by code review of `pkg/datatug-core/schemer/` and `pkg/schemers/*`. |

## Outstanding Questions

- **Whether `ListCollections` should accept a richer scope object** than `parent *dal.Key` (e.g. catalog+schema pair for SQL). Tier-1 keeps it minimal; Tier-2 SQL extensions MAY add richer methods. Plan time decides if the Tier-1 surface needs adjustment.
- **Whether `Referrer.Fields` is sufficient** or whether reciprocal foreign-key metadata (cascade actions, deferred-vs-immediate) needs Tier-1 representation. Defer to plan time.

---
*This document follows the https://specscore.md/feature-specification*
