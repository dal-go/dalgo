---
format: https://specscore.md/feature-specification
status: Implemented
---

# Feature: ddl Applier (Visitor for AlterOp Dispatch)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/ddl/applier?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/ddl/applier?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/ddl/applier?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/ddl/applier?op=request-change) |

**Status:** Implemented
**Source Ideas:** —
**Source Idea:** —
**Parent Feature:** [`ddl`](../README.md)

## Summary

Adds a public `ddl.Applier` interface plus an `ApplyTo(ctx, applier) error` method on each of the six concrete `AlterOp` types, so driver-side schema modifiers can dispatch `AlterOp` values to operation-specific handlers without resorting to type switches against unexported types or reflection.

The current `AlterOp` interface (defined in the [`alter-ops`](../alter-ops/README.md) Feature) is sealed — its concrete implementations (`addFieldOp`, `dropFieldOp`, `modifyFieldOp`, `renameFieldOp`, `addIndexOp`, `dropIndexOp`) are unexported. A driver implementing `SchemaModifier.AlterCollection(ctx, name, ops...)` receives a slice of `AlterOp` values but cannot dispatch on them because the concrete types are inaccessible. This Feature closes that gap with a classic visitor pattern: the driver provides an `Applier` implementation and the AlterOp does the dispatch via `ApplyTo`.

This is a small, additive, non-breaking change. External callers cannot already implement `AlterOp` (it's sealed), so adding a method to the interface affects only the six in-package types — all updated atomically with the new method.

## Problem

The `dalgo2sqlite` driver's `AlterCollection` implementation (specified in `dalgo2sqlite/spec/features/dbschema-ddl-coverage`) is currently blocked: with no public way to dispatch on the concrete `AlterOp` type, the driver has only two paths today, both bad:

1. **Reflection** on the runtime type. Brittle, expensive, and breaks if the unexported type names change.
2. **A bespoke method on the driver's struct** that pattern-matches against constructor-returned types via type assertion — but since the concrete types are unexported, the driver literally cannot write the type-switch arms.

The visitor pattern is the standard fix: `AlterOp` gets a "double-dispatch" method, the driver writes one `Applier` implementation, and the dispatch happens inside `ApplyTo` where the concrete type is in scope.

Beyond `dalgo2sqlite`, the same dispatch problem will hit the upcoming `dalgo2ingitdb` AlterCollection implementation. Solving it once at the dalgo layer unblocks every present and future driver.

## Behavior

### REQ: applier-interface

The `ddl` package MUST export a public interface `type Applier interface { … }` declaring exactly six methods, one per AlterOp variant. Each method takes `ctx context.Context` as its first parameter, the AlterOp-specific operand(s), and the already-resolved `Options` value as the last parameter. Each returns `error`.

The six methods are:

```go
type Applier interface {
    ApplyAddField(ctx context.Context, f dbschema.FieldDef, opts Options) error
    ApplyDropField(ctx context.Context, name dal.FieldName, opts Options) error
    ApplyModifyField(ctx context.Context, name dal.FieldName, newDef dbschema.FieldDef, opts Options) error
    ApplyRenameField(ctx context.Context, oldName, newName dal.FieldName, opts Options) error
    ApplyAddIndex(ctx context.Context, idx dbschema.IndexDef, opts Options) error
    ApplyDropIndex(ctx context.Context, name string, opts Options) error
}
```

The interface is intentionally NOT sealed — drivers (the dalgo2sqlite, dalgo2ingitdb, future dalgo2pg packages) MUST implement it in their own packages. Test stubs (e.g. a recording `fakeApplier`) MAY also implement it.

#### AC-1: interface-exists

**Given** a Go program that imports `github.com/dal-go/dalgo/ddl`
**When** the program declares `var _ ddl.Applier = (*myApplier)(nil)` where `myApplier` provides the six methods with the signatures above
**Then** the program compiles.

#### AC-2: method-signatures-fixed

**Given** the `Applier` interface declaration
**When** inspected via Go reflection in a test
**Then** the method set has exactly the six methods named `ApplyAddField`, `ApplyDropField`, `ApplyModifyField`, `ApplyRenameField`, `ApplyAddIndex`, `ApplyDropIndex` (no more, no fewer); each has `context.Context` as the first parameter and `error` as the return type.

### REQ: apply-to-method

Each of the six concrete `AlterOp` types in `ddl/alter_op.go` (`addFieldOp`, `dropFieldOp`, `modifyFieldOp`, `renameFieldOp`, `addIndexOp`, `dropIndexOp`) MUST gain an `ApplyTo(ctx context.Context, a Applier) error` method that delegates to the matching `Applier` method, passing the op's stored fields and `options` value.

#### AC-1: addFieldOp.ApplyTo-dispatches-correctly

**Given** an `AddField(f, opts...)` AlterOp value `op` and an `Applier` implementation `rec` that records the method name and arguments of each call
**When** `op.ApplyTo(ctx, rec)` is called
**Then** `rec` records exactly one call to `ApplyAddField` with the same `ctx`, the same field `f`, and the resolved `Options` value (matching what `ResolveOptions(opts...)` produced).

#### AC-2: every-AlterOp-type-has-ApplyTo

**Given** the six AlterOp constructors `AddField`, `DropField`, `ModifyField`, `RenameField`, `AddIndex`, `DropIndex`
**When** a test calls each constructor, casts the result to `AlterOp`, and invokes `ApplyTo(ctx, rec)` on a recording `Applier`
**Then** each invocation produces exactly one call to the matching `ApplyXxx` method on `rec`, with the expected arguments forwarded.

#### AC-3: ApplyTo-propagates-error

**Given** an `AlterOp` value and an `Applier` whose corresponding `ApplyXxx` method returns a sentinel error
**When** `op.ApplyTo(ctx, applier)` is called
**Then** the returned error is the same sentinel (identity via `errors.Is` is sufficient).

### REQ: alter-op-interface-extended

The sealed `AlterOp` interface (defined in the [`alter-ops`](../alter-ops/README.md) Feature) MUST gain `ApplyTo(ctx context.Context, a Applier) error` as a second method alongside the existing unexported `alterOp()` marker:

```go
type AlterOp interface {
    alterOp() // sealed marker (existing)
    ApplyTo(ctx context.Context, a Applier) error  // NEW
}
```

This is safe because the existing seal (`alterOp()` is unexported) prevents external packages from having implemented `AlterOp` — only the six in-package types ever satisfied it, and all six gain `ApplyTo` in REQ:apply-to-method.

#### AC-1: AlterOp-interface-has-ApplyTo

**Given** the `ddl.AlterOp` interface
**When** inspected via `reflect.TypeOf((*ddl.AlterOp)(nil)).Elem()` and its method set is iterated
**Then** the method set includes a method named `ApplyTo` whose signature is `func(context.Context, Applier) error`.

#### AC-2: driver-can-dispatch-via-interface

**Given** a slice `ops []ddl.AlterOp` returned from constructors (e.g. `[]ddl.AlterOp{ddl.AddField(f), ddl.DropIndex("ix")}`)
**When** a driver calls `op.ApplyTo(ctx, applier)` on each element without any type assertion
**Then** the calls compile and run, dispatching to the appropriate `ApplyXxx` method on the driver's `Applier`.

### REQ: applier-callable-from-AlterCollection

The existing `SchemaModifier.AlterCollection(ctx, name, ops...)` implementation pattern in drivers SHOULD use `op.ApplyTo(ctx, driverApplier)` for dispatch. This Feature does NOT change any existing dalgo signatures — `AlterCollection` keeps its current shape. The new dispatch surface is purely additive on `AlterOp`.

#### AC-1: no-AlterCollection-signature-change

**Given** the `ddl.SchemaModifier.AlterCollection(ctx context.Context, name string, ops ...AlterOp) error` method signature shipped by the [`schema-modifier`](../schema-modifier/README.md) Feature
**When** this Feature is implemented
**Then** the `AlterCollection` signature is identical to its pre-Feature form. Drivers that don't use `Applier` continue to work without modification.

## Architecture

### Files

| File | Change |
|---|---|
| `ddl/applier.go` | New file. Declares the `Applier` interface with the six `ApplyXxx` methods. Documentation comment explains the visitor-pattern intent and the relationship to `AlterOp.ApplyTo`. |
| `ddl/alter_op.go` | Modify the `AlterOp` interface to add the `ApplyTo(ctx context.Context, a Applier) error` method. Add an `ApplyTo` method on each of the six concrete op types: `addFieldOp`, `dropFieldOp`, `modifyFieldOp`, `renameFieldOp`, `addIndexOp`, `dropIndexOp`. |
| `ddl/applier_test.go` | New file. Tests for the dispatch contract: each AlterOp constructor produces a value whose `ApplyTo` calls the right `ApplyXxx` method with the right arguments. Use an in-test `recordingApplier` struct that captures method name + args. |
| `ddl/alter_op.go` (existing tests) | No test code changes required; existing tests don't break (additive method). Lint may flag missing test coverage for new methods — `applier_test.go` covers it. |

### Visitor-pattern dispatch

```
caller                                  AlterOp interface         concrete op type           Applier (driver impl)
  │                                           │                          │                          │
  │  AlterCollection(ctx, name, ops...)       │                          │                          │
  ├──────────────────────────────────────────▶│                          │                          │
  │                                           │                          │                          │
  │  for op := range ops:                     │                          │                          │
  │     op.ApplyTo(ctx, driverApplier)        │                          │                          │
  ├──────────────────────────────────────────▶│                          │                          │
  │                                           │                          │                          │
  │                                           │  dispatched to concrete  │                          │
  │                                           │─────────────────────────▶│                          │
  │                                           │                          │  applier.ApplyXxx(ctx, ...)
  │                                           │                          ├─────────────────────────▶│
  │                                           │                          │                          │
  │                                           │                          │                          │ (executes SQL, returns)
  │                                           │                          │◀─────────────────────────│
  │                                           │◀─────────────────────────│                          │
  │◀──────────────────────────────────────────│                          │                          │
```

## Data Flow

A driver's `AlterCollection` (using the visitor):

```go
// dalgo2sqlite/schema_modifier.go (illustrative)
func (d *Database) AlterCollection(ctx context.Context, name string, ops ...ddl.AlterOp) error {
    return d.inTx(ctx, func(tx *sql.Tx) error {
        applier := &sqliteAlterApplier{tx: tx, table: name}
        for _, op := range ops {
            if err := op.ApplyTo(ctx, applier); err != nil {
                return err  // transaction rolls back via inTx
            }
        }
        return nil
    })
}

type sqliteAlterApplier struct {
    tx    *sql.Tx
    table string
}

func (a *sqliteAlterApplier) ApplyAddField(ctx context.Context, f dbschema.FieldDef, opts ddl.Options) error {
    /* build + exec ALTER TABLE … ADD COLUMN … */
}
// ... five more methods
```

## Error Handling & Failure Modes

| Failure | Surfaced via | Resolution |
|---|---|---|
| Driver's `ApplyXxx` returns an error | `ApplyTo` returns the same error verbatim (no wrapping). Caller (`AlterCollection`) surfaces it to the user. | Caller-defined; transactional drivers roll back. |
| Caller passes an `AlterOp` not produced by one of the six constructors | Impossible: `AlterOp` is sealed (unexported `alterOp()` marker). | N/A. |
| Caller passes `nil` Applier to `ApplyTo` | Concrete types invoke a method on `nil` interface, producing a `runtime` panic (`nil pointer dereference`). | This is acceptable failure-fast behavior; callers MUST provide a non-nil Applier. A check `if a == nil { return …error }` is NOT added — it would slow the hot path for no realistic benefit. |
| Driver doesn't implement all six methods | Compile error. | N/A. |

## Testing Strategy

In-package Go tests in `ddl/applier_test.go`:

- A `recordingApplier` struct that implements `Applier` by appending each call (method name + args) to a slice. Tests assert the slice contents after calling `ApplyTo`.
- Six positive tests, one per AlterOp variant, asserting the right `ApplyXxx` method is called with the right arguments.
- One error-propagation test: a `failingApplier` returns a sentinel error; `ApplyTo` MUST return the same error.
- One reflection-based test asserting `AlterOp` interface's method set contains `ApplyTo` with the right signature.

No driver integration is required at this layer — driver-side dispatch is verified by the consuming Feature (`dalgo2sqlite` AlterCollection round-trip test).

## Rehearse Integration

All ACs are testable via `go test ./ddl/...`. No external scaffolding needed.

## Out of Scope

- **Sealing the Applier interface.** External drivers MUST be able to implement `Applier`. The interface is intentionally open.
- **Default no-op Applier.** Not provided. Drivers that genuinely want a no-op (e.g. a discovery/recording-only test stub) write their own one-liner per method.
- **Async/streaming dispatch.** `ApplyTo` is synchronous and returns a single `error`. No future, no callback. The driver decides whether to parallelize internally.
- **Renaming existing AlterOp constructors.** The public constructor names `AddField`, `DropField`, etc., stay as-is. The new method names use the `Apply` prefix to avoid collision.
- **Adding new AlterOp variants.** This Feature touches only the existing six. New AlterOp variants (e.g. `SetTableProperty`, `ChangeOrderingKey`) are separate future Features that would also extend the `Applier` interface and add an `ApplyTo` method on the new concrete type.
- **Removing the `alterOp()` sealed marker.** Marker stays. The interface gains a public `ApplyTo` method alongside it, but remains sealed against external implementations.

## Assumption Carryover

No source Idea exists. The implicit assumptions this Feature commits to:

| Tier | Assumption | Status |
|------|------------|--------|
| Must-be-true | The `AlterOp` interface is sealed (`alterOp()` unexported) so no external implementations exist | Verified: see [`alter-ops`](../alter-ops/README.md) Feature, Status: Implemented. Adding `ApplyTo` is safe. |
| Must-be-true | `dbschema.FieldDef`, `dbschema.IndexDef`, `dal.FieldName`, and `Options` are all importable in driver packages | Verified: all are exported types from `dal-go/dalgo` already consumed by drivers. |
| Must-be-true | The six AlterOp types' stored fields contain everything an Applier method needs | Plan-time check: each concrete type stores its constructor's parameters plus the `options Options` field. Audit confirms. |
| Should-be-true | The visitor pattern is the right shape for this dispatch problem (vs. a generic `Apply(op AlterOp) error` method that type-switches internally) | Carried; the visitor pattern's appeal is that the driver writes one method per op and the compiler checks all six are present. The generic `Apply` alternative pushes type assertion into the driver, which is exactly what we're solving. |
| Might-be-true | Future AlterOp additions (if any) will continue to follow this pattern | Deferred; if proven wrong, the Applier interface can be replaced (it's not sealed externally — but in practice drivers implement it, which is the surface that matters). |

## Open Questions

- **`ctx context.Context` as first parameter vs. struct-stored.** The Feature pins `ctx` as the FIRST parameter of every `Apply*` method (and of `ApplyTo`). Rationale: idiomatic for a published interface other packages will implement; matches stdlib patterns (e.g. `database/sql.DB.QueryContext`); makes test stubs trivial; aligns with Go vet's `contextcheck`. The alternative — storing `ctx` on the driver's adapter struct and having Apply methods take no ctx — is a recognized pattern for short-lived adapters (the `sqliteAlterApplier` sketch in `dalgo2sqlite/spec/plans/2026-05-13-dbschema-ddl-coverage.md` uses it). When the dalgo2sqlite plan executes Tasks 20–21, its `sqliteAlterApplier` struct MUST be updated to match this spec's contract (ctx-in-signature). The plan's pre-flight sketch is illustrative and predates this spec; reconciliation is a plan-time concern.
- **Variadic `...Option` vs resolved `Options` in `Apply*` method signatures.** The Feature pins the resolved-`Options` form. Rationale: each concrete op already calls `ResolveOptions(opts...)` once at construction and stores the result, so the driver sees the resolved view. Open question: should the dispatch instead pass `[]Option` (raw) so the Applier can re-extract or wrap? Plan-time decision: stick with resolved `Options` value unless a real consumer needs the raw form.
- **Should `Applier` also be in its own subpackage (`ddl/applier`) rather than at the `ddl` root?** Plan-time decision: keep in `ddl` package because the visitor pattern is tightly coupled to the AlterOp types defined there. A subpackage would force an import cycle or awkward bridging. The `ddl` root is the right home.

---

*This document follows the https://specscore.md/feature-specification*
