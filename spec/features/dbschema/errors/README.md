---
format: https://specscore.md/feature-specification
status: Implemented
---

# Feature: dbschema NotSupportedError

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/errors?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/errors?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/errors?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dbschema/errors?op=request-change) |

**Status:** Implemented
**Source Ideas:** —
**Source Idea:** [`dalgo-schema-modification`](../../../ideas/dalgo-schema-modification.md)
**Parent Feature:** [`dbschema`](../README.md)

## Summary

Defines the typed error `NotSupportedError` that wraps the existing `dal.ErrNotSupported` sentinel. Used by BOTH the read side (`dbschema.SchemaReader` and helper functions) AND the write side (`ddl.SchemaModifier` and helper functions). Callers can do a coarse `errors.Is(err, dal.ErrNotSupported)` (works across ALL DALgo not-supported failures) OR extract detail via `errors.As(err, &dbschema.NotSupportedError{})`.

This Feature lives in `dbschema` (not in `ddl`) because `dbschema` is the more foundational package — `ddl` imports `dbschema` and reuses the same typed error for symmetry.

## Behavior

### REQ: not-supported-error-struct

The `dbschema` package MUST export:

```go
type NotSupportedError struct {
    Op      string  // e.g. "CreateCollection", "DescribeCollection", "DropIndex"
    Backend string  // e.g. "dalgo2sql/sqlite"; may be empty
    Reason  string  // optional human explanation
}

func (e *NotSupportedError) Error() string { … }
func (e *NotSupportedError) Unwrap() error { return dal.ErrNotSupported }
```

`Unwrap()` returns the existing `dal.ErrNotSupported` sentinel so `errors.Is(err, dal.ErrNotSupported)` succeeds.

#### AC-1: struct-fields

**Given** a Go program importing `dbschema`
**When** the program declares `e := dbschema.NotSupportedError{Op: "CreateCollection", Backend: "dalgo2sql/sqlite", Reason: "read-only"}`
**Then** the program compiles and all three fields are accessible.

#### AC-2: error-method

**Given** a `*dbschema.NotSupportedError` `e` with `Op: "CreateCollection"`, `Backend: "dalgo2sql/sqlite"`, `Reason: "read-only mode"`
**When** `e.Error()` is called
**Then** the returned string is non-empty and contains the substrings `"CreateCollection"`, `"dalgo2sql/sqlite"`, and `"read-only mode"` in a readable single-line format.

#### AC-3: error-with-empty-fields

**Given** a `*dbschema.NotSupportedError` `e` with `Op: "DescribeCollection"`, `Backend: ""`, `Reason: ""`
**When** `e.Error()` is called
**Then** the returned string is non-empty and contains `"DescribeCollection"` and does NOT panic or include the empty fields verbatim.

### REQ: unwrap-to-sentinel

`*NotSupportedError` MUST unwrap to `dal.ErrNotSupported`. Callers using `errors.Is` for a coarse check MUST succeed.

#### AC-1: errors-is-succeeds

**Given** an error `err := &dbschema.NotSupportedError{Op: "CreateCollection"}`
**When** `errors.Is(err, dal.ErrNotSupported)` is called
**Then** the result is `true`.

#### AC-2: errors-as-succeeds

**Given** an error `err := &dbschema.NotSupportedError{Op: "DropIndex", Backend: "x"}` returned as `error`
**When** the caller runs `var ue *dbschema.NotSupportedError; ok := errors.As(err, &ue)`
**Then** `ok` is `true`, `ue.Op == "DropIndex"`, `ue.Backend == "x"`.

### REQ: shared-with-ddl

The `ddl` package MUST import and reuse `dbschema.NotSupportedError` (not define its own). The `ddl` helper functions (`CreateCollection`, etc.) return `*dbschema.NotSupportedError` for the type-assertion-failed case.

#### AC-1: ddl-uses-shared-type

**Given** a Go program that calls `ddl.CreateCollection(ctx, db, c)` against a `db` that does NOT implement `ddl.SchemaModifier`
**When** the returned error is inspected via `errors.As(err, &target)` with `var target *dbschema.NotSupportedError`
**Then** `errors.As` returns `true`.

## Architecture

| File | Contents |
|---|---|
| `dbschema/errors.go` | `NotSupportedError` struct + `Error()` + `Unwrap()` + godoc. |
| `dbschema/errors_test.go` | Tests covering the ACs above. |

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
