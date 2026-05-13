# Feature: Schema Modification (DDL) Execution Surface (`ddl`)

> [View in SpecStudio](https://specstudio.synchestra.io/project/features?id=dalgo@dal-go@github.com&path=spec%2Ffeatures%2Fddl) — graph, discussions, approvals

**Status:** Implemented
**Source Idea:** [`dalgo-schema-modification`](../../ideas/dalgo-schema-modification.md)

## Summary

This umbrella Feature defines the schema-modification execution surface as a new top-level dalgo sub-package `ddl`. It provides:

- A `SchemaModifier` capability interface that drivers opt in to (3 methods: `CreateCollection`, `DropCollection`, `AlterCollection`).
- A composable `AlterOp` model — `AlterCollection(ctx, name, ops...)` accepts a variadic list of field-level AND index-level alterations: `AddField`, `DropField`, `ModifyField`, `RenameField`, `AddIndex`, `DropIndex`. Drivers translate the batch to engine-specific DDL (e.g. one combined `ALTER TABLE` on PostgreSQL, multiple statements on SQLite).
- A `TransactionalDDL` capability interface so drivers can advertise whether they guarantee all-or-nothing atomicity for batched operations.
- A `PartialSuccessError` typed error for non-transactional drivers that partway-fail a batch.
- Top-level helper functions for the three operations, plus functional-options for opt-in idempotency. `IfNotExists()` applies to `CreateCollection` and to Add* AlterOps (`AddField`, `AddIndex`); `IfExists()` applies to `DropCollection` and to Drop* AlterOps (`DropField`, `DropIndex`). `ModifyField` and `RenameField` accept `Option` for surface symmetry but mismatched options are silently ignored (you usually want a real error if the target field is gone).

The `ddl` package imports [`dbschema`](../dbschema/README.md) for both the structural types (`CollectionDef`, `FieldDef`, `IndexDef`, etc.) AND the shared `NotSupportedError` typed error. Drivers that implement DDL satisfy `ddl.SchemaModifier`; drivers that don't cause the helper functions to return `*dbschema.NotSupportedError`.

The driving consumer is `datatug-cli`'s `db copy` command (see [`datatug-cli` Idea `cross-engine-db-copy`](https://github.com/datatug/datatug-cli/blob/main/spec/ideas/cross-engine-db-copy.md)).

## Problem

DALgo has no portable way to create or drop collections, primary keys, or indexes. The cross-engine `db copy` consumer must auto-create the target schema from the source's introspection — that's impossible without a DDL surface. Each driver currently provides its own engine-specific provisioning helpers (if any), and consumers must hand-write per-engine bootstrap code. This Feature provides the portable execution surface.

## Children

| Feature | Summary |
|---|---|
| [schema-modifier/](schema-modifier/README.md) | `SchemaModifier` capability interface — three methods: `CreateCollection`, `DropCollection`, `AlterCollection`. Drivers opt in by implementing. |
| [alter-ops/](alter-ops/README.md) | Sealed `AlterOp` interface + six constructors, each accepting `opts ...Option`: `AddField(FieldDef, opts...)`, `DropField(FieldName, opts...)`, `ModifyField(FieldName, FieldDef, opts...)`, `RenameField(old, new FieldName, opts...)`, `AddIndex(IndexDef, opts...)`, `DropIndex(name string, opts...)`. |
| [transactional-ddl/](transactional-ddl/README.md) | `TransactionalDDL` capability interface (`SupportsTransactionalDDL() bool`). Mirrors `ConcurrencyAware`. |
| [options/](options/README.md) | Functional-options pattern: `Option`, `Options`, `IfNotExists()`, `IfExists()` — apply to `CreateCollection`/`DropCollection` AND to all six AlterOp constructors. Drivers silently ignore mismatched options (e.g. `IfExists()` on an Add* op). |
| [operations/](operations/README.md) | Top-level helper functions for the three `SchemaModifier` methods. Each type-asserts against `SchemaModifier`; non-implementers cause `*dbschema.NotSupportedError`. |
| [errors/](errors/README.md) | `PartialSuccessError` typed error returned when a non-transactional driver partway-fails a batch. |
| [applier/](applier/README.md) | Visitor-pattern `Applier` interface + `ApplyTo(ctx, applier) error` method on each `AlterOp` concrete type. Lets driver-side schema modifiers dispatch AlterOps without reflection or unexported-type access. |

The shared `NotSupportedError` typed error lives in [`dbschema/errors/`](../dbschema/errors/README.md) (used by both read and write sides); `ddl` imports it.

## Non-Goals

Carried from the source Idea, reinforced at Feature-spec time:

- **No introspection (read-side `SchemaReader`).** Lives in the sibling Feature [`dbschema/schema-reader/`](../dbschema/schema-reader/README.md). This Feature is write-only.
- **No schema migration / diffing** — `AlterCollection` accepts explicit AlterOps; it does NOT compute a diff between current and desired state. Diffing is a consumer concern (e.g. datatug's comparator).
- **No idempotency on `CreateIndex`/`DropIndex` as separate methods** — those methods do not exist. Index ops are AlterOps; per-AlterOp idempotency rides on `Option`.
- **No foreign-key declarations** — defer past MVP.
- **No views, triggers, stored procedures.**
- **No per-engine raw DDL passthrough.**
- **No driver-side implementations** — `dalgo2sql` (SQLite + PostgreSQL) and `dalgo2ingitdb` adopt the interface as **separate Features in their own repos**.
- **No embedding of `SchemaModifier` into `dal.DB`** — DDL is genuinely optional (read-only wrappers, analytics drivers may not implement it). Unlike `ConcurrencyAware` (which every backend has an answer for), opt-in via type assertion is the correct shape here.

## Architecture

```
ddl/                              ← this umbrella's package
├── doc.go                        ← package godoc
├── modifier.go                   ← SchemaModifier interface (3 methods)
├── alter_op.go                   ← AlterOp sealed interface + 6 constructors
│                                    (AddField, DropField, ModifyField, RenameField,
│                                     AddIndex, DropIndex)
├── transactional.go              ← TransactionalDDL capability interface
├── options.go                    ← Option, Options, IfNotExists(), IfExists()
│                                    (apply to CreateCollection/DropCollection only)
├── operations.go                 ← top-level helpers (3)
├── errors.go                     ← PartialSuccessError
└── *_test.go                     ← in-package tests using a mock dal.DB
```

Imports: `dal` (for `dal.DB`, `dal.FieldName`, `dal.Adapter`) and `dbschema` (for `CollectionDef`, `FieldDef`, `IndexDef`, `NotSupportedError`).

### Component diagram

```
                       ┌──────────────────────────────────────┐
                       │       ddl (new top-level package)    │
                       │                                      │
                       │  ┌──────────────────────────────┐    │
                       │  │  Top-level helpers (3)       │◄───┼── consumer call site
                       │  │   CreateCollection           │    │
                       │  │   DropCollection             │    │
                       │  │   AlterCollection(ops...)    │    │
                       │  └────────────┬─────────────────┘    │
                       │               │ type-asserts         │
                       │               ▼                      │
                       │  ┌──────────────────────────────┐    │
                       │  │  SchemaModifier (interface)  │    │
                       │  └────────────┬─────────────────┘    │
                       │               │ implemented by       │
                       │               ▼                      │
                       │   driver-specific concrete type      │
                       │   (lives in dalgo2sql, dalgo2ingitdb)│
                       └──────────────────────────────────────┘
                                       │
                                       │ if type-assert fails:
                                       ▼
                       *dbschema.NotSupportedError that unwraps to
                       dal.ErrNotSupported (existing sentinel)
```

## Data Flow

**Initial creation:**

1. Caller builds a `dbschema.CollectionDef` (driver-agnostic) with inline `Fields`, `PrimaryKey`, and `Indexes`.
2. Caller invokes `ddl.CreateCollection(ctx, targetDB, c)`.
3. Helper type-asserts `targetDB.(ddl.SchemaModifier)`:
   - Success → delegates to the driver's `CreateCollection`. The driver emits engine-specific SQL (table + indexes in one or more statements) or filesystem operations.
   - Failure → helper returns `*dbschema.NotSupportedError` wrapping `dal.ErrNotSupported`.

**Post-creation alterations:**

1. Caller builds a slice of `AlterOp` values (composable: field-level AND index-level).
2. Caller invokes `ddl.AlterCollection(ctx, targetDB, "users", ddl.AddField(f), ddl.AddIndex(idx), ddl.DropField("legacy"))`.
3. Helper type-asserts and delegates to the driver's `AlterCollection`.
4. Driver decides how to apply the batch:
   - Engines that support combined `ALTER TABLE` (PostgreSQL) emit one statement.
   - Engines without combined ALTER emit a sequence; transactional drivers wrap in a transaction; non-transactional drivers MAY produce `*ddl.PartialSuccessError` if a sub-op fails partway through.
   - Consumers wanting strict atomicity check `ddl.SupportsTransactionalDDL(db)` before relying on rollback.

## Error Handling and Failure Modes

| Failure mode | Behavior |
|---|---|
| `db` does not implement `ddl.SchemaModifier` | Helper returns `*dbschema.NotSupportedError{Op, Backend, Reason: "driver does not implement ddl.SchemaModifier"}`. `errors.Is(err, dal.ErrNotSupported)` → `true`. |
| Driver supports DDL but not a specific operation | Driver returns `*dbschema.NotSupportedError{Op, Backend, Reason}`. Same `errors.Is` chain. |
| Driver supports the operation but not a specific option (e.g. `IfNotExists()`) | Driver returns `*dbschema.NotSupportedError`. |
| Driver supports the operation but does not recognize a specific `dbschema.DefaultExpr` case | Driver returns `*dbschema.NotSupportedError` with `Reason` naming the expression type. |
| `dbschema.DefaultLiteral.Value` Go type cannot be serialized to the target engine | Driver returns a driver-specific error (not `NotSupportedError` — the surface is supported, the value is invalid). |
| Strict mode: create-existing or drop-missing without `IfNotExists`/`IfExists` (on `CreateCollection`/`DropCollection` only) | Driver returns a driver-specific error. Callers wanting idempotency pass the option. |
| `AlterCollection` batch: one sub-op fails on a non-transactional driver | Driver returns `*ddl.PartialSuccessError` listing `Applied`, `FirstFailed`, `NotAttempted`, `Cause`. Transactional drivers (advertised via `TransactionalDDL.SupportsTransactionalDDL() == true`) MUST NOT produce this — they roll back and return a regular error. |
| `AlterCollection` batch: one AlterOp targets an absent/duplicate object (e.g. `AddField` on existing field) | Driver returns `*dbschema.NotSupportedError` for that op (or a driver-specific error wrapped in `PartialSuccessError.Cause` on non-transactional drivers). |

## Testing Strategy

In-package Go tests in `ddl/*_test.go`. Each child Feature scopes its own tests:

- **schema-modifier** — assert the 3-method interface signatures via compile-time type assertions on a stub implementer.
- **alter-ops** — verify each of the 6 constructors produces an `AlterOp` and the sealed-interface marker prevents external implementations (subprocess `go build` against `testdata/external_sealing/`).
- **transactional-ddl** — verify the capability interface, the `SupportsTransactionalDDL(db)` helper, and the "doesn't implement = treat as false" convention.
- **options** — verify each functional option sets the right `Options` field.
- **operations** — implement two stub `dal.DB` types in the test file: one that satisfies `SchemaModifier`, one that doesn't. Verify each of the 3 helpers dispatches correctly or returns `*dbschema.NotSupportedError`.
- **errors** — construct `*PartialSuccessError`, verify `Unwrap` chains to `Cause`, verify `errors.Is(err, dal.ErrNotSupported)` propagates when the cause is or wraps that sentinel.

`NotSupportedError` itself is tested in [`dbschema/errors/`](../dbschema/errors/README.md).

No external integration tests live in this Feature. Driver behavior is verified in each driver's repo when it adopts the interface.

## Rehearse Integration

All ACs are testable via `go test ./ddl/...`. No external test scaffolding needed. Rehearse stubs intentionally skipped.

## Assumption Carryover

From the source Idea, write-side concerns:

| Idea assumption | Status |
|---|---|
| DDL is a capability interface, not embedded into `dal.DB` | Carried; opt-in via type assertion. |
| `dal.ErrNotSupported` is the canonical sentinel; `NotSupportedError` is the typed wrapper | Carried; `Unwrap()` returns the sentinel. |
| Driver-side implementations live in driver repos | Carried; explicitly out-of-scope here. |
| Sequencing: spec + DALgo branch prototype + `replace` directive in datatug-cli | Carried. |

## CHANGELOG Entry (draft for the maintainer)

```markdown
### Added
- New top-level `ddl` sub-package providing a portable schema-modification
  execution surface: 3-method `SchemaModifier` capability interface
  (`CreateCollection`, `DropCollection`, `AlterCollection`); sealed `AlterOp`
  type with six composable constructors (`AddField`, `DropField`, `ModifyField`,
  `RenameField`, `AddIndex`, `DropIndex`) — all six accept `Option` for
  opt-in idempotency; `TransactionalDDL` capability interface for atomicity
  advertisement; `PartialSuccessError` typed error for non-transactional
  drivers that partway-fail a batch.
- New top-level `dbschema` sub-package (shipped together) providing the
  portable schema-description vocabulary (CollectionDef, FieldDef, IndexDef,
  Type, Precision, DefaultExpr), the `SchemaReader` introspection capability
  interface + helper functions, and the shared `NotSupportedError` typed
  error wrapping `dal.ErrNotSupported`. Callers can do a coarse
  `errors.Is(err, dal.ErrNotSupported)` or extract detail via
  `errors.As(err, &dbschema.NotSupportedError{})`.

This is purely additive — no existing interfaces, types, or behavior change.
External `dal.DB` implementations do NOT need to update; DDL support is
opt-in per driver.
```

## Outstanding Questions

- **Backend identification source.** Resolved at Feature-spec time: helpers populate `Backend` from `db.Adapter().Name()` if `db.Adapter()` returns a non-nil `dal.Adapter`; otherwise leave `Backend` empty. See [`operations/`](operations/README.md) `REQ:not-supported-on-non-implementer` AC-2 and AC-3 for the contract.
- **Option misuse handling.** If a caller passes `IfNotExists()` to `DropCollection` (or `IfExists()` to `CreateCollection`), drivers MUST silently ignore the mismatched option per [`options/`](options/README.md) REQ. Plan decides whether to add a test-level lint warning.

---
*This document follows the https://specscore.md/feature-specification*
