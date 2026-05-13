# Feature: ddl TransactionalDDL Capability

> [View in SpecStudio](https://specstudio.synchestra.io/project/features?id=dalgo@dal-go@github.com&path=spec%2Ffeatures%2Fddl%2Ftransactional-ddl) — graph, discussions, approvals

**Status:** Approved
**Source Idea:** [`dalgo-schema-modification`](../../../ideas/dalgo-schema-modification.md)
**Parent Feature:** [`ddl`](../README.md)

## Summary

Defines the `TransactionalDDL` capability interface — drivers advertise whether they guarantee all-or-nothing atomicity when a single DDL call performs multiple sub-operations (e.g. `AlterCollection` with multiple `AlterOp` values, or `CreateCollection` with inline indexes). The pattern mirrors [`dal.ConcurrencyAware`](../../concurrency-capability/README.md): a one-method optional interface that consumers type-assert against.

**Default consumer expectation** when the capability is absent or returns `false`: **best-effort with partial success.** The driver MAY succeed at some sub-operations and fail at others; consumers receive a `*ddl.PartialSuccessError` (defined in [`errors/`](../errors/README.md)) describing what was applied. Consumers wanting strict atomicity MUST check the capability before depending on rollback behavior.

## Behavior

### REQ: transactional-ddl-interface

The `ddl` package MUST export the capability interface:

```go
type TransactionalDDL interface {
    // SupportsTransactionalDDL reports whether this driver guarantees
    // all-or-nothing atomicity for DDL calls that perform multiple
    // sub-operations (notably AlterCollection with multiple AlterOps).
    //
    // When true: if any sub-operation fails, the driver MUST roll back
    // all previously-applied sub-operations in the same call. The whole
    // call returns a non-nil error; the DB is left in its pre-call state.
    //
    // When false (or interface not implemented): the driver MAY apply
    // some sub-operations and fail on others. Callers receive a
    // *PartialSuccessError listing applied/failed/not-attempted ops.
    //
    // The return value is constant from the moment a DB value is returned
    // by its constructor until it is discarded; the same stability contract
    // as ConcurrencyAware.
    SupportsTransactionalDDL() bool
}
```

#### AC-1: interface-exists

**Given** a Go program importing `ddl`
**When** the program references `ddl.TransactionalDDL`
**Then** the program compiles and the interface has exactly one method `SupportsTransactionalDDL() bool`.

#### AC-2: db-need-not-satisfy

**Given** a `dal.DB` value `db` whose concrete type does NOT implement `ddl.TransactionalDDL`
**When** the program performs `_, ok := db.(ddl.TransactionalDDL)`
**Then** `ok` is `false` (no panic).

#### AC-3: stable-answer

**Given** any concrete `dal.DB` value `db` that implements `ddl.TransactionalDDL`
**When** `db.SupportsTransactionalDDL()` is called multiple times
**Then** all calls return the same `bool`.

### REQ: helper-function

The `ddl` package MUST export a top-level helper that encapsulates the type assertion and the convention that "doesn't implement = treat as non-transactional":

```go
func SupportsTransactionalDDL(db dal.DB) bool
```

The helper performs `db.(ddl.TransactionalDDL)`; on success it returns the result of the method, on failure it returns `false`.

#### AC-1: helper-true-on-implementer

**Given** a `dal.DB` stub that implements `ddl.TransactionalDDL` and returns `true`
**When** the program calls `ddl.SupportsTransactionalDDL(stub)`
**Then** the result is `true`.

#### AC-2: helper-false-on-non-implementer

**Given** a `dal.DB` stub whose concrete type does NOT implement `ddl.TransactionalDDL`
**When** the program calls `ddl.SupportsTransactionalDDL(stub)`
**Then** the result is `false`.

## Architecture

| File | Contents |
|---|---|
| `ddl/transactional.go` | `TransactionalDDL` interface + `SupportsTransactionalDDL(db)` helper + godoc. |
| `ddl/transactional_test.go` | Tests covering the ACs above. |

## Outstanding Questions

- **Granularity.** This capability is binary: a driver either does or doesn't guarantee transactional DDL across batches. Real engines have nuance (PostgreSQL is transactional for most DDL but not all; MySQL is NOT transactional for DDL even though it has transactions). The MVP picks the simpler shape and trusts drivers to return `false` when they cannot uphold the contract for ALL batched ops. Refining to "transactional for SOME ops but not others" is a follow-up if a real driver needs it.

---
*This document follows the https://specscore.md/feature-specification*
