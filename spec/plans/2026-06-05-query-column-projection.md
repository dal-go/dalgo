---
format: https://specscore.md/plan-specification
status: Implemented
---

# Plan: Column selection in the query builder, projected by dalgo2memory

**Status:** Implemented
**Source Feature:** query-column-projection
**Date:** 2026-06-05
**Owner:** alex
**Supersedes:** —

## Summary

Decomposes the `query-column-projection` Feature into the original explicit-column work plus the YAML-native negative-projection extension. The extension adds an explicit wildcard-exclusion model, round-trips it through DTQL, and executes it in DALgo2SQL without changing existing wildcard or explicit projection behavior. All acceptance criteria are covered by a task; none are deferred.

## Approach

The original builder and dalgo2memory tasks remain unchanged. Negative projection then proceeds from shared AST to interchange to execution: Task 4 adds the wildcard-exclusion projection item without extending `StructuredQuery`; Task 5 adds its strict, canonical DTQL YAML form; Task 6 teaches DALgo2SQL to emit the real wildcard and filter driver-discovered columns in place. This ordering keeps missing exclusions non-fatal and preserves the requested exclusion list for future diagnostics.

## Tasks

### Task 1: SelectColumns terminal on dal.QueryBuilder

**Verifies:** query-column-projection#ac:select-columns-recorded
**Status:** complete

Add `SelectColumns(columns ...dal.Column) dal.StructuredQuery` to `QueryBuilder` and the `IQueryBuilder` interface, recording the ordered column list on the `StructuredQuery` via `newQuery()` (mirroring the existing `Select*` terminals), so `Columns()` returns them while queries built by any other terminal report an empty `Columns()`.

### Task 2: dalgo2memory projects a non-empty Columns() into map records

**Verifies:** query-column-projection#ac:single-source-projection, query-column-projection#ac:join-projection-qualified, query-column-projection#ac:empty-columns-unchanged
**Depends-On:** 1
**Status:** complete

When `q.Columns()` is non-empty, project each result row of both `ExecuteQueryToRecordsReader` (single-source) and `executeJoinQuery` (join) to a `map[string]any` with one entry per selected column, keyed by its `Alias` (falling back to the field name) and resolved via the shared per-source resolver (empty `Source()` -> base; alias/name -> source, collision-correct), bypassing the keys-only `IntoRecord()==nil` branch. An empty `Columns()` leaves the existing full-record output unchanged.

### Task 3: column validation errors

**Verifies:** query-column-projection#ac:unknown-column-source-errors, query-column-projection#ac:non-field-column-errors
**Depends-On:** 2
**Status:** complete

Validate the selected columns before producing rows: a column whose `FieldRef` names a non-empty source matching no recordset, or whose expression is not a `FieldRef`, returns a descriptive error and no rows — consistent with the `WHERE`/`ORDER BY` unresolvable-source behavior.

### Task 4: model wildcard exclusion as a projection item

**Verifies:** query-column-projection#ac:wildcard-exclusion-round-trip
**Depends-On:** 3
**Status:** complete

Add an additive `dal.Column` wildcard projection variant carrying optional source scope and the ordered requested exclusions. Preserve duplicates and render compact diagnostic notation without changing the `StructuredQuery` interface or the meaning of an empty `Columns()` list.

### Task 5: round-trip negative projection through DTQL YAML

**Verifies:** query-column-projection#ac:wildcard-exclusion-round-trip
**Depends-On:** 4
**Status:** complete

Add the strict `wildcard: {source?, exclude}` column form to DTQL shapes, schema, canonical serialization, deserialization, equality, examples, and reference documentation. Reject mixed/empty/unknown-source shapes while retaining exclusion spelling, order, and duplicates.

### Task 6: execute wildcard exclusion in DALgo2SQL

**Verifies:** query-column-projection#ac:sql-wildcard-exclusion-results, query-column-projection#ac:projection-regression-compatibility
**Depends-On:** 5
**Status:** complete

Compile a single-source wildcard exclusion to the source wildcard, use SQL driver column metadata to omit matched names from record and recordset readers, and protect any explicit helper projection appended after the wildcard. Cover unqualified and qualified forms, missing and duplicate exclusions, stable ordering, identity handling, invalid shapes, and existing explicit/wildcard regression behavior.

## Open Questions

- Output-key collision when two selected columns resolve to the same alias/name — the in-scope ACs use distinct keys; last-write-wins vs. error is settled during implementation.

---
*This document follows the https://specscore.md/plan-specification*
