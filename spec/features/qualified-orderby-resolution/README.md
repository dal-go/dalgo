# Feature: Source-qualified, multi-key ORDER BY resolution in dalgo2memory

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/qualified-orderby-resolution?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/qualified-orderby-resolution?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/qualified-orderby-resolution?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/qualified-orderby-resolution?op=request-change) |
**Status:** Under Review
**Date:** 2026-06-05
**Owner:** alex
**Source Ideas:** qualified-orderby-resolution
**Supersedes:** —

## Summary

Gives `dalgo2memory` one source-aware, multi-key `ORDER BY` comparator shared by single-source and join queries: each order key resolves to the recordset its `FieldRef.Source()` names, multiple keys order in declared precedence, and a base-record-id tiebreak keeps the result deterministic. This completes the deferred `OrderBy` resolution the `query-joins` Feature left as an open question.

## Problem

`dalgo2memory` orders results two divergent, incomplete ways. The single-source `sortRows` sorts by a **single** bare field name (`orderBy[0]`, ignoring `FieldRef.Source()`), and the join executor **ignores `OrderBy` entirely**, sorting only by base record id. So a join `ORDER BY o.createdAt` has no effect, a multi-key order is impossible, and a qualified key under a name collision (`u.status` vs `o.status`) cannot be ordered correctly because the join's merged output map overlays one source over the other. The qualified-field resolver already exists for the join `WHERE` clause; this Feature extends the same resolution to ordering and unifies the two sort paths.

## Behavior

### Resolution and ordering

`ORDER BY` keys are resolved against the same per-source data the join `WHERE` clause uses, then applied as an ordered, multi-key comparison.

#### REQ: source-qualified-key-resolution

Each `ORDER BY` `FieldRef` key MUST be resolved to a recordset by its `Source()`: an empty source denotes the `From` base; a non-empty source denotes the recordset whose `Alias()` (falling back to `Name()`) matches. In a join query the value MUST be read from that source's own row data, so a key under a name collision (`u.status` vs `o.status`) orders by the named source's value, not whichever the merged output map happened to retain.

#### REQ: multi-key-ordering

Results MUST be ordered by the `OrderBy()` expressions in declared order: rows compare on the first key, and only when that key compares equal do they compare on the next, and so on. Each key MUST independently honor its `Descending()` flag.

#### REQ: deterministic-tiebreak

When every `ORDER BY` key compares equal, or no `ORDER BY` is given, rows MUST fall back to a deterministic order by base record id, so results are stable across runs.

### Edge handling

#### REQ: unresolvable-order-source-errors

An `ORDER BY` `FieldRef` whose non-empty `Source()` names no recordset in the query MUST produce a descriptive error and yield no rows — consistent with the join `WHERE` clause's unresolvable-source behavior — rather than silently falling back to the tiebreak order.

#### REQ: non-field-order-key

An `ORDER BY` expression that is not a `FieldRef` (e.g. a constant or computed expression) MUST NOT cause a crash; it is skipped as an ordering key, and the remaining keys (and finally the base-id tiebreak) still apply.

### Unification

#### REQ: unified-single-source-unchanged

Single-source and join queries MUST share one ordering implementation. A single-source query whose `ORDER BY` fields are unqualified (empty `Source()`, resolving to the one base recordset) MUST return results in the same order as before this Feature.

## Acceptance Criteria

### AC: join-orders-by-qualified-key (verifies REQ:source-qualified-key-resolution)

**Given** an INNER join of `users u` and `orders o` where both rows carry a colliding `status` field and the orders differ in `o.amount`
**When** the query is executed with `ORDER BY o.amount` ascending
**Then** the result rows are in ascending `o.amount` order, and ordering by `o.status` orders by the order's status value rather than the user's.

### AC: orders-by-multiple-keys (verifies REQ:multi-key-ordering)

**Given** a join result with two order keys — primary `u.id` ascending, secondary `o.amount` descending
**When** the query is executed
**Then** rows are ordered by `u.id` ascending, and within equal `u.id` by `o.amount` descending.

### AC: tiebreak-no-order-by (verifies REQ:deterministic-tiebreak)

**Given** a join query with no `ORDER BY`
**When** it is executed
**Then** the rows are returned in ascending base-record-id order, deterministically across runs.

### AC: tiebreak-all-keys-equal (verifies REQ:deterministic-tiebreak)

**Given** a join query ordered by a key whose value is equal across all result rows
**When** it is executed
**Then** the rows fall back to ascending base-record-id order.

### AC: unresolvable-order-source-errors (verifies REQ:unresolvable-order-source-errors)

**Given** a join query with `ORDER BY z.foo` where `z` names neither recordset
**When** it is executed
**Then** it returns a descriptive error naming the unresolved source and yields no result rows.

### AC: single-source-unresolvable-order-source (verifies REQ:unresolvable-order-source-errors)

**Given** a single-source query whose `ORDER BY` key has a non-empty `Source()` that matches neither the base recordset's alias nor its name
**When** it is executed
**Then** it returns a descriptive error and yields no result rows (the same rule as joins).

### AC: non-field-order-key-ignored (verifies REQ:non-field-order-key)

**Given** a query whose `ORDER BY` expression is a non-`FieldRef` (e.g. a constant)
**When** it is executed
**Then** it returns its rows without error, in the deterministic base-id tiebreak order.

### AC: single-source-order-unchanged (verifies REQ:unified-single-source-unchanged)

**Given** an existing single-source query ordered by an unqualified field, ascending and (separately) descending
**When** it is executed through the unified comparator
**Then** the result order matches the pre-Feature `sortRows` behavior for both directions.

## Architecture & Components

- **Shared comparator (`dalgo2memory`).** A single ordering routine takes the `[]dal.OrderExpression` and, per row, a way to resolve a `FieldRef` to a value. It pre-resolves/validates the keys (surfacing `unresolvable-order-source-errors` before sorting, since `sort` callbacks cannot return errors), then performs a stable multi-key sort using the existing `compare()` helper, with base-record-id as the final tiebreak.
- **Single-source path (`ExecuteQueryToRecordsReader`).** Replaces `sortRows`; resolves an empty/base-matching `Source()` against the one base recordset's `data` map.
- **Join path (`executeJoinQuery`).** Replaces the base-id-only `sort.SliceStable`; resolves each key against the per-source map already built for `WHERE` (empty → base, alias/name → joined source), reusing the join resolver.

## Data Flow

Rows are produced (single-source scan or join nested loop) → each `ORDER BY` key is resolved/validated against the query's known sources (unknown non-empty source → error, returned before sorting) → rows are stably sorted key-by-key in declared order with per-key `Descending()`, falling back to base-id → `Limit` → output records.

## Error Handling & Failure Modes

- `ORDER BY` key with an unknown non-empty source → descriptive error, no rows (`REQ:unresolvable-order-source-errors`).
- `ORDER BY` key that is not a `FieldRef` → ignored for ordering, no error (`REQ:non-field-order-key`).
- A qualified key resolving to an absent value (e.g. the joined source on an unmatched LEFT row) sorts by that absent/zero value via `compare()`, not an error.

## Testing Strategy

Table tests in `dalgo2memory`: a join ordered by a qualified numeric key and by a colliding `status` key; a two-key order (asc then desc); the no-`ORDER BY` / all-equal tiebreak; the unknown-source error; a non-`FieldRef` key; and the single-source ascending/descending order through the unified comparator (asserting parity with prior behavior). `dalgo2memory` MUST remain at 100% statement coverage, so every branch of the comparator and key-resolution is exercised.

## Rehearse Integration

All eight ACs are testable through pure Go execution over in-memory collections, so they map directly to `dalgo2memory` table tests (see `## Testing Strategy`). Per-AC Rehearse stub files are deferred to the Plan, where each AC becomes a concrete `*_test.go` case; the rehearsal surface is the Go test suite.

## Out of Scope

- Column projection in `dalgo2memory` (`q.Columns()` is unimplemented adapter-wide) — parked as the sidekick seed `dalgo2memory-column-projection`.
- `NULLS FIRST`/`NULLS LAST` control — absent/zero values fall out of `compare()`'s natural order.
- `ORDER BY` on functions/computed expressions as orderable keys — non-`FieldRef` keys are ignored, not evaluated.
- SQL adapter ordering (`dalgo2sql`/`dalgo2sqlite`) — separate repos.
- Self-joins and 3+ table ordering — single two-table join, matching the `query-joins` MVP.

## Assumption Carryover

From the source Idea `qualified-orderby-resolution`:

- **Carried (Must):** the join `WHERE` per-source resolver backs `ORDER BY` too, with no new aliasing machinery — validated by `AC:join-orders-by-qualified-key`.
- **Carried (Must):** unifying single-source ordering into the source-aware comparator does not change existing single-source results — validated by `AC:single-source-order-unchanged`.
- **Carried (Should):** `compare()` plus key-by-key short-circuit gives deterministic multi-key order — validated by `AC:orders-by-multiple-keys`, `AC:tiebreak-no-order-by`, and `AC:tiebreak-all-keys-equal`.
- **Resolved (was Should/open question):** an `ORDER BY` key naming no recordset **errors** (not silent fallback) — now `REQ:unresolvable-order-source-errors`; the same rule resolves the Idea's single-source non-matching-source open question (non-base source → error).
- **Deferred (Might):** whether consumers need multi-key now — this Feature commits to multi-key regardless, as the comparator cost is the same.

## Open Questions

- Final location/signature of the shared comparator so both entry points use it without exposing `dalgo2memory` internals — an implementation detail for the Plan.

---
*This document follows the https://specscore.md/feature-specification*
