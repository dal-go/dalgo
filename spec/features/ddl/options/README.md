# Feature: ddl Options (`IfNotExists`, `IfExists`)

> [View in SpecStudio](https://specstudio.synchestra.io/project/features?id=dalgo@dal-go@github.com&path=spec%2Ffeatures%2Fddl%2Foptions) — graph, discussions, approvals

**Status:** Approved
**Source Idea:** [`dalgo-schema-modification`](../../../ideas/dalgo-schema-modification.md)
**Parent Feature:** [`ddl`](../README.md)

## Summary

Defines the functional-options pattern for opt-in idempotent semantics. `Option` and the constructors `IfNotExists()` / `IfExists()` are shared across:

- **Collection-level methods**: `CreateCollection(c, opts ...Option)` and `DropCollection(name, opts ...Option)`.
- **All six AlterOps**: `AddField`, `DropField`, `ModifyField`, `RenameField`, `AddIndex`, `DropIndex` — each accepts `opts ...Option`.

By default, every operation is **strict** (errors when the target exists for Add/Create, errors when the target is absent for Drop). Passing `IfNotExists()` to an Add/Create operation OR `IfExists()` to a Drop operation makes that operation idempotent (driver treats the mismatched state as a no-op). Drivers MUST silently ignore semantically-mismatched options (e.g. `IfNotExists()` on `DropField`, or any option on `ModifyField`/`RenameField`).

## Behavior

### REQ: option-type

The `ddl` package MUST export:

```go
type Options struct {
    IfNotExists bool
    IfExists    bool
}
type Option func(*Options)
```

`Options` is the resolved options struct; `Option` is the functional-option type accepted by `CreateCollection`, `DropCollection`, and all six `AlterOp` constructors via variadic `opts ...Option`. `AlterCollection` itself accepts `AlterOp` values (not `Option` directly) — but each individual AlterOp carries its own resolved `Options` set by the caller via the constructor.

#### AC-1: option-type-compiles

**Given** a Go program importing `ddl`
**When** the program declares `var fn ddl.Option` and `var o ddl.Options`
**Then** the program compiles.

### REQ: option-constructors

The package MUST export:

```go
func IfNotExists() Option { return func(o *Options) { o.IfNotExists = true } }
func IfExists() Option    { return func(o *Options) { o.IfExists = true } }
```

#### AC-1: if-not-exists-sets-flag

**Given** a Go program importing `ddl`
**When** the program constructs `opts := ddl.Options{}` and applies `ddl.IfNotExists()(&opts)`
**Then** `opts.IfNotExists == true && opts.IfExists == false`.

#### AC-2: if-exists-sets-flag

**Given** a Go program importing `ddl`
**When** the program constructs `opts := ddl.Options{}` and applies `ddl.IfExists()(&opts)`
**Then** `opts.IfExists == true && opts.IfNotExists == false`.

#### AC-3: options-are-independent

**Given** a Go program importing `ddl`
**When** the program applies both `ddl.IfNotExists()` and `ddl.IfExists()` to the same `Options{}`
**Then** both flags end up `true` (no mutual exclusion enforced at the option layer).

### REQ: option-misuse-tolerance

The package documentation MUST note the mismatched-option semantics across the full surface:

- `IfNotExists()` on a Drop operation (`DropCollection`, `DropField`, `DropIndex`) is meaningless — silently ignored.
- `IfExists()` on a Create/Add operation (`CreateCollection`, `AddField`, `AddIndex`) is meaningless — silently ignored.
- Any `Option` on `ModifyField` or `RenameField` is meaningless — silently ignored. (Idempotency for modify/rename is conceptually unclear; if the target field is gone, you typically want a real error.)

Drivers MUST silently ignore the mismatched option rather than error — the meaning is unambiguous (there is nothing to do with a mismatched hint).

#### AC-1: godoc-mentions-tolerance

**Given** the `ddl` package
**When** inspected via `go doc github.com/dal-go/dalgo/ddl.IfNotExists` and `go doc github.com/dal-go/dalgo/ddl.IfExists`
**Then** the doc blocks mention that the mismatched-operation case is silently ignored by drivers.

## Architecture

| File | Contents |
|---|---|
| `ddl/options.go` | `Option`, `Options`, `IfNotExists`, `IfExists` + godoc. |
| `ddl/options_test.go` | Tests covering the ACs above. |

## Outstanding Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
