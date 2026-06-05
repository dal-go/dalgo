# Feature: Column selection in the query builder, projected by dalgo2memory

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/query-column-projection?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/query-column-projection?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/query-column-projection?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/query-column-projection?op=request-change) |
**Status:** Under Review
**Date:** 2026-06-05
**Owner:** alex
**Source Ideas:** query-column-projection
**Supersedes:** —

## Summary

Adds a `SelectColumns` terminal to `dal.QueryBuilder` so a caller can select a subset of columns, and teaches `dalgo2memory` to honor a non-empty `q.Columns()` by returning each record's data as a map of exactly those columns (aliased), for both single-source and join queries. Empty `Columns()` keeps today's full-record behavior.

## Problem

`dalgo2memory` never consumes `q.Columns()` — but the deeper gap is upstream: `dal.QueryBuilder` exposes only `SelectIntoRecord`/`SelectIntoRecordset`/`SelectKeysOnly` and has no way to set columns, and `structuredQuery.columns` is never assigned anywhere in `dal`. So column selection is unreachable from normal code, and the only producer of a query with non-empty `Columns()` is DTQL's deserialized `reconstructedQuery`. This Feature makes column selection a first-class builder capability and gives it a real executor in the in-memory adapter, reusing the source-aware field resolver the join `WHERE`/`ORDER BY` already use.

## Behavior

### Selecting columns (the `dal` builder)

#### REQ: select-columns-terminal

`dal.QueryBuilder` MUST provide a `SelectColumns(columns ...dal.Column) dal.StructuredQuery` terminal (also on the `IQueryBuilder` interface) that records the given ordered, non-empty column list on the `StructuredQuery`, readable via `Columns()`. A query finalized by any other `Select*` terminal MUST have an empty `Columns()`.

### Projecting columns (`dalgo2memory`)

#### REQ: project-selected-columns

When `q.Columns()` is non-empty, `dalgo2memory` MUST return each result record's data as a `map[string]any` containing exactly the selected columns and no others, keyed by each column's `Alias` (falling back to the column's field name when the alias is empty), for both single-source and join queries.

#### REQ: qualified-column-resolution

Each selected column's `FieldRef` expression MUST be resolved to a recordset by its `Source()` — empty denotes the `From` base; non-empty denotes the recordset whose `Alias()`/`Name()` matches — using the same per-source resolution as the join `WHERE`/`ORDER BY`. A projected column under a name collision (`u.status` vs `o.status`) MUST take the named source's value.

#### REQ: empty-columns-unchanged

A query with an empty `Columns()` MUST return full records exactly as before this Feature — no projection, the existing `IntoRecord`/keys-only behavior is untouched.

#### REQ: column-error-handling

When projecting, a selected column whose `FieldRef` names a non-empty source that matches no recordset in the query, or whose expression is not a `FieldRef`, MUST produce a descriptive error and yield no result rows — consistent with the `WHERE`/`ORDER BY` unresolvable-source behavior. (Non-`FieldRef` column expressions are out of MVP scope and rejected rather than silently dropped.)

## Acceptance Criteria

### AC: select-columns-recorded (verifies REQ:select-columns-terminal)

**Given** a `dal.QueryBuilder`
**When** `SelectColumns` is called with two columns — `id` and `status` aliased as `s`
**Then** the resulting query's `Columns()` returns those two columns in order, while a query built with `SelectIntoRecord` returns an empty `Columns()`.

### AC: single-source-projection (verifies REQ:project-selected-columns)

**Given** a single-source query over rows with fields `id`, `name`, `status`, selecting `id` and `name` aliased as `n`
**When** it is executed by `dalgo2memory`
**Then** each result record's data map has exactly the keys `id` and `n` (the alias), and does not contain `name` or `status`.

### AC: join-projection-qualified (verifies REQ:project-selected-columns, REQ:qualified-column-resolution)

**Given** an INNER join of `users u` and `orders o` where both carry a `status` field, selecting `u.id` and `o.status`
**When** it is executed
**Then** each result record's data map contains exactly `id` and `status`, with `status` taking the order's value (not the user's field of the same name).

### AC: empty-columns-unchanged (verifies REQ:empty-columns-unchanged)

**Given** a query finalized with `SelectIntoRecord` (empty `Columns()`)
**When** it is executed by `dalgo2memory`
**Then** it returns the full records exactly as before this Feature, with no projection applied.

### AC: unknown-column-source-errors (verifies REQ:column-error-handling)

**Given** a query selecting a column whose `FieldRef` is qualified with a source that names no recordset in the query
**When** it is executed
**Then** it returns a descriptive error naming the unresolved source and yields no result rows.

### AC: non-field-column-errors (verifies REQ:column-error-handling)

**Given** a query selecting a column whose expression is not a `FieldRef` (e.g. a constant)
**When** it is executed
**Then** it returns a descriptive error and yields no result rows.

## Architecture & Components

- **`dal` builder.** New `SelectColumns(columns ...dal.Column) StructuredQuery` terminal on `QueryBuilder` (and `IQueryBuilder`) that sets `structuredQuery.columns` via `newQuery()`, mirroring the existing terminals. No other terminal sets columns.
- **`dalgo2memory` projection.** A projection step applied when `q.Columns()` is non-empty in both `ExecuteQueryToRecordsReader` (single-source) and `executeJoinQuery` (join): validate the selected columns up front (FieldRef-only, source resolvable), then for each row build a `map[string]any` keyed by `Alias`/field-name, resolving each column's `FieldRef` via the shared per-source resolver (empty `Source()` → base; alias/name → source). Reuses the single-source `sources` map and the join `sources` map established by `qualified-orderby-resolution`/`query-joins`.

## Data Flow

`SelectColumns(cols...)` records the columns on the `StructuredQuery` → `dalgo2memory` checks `q.Columns()`; if non-empty it validates the columns (FieldRef + resolvable source, else error before producing rows), assembles rows (single-source scan or join loop), and projects each row to a map of the selected columns keyed by alias/name → records with map data. Empty `Columns()` follows the existing full-record path.

## Error Handling & Failure Modes

- Selected column with an unknown non-empty source → descriptive error, no rows (`REQ:column-error-handling`).
- Selected column whose expression is not a `FieldRef` → descriptive error, no rows (`REQ:column-error-handling`).
- A resolvable column whose value is absent on a row (e.g. the joined source on an unmatched LEFT row) projects to a `nil`/absent map entry, not an error.

## Testing Strategy

Table tests in `dal` (the `SelectColumns` terminal records columns; other terminals leave them empty) and in `dalgo2memory` (single-source subset projection with an alias; join projection with a `u`/`o` collision; empty-`Columns()` returns full records unchanged; unknown-source and non-`FieldRef` column errors). `dalgo2memory` MUST remain at 100% statement coverage, exercising every branch of the projection and column validation.

## Rehearse Integration

All six ACs are testable through pure Go — `dal` builder calls and `dalgo2memory` execution over in-memory collections — so they map directly to table tests (see `## Testing Strategy`). Per-AC Rehearse stub files are deferred to the Plan, where each AC becomes a concrete `*_test.go` case; the rehearsal surface is the Go test suite.

## Out of Scope

- Computed / function / constant column expressions — only `FieldRef` columns are projected; others are rejected (`REQ:column-error-handling`).
- The columnar recordset reader (`ExecuteQueryToRecordsetReader` stays `ErrNotSupported`) — projection lands in the `RecordsReader` path as map records.
- Projecting into a typed struct target — a column-projected query returns `map[string]any` keyed by alias/name.
- `DISTINCT`, aggregates, `GROUP BY`.
- SQL adapter projection (`dalgo2sql`/`dalgo2sqlite`) — separate repos, unblocked by the shared `dal` capability.

## Assumption Carryover

From the source Idea `query-column-projection`:

- **Carried (Must):** a column-selection terminal can be added to `dal.QueryBuilder` without disrupting the existing terminals — validated by `AC:select-columns-recorded`.
- **Carried (Must):** the join source-aware resolver can evaluate a column's `FieldRef` for projection — validated by `AC:join-projection-qualified`.
- **Carried (Should):** a `map[string]any` keyed by alias/name is an acceptable projected-output shape — embodied by `REQ:project-selected-columns` and its ACs.
- **Carried (Should):** empty `Columns()` preserves today's full-record behavior — validated by `AC:empty-columns-unchanged`.
- **Resolved:** a non-`FieldRef` or unresolvable column errors (rather than silently dropping) — now `REQ:column-error-handling`.
- **Deferred (Might):** whether consumers adopt column selection — not validated here; the capability is delivered regardless.

## Open Questions

- Final signature/name of the builder terminal (`SelectColumns(...)` vs. a chainable `Columns(...)` before a `Select*`) — an implementation detail for the Plan.
- Output key collision when two selected columns resolve to the same alias/name (e.g. `u.status` and `o.status` both unaliased) — the MVP's ACs use distinct keys; last-write-wins vs. error is a Plan-time decision.

---
*This document follows the https://specscore.md/feature-specification*
