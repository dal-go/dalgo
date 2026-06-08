---
format: https://specscore.md/feature-specification
status: Implemented
---

# Feature: ddl Top-Level Operations

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/ddl/operations?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/ddl/operations?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/ddl/operations?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/ddl/operations?op=request-change) |

**Status:** Implemented
**Source Ideas:** —
**Source Idea:** [`dalgo-schema-modification`](../../../ideas/dalgo-schema-modification.md)
**Parent Feature:** [`ddl`](../README.md)

## Summary

Defines the three top-level helper functions that consumers call to perform DDL operations. Each helper takes a `dal.DB`, type-asserts it against `ddl.SchemaModifier`, and either dispatches to the driver's method or returns `*dbschema.NotSupportedError`.

The helpers are the sanctioned consumer entry point. Callers MAY type-assert against `SchemaModifier` directly, but the helpers produce consistent error envelopes (with `Op` and `Backend` populated from `db.Adapter().Name()`).

Index-level operations (adding or removing indexes after initial collection creation) are NOT separate helpers — they are `AlterOp` values passed to `AlterCollection`. See [`alter-ops/`](../alter-ops/README.md).

## Behavior

### REQ: helper-signatures

The `ddl` package MUST export:

```go
func CreateCollection(ctx context.Context, db dal.DB, c dbschema.CollectionDef, opts ...Option) error
func DropCollection(ctx context.Context, db dal.DB, name string, opts ...Option) error
func AlterCollection(ctx context.Context, db dal.DB, name string, ops ...AlterOp) error
```

Each helper has the exact signature above. `AlterCollection` takes a variadic list of `AlterOp` values; `CreateCollection` and `DropCollection` take a variadic list of `Option` values for idempotency.

#### AC-1: helpers-compile

**Given** a Go program importing `ddl`
**When** the program references `ddl.CreateCollection`, `ddl.DropCollection`, `ddl.AlterCollection`
**Then** the program compiles.

### REQ: dispatch-on-implementer

When `db.(ddl.SchemaModifier)` succeeds, each helper MUST delegate to the corresponding method on the `SchemaModifier`, forwarding `ctx`, the operation-specific argument, and the variadic options/ops. The helper MUST return the error returned by the method (unchanged).

#### AC-1: create-collection-dispatches

**Given** a stub `dal.DB` that also implements `ddl.SchemaModifier` and records every call it receives
**When** the program calls `ddl.CreateCollection(ctx, stub, c, ddl.IfNotExists())`
**Then** the stub's `CreateCollection` method is invoked exactly once with the same `ctx`, the same `c`, and an `opts` slice containing the `IfNotExists` option; the helper returns whatever the stub returned.

#### AC-2: drop-collection-dispatches

**Given** a stub `dal.DB` that implements `ddl.SchemaModifier`
**When** the program calls `ddl.DropCollection(ctx, stub, "users", ddl.IfExists())`
**Then** the stub's `DropCollection` method is invoked once with the name `"users"` and the `IfExists` option.

#### AC-3: alter-collection-dispatches-mixed-ops

**Given** a stub `dal.DB` that implements `ddl.SchemaModifier`
**When** the program calls `ddl.AlterCollection(ctx, stub, "users", ddl.AddField(f), ddl.AddIndex(idx), ddl.DropField("legacy"))`
**Then** the stub's `AlterCollection` method is invoked exactly once with the same `ctx`, the name `"users"`, and an `ops` slice containing the three `AlterOp` values in their original order (field op, index op, field op).

### REQ: not-supported-on-non-implementer

When `db.(ddl.SchemaModifier)` fails (the concrete type does NOT implement `SchemaModifier`), each helper MUST return a non-nil `*dbschema.NotSupportedError` with:
- `Op` set to the operation name (`"CreateCollection"`, `"DropCollection"`, `"AlterCollection"`).
- `Backend` set to `db.Adapter().Name()` if `db.Adapter()` returns a non-nil `dal.Adapter`; otherwise empty string.
- `Reason` set to a fixed phrase indicating the driver does not implement `SchemaModifier`.

`errors.Is(err, dal.ErrNotSupported)` MUST return `true` for the returned error.

#### AC-1: not-implementer-returns-typed-error

**Given** a `dal.DB` stub `db` whose concrete type does NOT implement `ddl.SchemaModifier`
**When** the program calls `ddl.CreateCollection(ctx, db, c)`
**Then** the returned error is non-nil, `errors.Is(err, dal.ErrNotSupported)` is `true`, and `errors.As(err, &ue)` succeeds with `ue.Op == "CreateCollection"`.

#### AC-2: backend-populated-from-adapter

**Given** a `dal.DB` stub whose `Adapter()` returns a non-nil `dal.Adapter` with `Name() == "stub-driver"` AND which does NOT implement `ddl.SchemaModifier`
**When** the program calls `ddl.AlterCollection(ctx, db, "users", ddl.AddField(f))`
**Then** the returned `*NotSupportedError` has `Backend == "stub-driver"` and `Op == "AlterCollection"`.

#### AC-3: backend-empty-when-adapter-nil

**Given** a `dal.DB` stub whose `Adapter()` returns `nil` AND which does NOT implement `ddl.SchemaModifier`
**When** the program calls `ddl.DropCollection(ctx, db, "x")`
**Then** the returned `*NotSupportedError` has `Backend == ""` and `e.Error()` still produces a non-empty readable string.

## Architecture

| File | Contents |
|---|---|
| `ddl/operations.go` | The three top-level helper functions + godoc. |
| `ddl/operations_test.go` | Tests with two stub `dal.DB` types (one satisfies `SchemaModifier`, one does not). |

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
