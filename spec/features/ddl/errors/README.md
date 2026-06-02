# Feature: ddl PartialSuccessError

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/ddl/errors?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/ddl/errors?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/ddl/errors?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/ddl/errors?op=request-change) |

**Status:** Implemented
**Source Idea:** [`dalgo-schema-modification`](../../../ideas/dalgo-schema-modification.md)
**Parent Feature:** [`ddl`](../README.md)

## Summary

Defines the `PartialSuccessError` typed error returned by `AlterCollection` (and any future batched DDL call) when a non-transactional driver succeeds at some sub-operations and fails at others. Distinct from [`dbschema.NotSupportedError`](../../dbschema/errors/README.md) — `NotSupportedError` is "the driver can't do this at all"; `PartialSuccessError` is "the driver started doing this, then failed partway."

Consumers that want to avoid partial-success behavior MUST check the [`TransactionalDDL`](../transactional-ddl/README.md) capability before calling `AlterCollection` with multiple `AlterOp` values; transactional drivers never produce `*PartialSuccessError`.

## Behavior

### REQ: partial-success-error-struct

The `ddl` package MUST export:

```go
type PartialSuccessError struct {
    Op           string     // e.g. "AlterCollection"
    Collection   string     // the target collection name
    Backend      string     // e.g. "dalgo2sql/sqlite"; may be empty
    Applied      []AlterOp  // ops the driver confirmed applied (in original order)
    FirstFailed  AlterOp    // the op that failed
    NotAttempted []AlterOp  // ops the driver did not try (in original order)
    Cause        error      // the underlying driver error from the failed op
}

func (e *PartialSuccessError) Error() string { … }
func (e *PartialSuccessError) Unwrap() error { return e.Cause }
```

#### AC-1: struct-fields

**Given** a Go program importing `ddl`
**When** the program declares a `*PartialSuccessError` populating all six fields
**Then** the program compiles and all fields are accessible.

#### AC-2: error-method

**Given** a `*ddl.PartialSuccessError` with `Op: "AlterCollection"`, `Collection: "users"`, `Backend: "dalgo2sql/sqlite"`, `Applied` slice of length 2, `FirstFailed` non-nil, `NotAttempted` slice of length 1, `Cause: <some error>`
**When** `e.Error()` is called
**Then** the returned string is non-empty, contains `"AlterCollection"`, `"users"`, and a count/summary that distinguishes applied / failed / not-attempted

#### AC-3: unwrap-to-cause

**Given** a `*ddl.PartialSuccessError` with `Cause: someErr`
**When** `errors.Unwrap(err)` is called
**Then** the result is `someErr`.

#### AC-4: errors-is-via-cause

**Given** a `*ddl.PartialSuccessError` whose `Cause` is or wraps `dal.ErrNotSupported`
**When** `errors.Is(err, dal.ErrNotSupported)` is called
**Then** the result is `true` (transitively through `Unwrap`).

### REQ: producer-contract

Drivers that opt out of transactional DDL (i.e. `SupportsTransactionalDDL` returns `false` OR the interface is not implemented) and fail partway through an `AlterCollection` call MUST return `*PartialSuccessError` populated as follows:

- `Op = "AlterCollection"`
- `Collection` = the target name passed by the caller
- `Backend` = `db.Adapter().Name()` if `Adapter()` returns non-nil; otherwise empty string
- `Applied` = the ops that completed successfully, in their original order
- `FirstFailed` = the op that failed
- `NotAttempted` = the ops that came after `FirstFailed` and were not tried (may be empty if the driver attempts every op regardless of earlier failures — implementation detail)
- `Cause` = the underlying error from the failed op (driver-specific)

Drivers that DO guarantee transactional DDL (`SupportsTransactionalDDL` returns `true`) MUST NOT produce `*PartialSuccessError`. Their failure mode is a regular `error` (rollback already performed; nothing was applied).

#### AC-1: applied-list-preserves-order

**Given** a non-transactional driver that applies ops A and B successfully then fails on C in a call `AlterCollection(ctx, db, "t", A, B, C, D)`
**When** the driver returns `*PartialSuccessError`
**Then** `Applied = [A, B]` in that order, `FirstFailed = C`, and `NotAttempted` is either `[D]` (driver short-circuits) or `[]` (driver attempts D too — in which case D's outcome is separately encoded; documented per-driver).

## Architecture

| File | Contents |
|---|---|
| `ddl/errors.go` | `PartialSuccessError` struct + `Error()` + `Unwrap()` + godoc. |
| `ddl/errors_test.go` | Tests covering the ACs above. |

## Open Questions

- **Multiple failures.** If a non-transactional driver attempts every op regardless of earlier failures, multiple ops may fail. The MVP shape only records the FIRST failure (`FirstFailed`) and folds the rest into the driver-specific `Cause`. Refining to a `Failures []FailureRecord` slice is a follow-up if a real driver needs to surface multi-failure detail.
- **Relationship to `dal.ErrNotSupported`.** If the underlying `Cause` is `dal.ErrNotSupported` (e.g. SQLite can't drop a column), `errors.Is(err, dal.ErrNotSupported)` returns `true` via `Unwrap`. Consumers can therefore use a coarse not-supported check across all DDL paths.

---
*This document follows the https://specscore.md/feature-specification*
