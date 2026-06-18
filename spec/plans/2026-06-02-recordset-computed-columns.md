# Plan: Computed Columns in recordset (neutral evaluator contract)

**Status:** Implemented
**Source Feature:** recordset-computed-columns
**Date:** 2026-06-02
**Owner:** alex
**Supersedes:** —

## Summary

Decomposes the `recordset-computed-columns` Feature into five linear, bottom-up
tasks: the neutral `Evaluator` contract and computed-column type first, then lazy
resolution in `Row`, then memoization + error handling on that path, then `Data`
materialization, and finally the write-guard. All 10 acceptance criteria are
covered by a task; none are deferred.

## Approach

Tasks follow the dependency chain in the `recordset` package. Task 1 introduces
the types every later task needs (`Evaluator`, `ComputedColumn`,
`NewComputedColumn`) and asserts the no-scripting-dependency boundary. Task 2
adds the core lazy resolution branch in `columnarRow` for the by-name/by-index
accessors — detecting a `ComputedColumn`, building the stored-sibling map
(excluding computed columns), invoking the evaluator, and returning the raw
value. Task 3 layers per-`Row`-instance memoization and evaluator-error
propagation/caching onto that same branch. Task 4 reuses the branch so
`Row.Data(rs)` materializes computed columns through the memoized path rather
than their unused stored cell. Task 5 adds the set-rejection guard at the row
layer. ACs are grouped by the unit of implementation work, never split into
AC-wrapper tasks.

## Tasks

### Task 1: Evaluator contract, ComputedColumn marker, and constructor

**Verifies:** recordset-computed-columns#ac:evaluator-compiles, recordset-computed-columns#ac:recordset-has-no-scripting-dependency, recordset-computed-columns#ac:computed-column-registers-and-marks

Add the `Evaluator` interface (`Eval(stored map[string]any) (any, error)`), the
`ComputedColumn` interface (`Column[any]` + `Evaluator() Evaluator`), and
`NewComputedColumn(name string, evaluator Evaluator, options ...ColumnOption) Column[any]`
producing a value implementing both, with no per-row stored backing. The
`recordset` package takes on no formula-language/scripting dependency; the bare
computed column's own `GetValue(row)` returns a fail-loud error (resolution is
the Row's job, added in Task 2).

### Task 2: Lazy resolution in Row for GetValueByName/GetValueByIndex

**Verifies:** recordset-computed-columns#ac:lazy-resolves-from-siblings, recordset-computed-columns#ac:input-excludes-computed, recordset-computed-columns#ac:returns-raw-uncoerced-value

Branch `columnarRow.GetValueByName`/`GetValueByIndex`: when the resolved column
is a `ComputedColumn`, build a `map[string]any` of the row's non-computed columns
keyed by name (excluding every computed column), invoke the evaluator only at
access time, and return its result unchanged (`any`, no coercion). Non-computed
columns keep the existing stored path.

### Task 3: Per-Row-instance memoization and evaluator-error handling

**Verifies:** recordset-computed-columns#ac:memoized-single-eval, recordset-computed-columns#ac:eval-error-propagates-and-caches

Cache the resolved outcome — value or error — on the `Row` instance so the
evaluator is invoked at most once per (row instance, computed column). An
evaluator error is returned through the existing `(any, error)` return with a
nil value, never panics, and is itself cached so re-access does not re-invoke.

### Task 4: Computed-aware Row.Data materialization

**Verifies:** recordset-computed-columns#ac:data-materializes-computed

Route each computed column in `Row.Data(rs)` through the same memoized
resolution branch as the by-index accessor (not its unused stored cell), so the
materialized slice carries the computed value; propagate an evaluator error out
of `Data`.

### Task 5: Reject direct set on computed columns at the row layer

**Verifies:** recordset-computed-columns#ac:reject-set-on-computed

In `columnarRow.SetValueByName`/`SetValueByIndex`, detect a `ComputedColumn` and
return an error before delegating to the column's `SetValue`, so a computed value
cannot be written and a subsequent read still returns the evaluator-derived
value.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
