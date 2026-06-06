---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Computed Columns in recordset (neutral evaluator contract)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/recordset-computed-columns?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/recordset-computed-columns?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/recordset-computed-columns?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/recordset-computed-columns?op=request-change) |
**Status:** Approved
**Date:** 2026-06-02
**Owner:** alex
**Source Ideas:** recordset-computed-columns
**Supersedes:** —

## Summary

Gives dalgo's `recordset` package a backend-agnostic notion of a *computed
column*: a column whose value is derived at access time by an opaque
`Evaluator`, rather than read from stored cell storage. It serves dalgo
consumers (first: ingitdb-cli) that want derived column values without dalgo
taking a dependency on any formula language or scripting runtime.

## Problem

`recordset` columns are purely stored today: `Row.GetValueByName` resolves a
name to a `Column[any]` and returns `col.GetValue(rowIndex)` from that column's
`values []T` slice. There is no way to expose a column whose value is computed
from the row's other fields. Consumers that need derived values (ingitdb-cli's
inline Starlark formulas) must compute outside the recordset and bake results
into stored cells. dalgo should own the *wiring* — a neutral evaluator contract
and lazy resolution through the existing `Row` interface — while the formula
language stays entirely in the consumer.

A structural constraint shapes the design: `Column[T].GetValue(row int)`
receives only a row index, with no access to sibling columns or the
`Recordset`. A computed column therefore cannot resolve itself in isolation;
resolution must happen at the `Row` level, which holds the `Recordset` and can
gather the row's other column values for the evaluator.

## Behavior

### Evaluator contract

The neutral boundary between dalgo and the consumer's formula engine.

#### REQ: evaluator-interface

The `recordset` package MUST export an `Evaluator` interface with a single
method `Eval(stored map[string]any) (value any, err error)`. The `recordset`
package MUST NOT import or depend on any formula language, expression engine,
or scripting runtime; it only invokes an opaque `Evaluator` supplied by the
caller.

### Computed column definition

How a computed column is represented and registered on a Recordset.

#### REQ: computed-column-marker

The `recordset` package MUST export a `ComputedColumn` interface composed of
`Column[any]` plus an `Evaluator() Evaluator` accessor, and a constructor that
produces a `Column[any]` implementing `ComputedColumn` from a name and an
`Evaluator`. A computed column MUST be registerable on a `Recordset` through the
same path as a stored column (i.e. as a `Column[any]`), and MUST be detectable
by a type assertion to `ComputedColumn`. The existing `Column[T]` interface and
all existing column types MUST be left unchanged (the marker is additive).

#### REQ: reject-direct-set

Setting a value on a computed column MUST fail: `SetValueByName` and
`SetValueByIndex` targeting a computed column MUST return an error and MUST NOT
alter any subsequent computed result. The rejection MUST be enforced at the
`Row` (`columnarRow`) layer — the row detects the `ComputedColumn` and returns
the error *before* delegating to the column's own `SetValue` — so enforcement
does not depend on the computed column's underlying stored-cell behavior. A
computed value is derived, never stored.

### Lazy resolution

How a computed value is produced when a Row is read.

#### REQ: lazy-eval-on-access

When `Row.GetValueByName` or `Row.GetValueByIndex` targets a computed column,
the `Row` MUST resolve the value by invoking the column's `Evaluator`, and MUST
do so only at access time — never eagerly when the row is created or when other
columns are read.

#### REQ: stored-fields-only-input

The `stored` map the `Row` passes to `Evaluator.Eval` MUST contain the values of
the row's non-computed columns, keyed by column name. It MUST NOT contain any
computed column (including the one being resolved), so a formula can reference
only stored sibling fields (no chained computed columns in this Feature).

#### REQ: memoize-per-row

For a given `Row` instance and computed column, the `Evaluator` MUST be invoked
at most once. The resolved outcome — the value, or the error — MUST be cached on
that `Row` instance and returned on every subsequent access of that computed
column. The memo is scoped to the `Row` *instance*, not the row index: because
`Recordset.GetRow(i)` returns a freshly allocated `Row` on each call, two `Row`
values for the same index each maintain their own cache and may each invoke the
`Evaluator` once.

#### REQ: data-resolves-computed

`Row.Data(rs)`, which materializes all column values for the row, MUST resolve
computed columns through the same lazy + memoized path as `GetValueByIndex`
(not by reading a computed column's unused stored cell). If any computed
column's `Evaluator` returns an error, `Data` MUST return that error.

#### REQ: propagate-eval-error

If the `Evaluator` returns an error, that error MUST be returned through the
existing `(any, error)` return of `GetValueByName` / `GetValueByIndex`, with a
nil value. Resolution MUST NOT panic and MUST NOT return a partial or fabricated
value (fail-loud at access time).

### Scope boundary

What this Feature deliberately does not transform.

#### REQ: no-coercion

`GetValueByName` / `GetValueByIndex` MUST return the `Evaluator`'s result
unchanged (as `any`). This Feature performs no coercion of the computed value to
a declared column type; that concern stays with the consumer.

## Acceptance Criteria

### AC: evaluator-compiles (verifies REQ:evaluator-interface)

**Given** a Go program importing `recordset`
**When** the program declares a type with method `Eval(stored map[string]any) (any, error)` and assigns it to a `recordset.Evaluator` variable
**Then** the program compiles.

### AC: recordset-has-no-scripting-dependency (verifies REQ:evaluator-interface)

**Given** the `recordset` package source and its import set
**When** the imports are inspected
**Then** no formula-language, expression-engine, or scripting-runtime package (e.g. a Starlark interpreter) appears among them.

### AC: computed-column-registers-and-marks (verifies REQ:computed-column-marker)

**Given** a `Recordset` constructed with a stored column `qty` and a computed column `label` built from an `Evaluator` `e`
**When** `GetColumnByName("label")` is called and the result is type-asserted to `recordset.ComputedColumn`
**Then** the assertion succeeds and the asserted value's `Evaluator()` is `e`, while `GetColumnByName("qty")` does not satisfy the `ComputedColumn` assertion.

### AC: reject-set-on-computed (verifies REQ:reject-direct-set)

**Given** a row with a computed column `full`
**When** `SetValueByName("full", "x", rs)` is called
**Then** it returns an error, and a subsequent `GetValueByName("full", rs)` returns the evaluator-derived value (not `"x"`).

### AC: lazy-resolves-from-siblings (verifies REQ:lazy-eval-on-access)

**Given** a `Recordset` whose row 0 has stored `first = "Ada"` and `last = "Lovelace"`, and a computed column `full` whose `Evaluator` returns `stored["first"] + " " + stored["last"]`
**When** `GetValueByName("full", rs)` is called on row 0
**Then** it returns `"Ada Lovelace"` with no error.

### AC: input-excludes-computed (verifies REQ:stored-fields-only-input)

**Given** a `Recordset` with stored columns `first` and `last` and two computed columns `full` and `greeting`, where `full`'s `Evaluator` records the keys of the `stored` map it receives
**When** `GetValueByName("full", rs)` is called
**Then** the recorded keys contain exactly `first` and `last` — including every stored column and excluding both `full` and `greeting`.

### AC: memoized-single-eval (verifies REQ:memoize-per-row)

**Given** a computed column whose `Evaluator` increments a call counter and returns a fixed value
**When** the same row's computed value is read three times
**Then** the counter equals `1` and all three reads return the same value.

### AC: eval-error-propagates-and-caches (verifies REQ:propagate-eval-error)

**Given** a computed column whose `Evaluator` returns an error and increments a call counter
**When** the computed value is accessed twice
**Then** each access returns that error with a nil value and no panic, and the counter equals `1` (the cached error is reused).

### AC: data-materializes-computed (verifies REQ:data-resolves-computed)

**Given** a `Recordset` whose row 0 has stored `first = "Ada"`, `last = "Lovelace"`, and a computed column `full` returning `stored["first"] + " " + stored["last"]`
**When** `row.Data(rs)` is called for row 0
**Then** the returned slice contains `"Ada Lovelace"` in the `full` column's position (the computed value, not a stored-cell default).

### AC: returns-raw-uncoerced-value (verifies REQ:no-coercion)

**Given** a computed column whose `Evaluator` returns the `int` value `42`
**When** `GetValueByName` is called for that column
**Then** the returned value is the `int` `42`, unchanged and without coercion error.

## Architecture & Components

All changes are additive and confined to the `recordset` package.

| Unit | What it does | Depends on |
|------|--------------|-----------|
| `Evaluator` interface | Neutral compiled-formula contract: `Eval(stored map[string]any) (any, error)`. | nothing |
| `ComputedColumn` interface | `Column[any]` + `Evaluator() Evaluator`. Marker for detection via type assertion. | `Column[any]`, `Evaluator` |
| `NewComputedColumn(name, evaluator, …)` | Constructs a `Column[any]` that also implements `ComputedColumn`. Its stored-cell storage is unused. | `columnBase`, `Evaluator` |
| `columnarRow` resolution | `GetValueByName`/`GetValueByIndex`/`Data` branch: if a resolved column is a `ComputedColumn`, gather the row's stored (non-computed) columns into a `map[string]any`, invoke the evaluator, memoize the outcome on the row instance, and return it; otherwise the existing stored path is unchanged. `SetValueByName`/`SetValueByIndex` reject computed columns at this layer. | `Recordset`, `ColumnAccessor`, `Evaluator` |
| per-row memo cache | A small per-row store of resolved `(value, error)` outcomes keyed by computed-column name. | `columnarRow` |

## Data Flow

1. A consumer compiles each formula once into a value implementing `Evaluator`
   and registers it as a `NewComputedColumn` alongside stored columns on a
   `Recordset`.
2. On `Row.GetValueByName(name, rs)`: resolve `col := rs.GetColumnByName(name)`.
3. If `col` is not a `ComputedColumn` → existing path (`col.GetValue(rowIndex)`).
4. If `col` is a `ComputedColumn` → consult the row's memo cache; on miss, build
   `stored` by iterating `rs` columns that are not computed, reading each via the
   row, keyed by column name; call `col.Evaluator().Eval(stored)`; store the
   `(value, error)` outcome in the cache; return it.
5. `GetValueByIndex` follows the same branch after resolving the column by index.
6. `Data(rs)` materializes every column for the row, routing each computed column
   through the same memoized branch (steps 3–4) rather than its stored cell.

## Error Handling & Failure Modes

- **Evaluator error** → returned verbatim through `(any, error)` with a nil
  value (REQ:propagate-eval-error); cached so re-access does not re-invoke
  (REQ:memoize-per-row).
- **Unknown column name** → unchanged from today: `GetValueByName` returns the
  existing "unexpected column name" error.
- **Set on a computed column** → error, no state change (REQ:reject-direct-set).
- **No panics** on any resolution path.

## Testing Strategy

Pure-Go unit tests in `recordset` using a fake `Evaluator` (a struct whose
`Eval` is a closure) — no scripting engine in dalgo's test suite. Tests cover
each AC: compile check, import-set check, marker detection, sibling-only input,
lazy resolution, memoization call-count, error propagation + error caching,
set-rejection, and raw-value pass-through. Construction follows the existing
`NewColumnarRecordset(name, cols…)` + `NewRow()` test idiom.

## Rehearse Integration

No `_tests/` Rehearse stubs. Per this repo's established convention
(`dbschema`, `transaction-message`), every AC has a pure-Go surface verified by
standard `*_test.go` tests, making separate Rehearse scenario files redundant.

## Not Doing / Out of Scope

- Embedding any formula language or scripting runtime in dalgo (neutral
  `Evaluator` only).
- Coercing the computed value to a declared column type (REQ:no-coercion).
- Chained computed columns — a formula referencing another computed column
  (REQ:stored-fields-only-input excludes computed inputs).
- Filtering/sorting on computed columns and foreign-key checks — remain on the
  consumer side (Phase B / ingitdb), not part of this Feature.
- Implementing or migrating ingitdb's `ExecuteQueryToRecordsetReader` — that is
  Phase B in the source Idea, a separate Feature.
- Thread-safe/concurrent resolution — memoization is per-row, single-threaded
  read (see Outstanding Questions).

## Assumption Carryover

From the source Idea `recordset-computed-columns`:

- **Validated and embodied here:** resolution lives on the `Row` (the carrier is
  `recordset.Row`); the evaluator receives the full stored-field map; lazy +
  memoized evaluation; dalgo carries no scripting dependency.
- **Deferred to Phase B (out of this Feature):** ingitdb's `select` flows
  through `dal.Record` today and its recordset query path is a stub; adopting
  this contract and flipping eager→lazy is Phase B.
- **Now answered (was open in the Idea):** type coercion is out of scope for
  Phase A (REQ:no-coercion); the evaluator is type-agnostic.

## Open Questions

- Memoization lifetime and thread-safety: per-`Row`-instance single-threaded
  read is assumed sufficient for Phase A (REQ:memoize-per-row); concurrent access
  to the same `Row` instance's computed value is not yet specified.

## Sidekick Seeds Generated

- [starlark4dalgo-pluggable-formula-engine-adapter-libraries](../../ideas/seeds/starlark4dalgo-pluggable-formula-engine-adapter-libraries.md) — captured 2026-06-02 by specstudio:specify

---
*This document follows the https://specscore.md/feature-specification*
