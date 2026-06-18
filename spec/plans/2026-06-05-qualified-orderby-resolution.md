# Plan: Source-qualified, multi-key ORDER BY resolution in dalgo2memory

**Status:** Implemented
**Source Feature:** qualified-orderby-resolution
**Date:** 2026-06-05
**Owner:** alex
**Supersedes:** —

## Summary

Decomposes the `qualified-orderby-resolution` Feature into three linear tasks: build the shared source-aware, multi-key ORDER BY comparator and wire it into the join executor, then add the join-ordering edge cases, then unify the single-source path onto the same comparator. All eight acceptance criteria are covered by a task; none are deferred.

## Approach

The Feature is one comparator used at two call sites, so the order is build-then-adopt. Task 1 creates the comparator (per-key source resolution reusing the join WHERE resolver, key-by-key stable sort via the existing `compare()`, base-id tiebreak) and adopts it in `executeJoinQuery`, replacing the base-id-only sort — its four ACs (qualified key, multi-key, and the two tiebreak branches) are the cohesive core behavior the comparator delivers, so they group on one task. Task 2 adds the comparator's edge handling exercised through the join path: pre-sort validation that errors on an unknown ORDER BY source, and skipping a non-`FieldRef` key. Task 3 retires `sortRows`, routing the single-source path through the same comparator (empty `Source()` -> base) and proving both unchanged ordering and the single-source unknown-source error. Tasks 2 and 3 both depend on Task 1's comparator.

## Tasks

### Task 1: Shared source-aware multi-key comparator, adopted by the join executor

**Verifies:** qualified-orderby-resolution#ac:join-orders-by-qualified-key, qualified-orderby-resolution#ac:orders-by-multiple-keys, qualified-orderby-resolution#ac:tiebreak-no-order-by, qualified-orderby-resolution#ac:tiebreak-all-keys-equal
**Status:** done

Build one ordering routine that resolves each `ORDER BY` `FieldRef` to its recordset (empty `Source()` -> base; non-empty -> the source whose `Alias()`/`Name()` matches, reusing the join WHERE resolver), stably sorts rows key-by-key in declared order honoring each `Descending()` via the existing `compare()`, and falls back to ascending base record id. Replace the join executor's base-id-only `sort.SliceStable` with it.

### Task 2: Join ORDER BY edge handling — unknown source errors, non-field keys skipped

**Verifies:** qualified-orderby-resolution#ac:unresolvable-order-source-errors, qualified-orderby-resolution#ac:non-field-order-key-ignored
**Depends-On:** 1
**Status:** done

Validate the resolved `ORDER BY` keys before sorting (since the sort callback cannot return an error): an `ORDER BY` `FieldRef` whose non-empty source names no recordset returns a descriptive error and no rows, while a non-`FieldRef` expression is skipped as an ordering key with the remaining keys and base-id tiebreak still applied.

### Task 3: Unify the single-source path onto the shared comparator

**Verifies:** qualified-orderby-resolution#ac:single-source-order-unchanged, qualified-orderby-resolution#ac:single-source-unresolvable-order-source
**Depends-On:** 1
**Status:** done

Route `ExecuteQueryToRecordsReader` through the shared comparator and remove `sortRows`, resolving an empty/base-matching `Source()` against the single base recordset so existing unqualified ordering (ascending and descending) is unchanged, and a non-empty source that matches neither the base alias nor name errors — the same rule as joins.

## Open Questions

- Final location/signature of the shared comparator so both entry points use it without exposing `dalgo2memory` internals — settled during implementation.

---
*This document follows the https://specscore.md/plan-specification*
