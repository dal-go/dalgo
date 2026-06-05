# Plan: Column selection in the query builder, projected by dalgo2memory

**Status:** Implementing
**Source Feature:** query-column-projection
**Date:** 2026-06-05
**Owner:** alex
**Supersedes:** —

## Summary

Decomposes the `query-column-projection` Feature into three linear tasks: add the `SelectColumns` terminal to `dal.QueryBuilder`, then project a non-empty `q.Columns()` into map records in `dalgo2memory` (single-source and join), then add the column validation errors. All six acceptance criteria are covered by a task; none are deferred.

## Approach

The Feature is a builder capability plus an executor that honors it, so the order is build-the-input then consume-it. Task 1 adds the `SelectColumns(...)` terminal that records `[]Column` on the `StructuredQuery` (the only producer of non-empty `Columns()` from the core builder), with the other terminals leaving it empty. Task 2 makes `dalgo2memory` honor a non-empty `Columns()`: it projects each result row of both the single-source and join paths to a `map[string]any` of exactly the selected columns (keyed by alias/field-name), resolving each column's `FieldRef` through the shared source-aware resolver, while an empty `Columns()` keeps the existing full-record path untouched — these three ACs are the cohesive core of the projection. Task 3 adds the up-front column validation the projection needs: an unknown column source and a non-`FieldRef` column each error with no rows. Task 2 depends on Task 1's terminal; Task 3 depends on Task 2's projection path.

## Tasks

### Task 1: SelectColumns terminal on dal.QueryBuilder

**Verifies:** query-column-projection#ac:select-columns-recorded
**Status:** done

Add `SelectColumns(columns ...dal.Column) dal.StructuredQuery` to `QueryBuilder` and the `IQueryBuilder` interface, recording the ordered column list on the `StructuredQuery` via `newQuery()` (mirroring the existing `Select*` terminals), so `Columns()` returns them while queries built by any other terminal report an empty `Columns()`.

### Task 2: dalgo2memory projects a non-empty Columns() into map records

**Verifies:** query-column-projection#ac:single-source-projection, query-column-projection#ac:join-projection-qualified, query-column-projection#ac:empty-columns-unchanged
**Depends-On:** 1

When `q.Columns()` is non-empty, project each result row of both `ExecuteQueryToRecordsReader` (single-source) and `executeJoinQuery` (join) to a `map[string]any` with one entry per selected column, keyed by its `Alias` (falling back to the field name) and resolved via the shared per-source resolver (empty `Source()` -> base; alias/name -> source, collision-correct), bypassing the keys-only `IntoRecord()==nil` branch. An empty `Columns()` leaves the existing full-record output unchanged.

### Task 3: column validation errors

**Verifies:** query-column-projection#ac:unknown-column-source-errors, query-column-projection#ac:non-field-column-errors
**Depends-On:** 2

Validate the selected columns before producing rows: a column whose `FieldRef` names a non-empty source matching no recordset, or whose expression is not a `FieldRef`, returns a descriptive error and no rows — consistent with the `WHERE`/`ORDER BY` unresolvable-source behavior.

## Open Questions

- Output-key collision when two selected columns resolve to the same alias/name — the in-scope ACs use distinct keys; last-write-wins vs. error is settled during implementation.

---
*This document follows the https://specscore.md/plan-specification*
