# Idea: Column selection in the query builder, projected by dalgo2memory

**Status:** Specified
**Date:** 2026-06-05
**Owner:** alex
**Promotes To:** query-column-projection
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we let a dalgo caller select a subset of columns in the query builder and have dalgo2memory return records projected to exactly those columns?

## Context

Surfaced while scoping qualified ORDER BY resolution and parked as the sidekick seed 'dalgo2memory-column-projection'. dalgo2memory never consumes q.Columns() — both the single-source path and the join executor return full records. But the deeper blocker is upstream: dal.QueryBuilder exposes only SelectIntoRecord/SelectIntoRecordset/SelectKeysOnly and has NO way to set Columns; structuredQuery.columns is never assigned anywhere in dal. The only producer of a query with non-empty Columns() today is DTQL's reconstructedQuery wrapper (built from deserialized YAML). So projecting columns in any adapter is currently unreachable from normal code. dal.Column is {Alias string, Expression Expression}; the qualified-field resolver used by the join WHERE/ORDER BY (sources map, empty Source() -> base, alias/name -> source) can evaluate a column's FieldRef expression for projection.

## Recommended Direction

Deliver the capability end to end. First add a column-selection terminal to dal.QueryBuilder (e.g. SelectColumns(cols ...dal.Column) StructuredQuery) that records the selected columns on the StructuredQuery, alongside the existing Select* terminals. Then teach dalgo2memory to honor a non-empty q.Columns(): for each result row of both the single-source and join paths, project to a map[string]any containing only the selected columns, evaluating each column's FieldRef expression via the same source-aware resolver the join WHERE/ORDER BY already use and keying the output by the column's Alias (falling back to the field name). An empty Columns() keeps today's full-record behavior. This reuses the existing resolver and the map-record output shape the join path already returns, so it is a small, cohesive slice that finally makes column selection real for the in-memory adapter and lets a DTQL-deserialized column query actually execute.

## Alternatives Considered

- **Implement projection in `dalgo2memory` only, leave the builder alone.** Smallest code change. Lost because `dal.QueryBuilder` cannot set `Columns()` and `structuredQuery.columns` is never assigned, so the only query that would exercise the projection is a DTQL-deserialized one — the feature would be near-dead code with no normal producer.
- **Add the columnar recordset reader (`ExecuteQueryToRecordsetReader`) and project there.** Column projection is conceptually a columnar concern, and the recordset package already models columns. Lost because the recordset reader is a much larger surface (currently `ErrNotSupported`) and the existing `RecordsReader` path with a map record is the smaller, already-proven output shape; the recordset reader can adopt the same selection later.
- **Make selected columns project into the caller's typed `IntoRecord` struct.** Keeps the existing typed-record ergonomics. Lost because a struct already fixes its fields, so column selection cannot restrict them meaningfully and aliasing has nowhere to land; a `map[string]any` keyed by alias/name is the honest shape for an arbitrary projection.

## MVP Scope

A short spike: dal.QueryBuilder can express SELECT of a subset of columns (with optional aliases), and dalgo2memory returns records whose data map contains exactly those columns (aliased), for both a single-source and a two-table join query. Verified by table tests selecting two of several fields (one aliased) over in-memory data; empty Columns() still returns full records; dalgo2memory stays at 100% coverage.

## Not Doing (and Why)

- Computed / function / constant column expressions — only FieldRef columns are in scope; the rest is future work
- The columnar recordset reader (ExecuteQueryToRecordsetReader stays ErrNotSupported) — projection lands in the RecordsReader path as map records
- Projecting into a typed struct target — a column-projected query returns map[string]any keyed by alias/name
- DISTINCT, aggregates, GROUP BY — out of scope
- SQL adapter projection (dalgo2sql, dalgo2sqlite) — separate repos, unblocked by the shared dal capability

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | A column-selection terminal can be added to `dal.QueryBuilder` that records `[]Column` on the `StructuredQuery` without disrupting the existing `SelectIntoRecord`/`SelectIntoRecordset`/`SelectKeysOnly` terminals. | Add the terminal, build a query with selected columns, and confirm `q.Columns()` returns them while the existing builder tests stay green. |
| Must-be-true | The source-aware resolver the join WHERE/ORDER BY already use can evaluate a column's FieldRef expression for projection (empty Source() -> base; alias/name -> source). | Project a join by qualified columns (including a `u`/`o` name collision) and assert each output value comes from the named source. |
| Should-be-true | A `map[string]any` keyed by the column Alias (falling back to the field name) is an acceptable output shape for a projected query — no typed-target requirement. | Table tests read the projected map; review DTQL/datatug expectations for whether a typed projection is ever required. |
| Should-be-true | An empty `Columns()` must preserve today's full-record behavior for every existing query. | Run the existing `dalgo2memory` suite unchanged; only queries built with the new terminal project. |
| Might-be-true | Consumers will actually adopt column selection (smaller reads, DTQL round-trip execution) rather than always fetching full records. | Review DTQL and datatug for a concrete column-selection use before widening past FieldRef columns. |


## SpecScore Integration

- **New Features this would create:** a `dal` query-builder column-selection capability and `dalgo2memory` column projection — likely one Feature covering both (the builder terminal plus the executor honoring it).
- **Existing Features affected:** `dtql` — its serialized `Columns` would gain an executor that actually honors them; `query-joins` and `qualified-orderby-resolution` — projection reuses their source-aware resolver and the join path's map-record output.
- **Dependencies:** Upstream — the merged `query-joins` (resolver + map output) and `qualified-orderby-resolution` (shared resolution patterns). Sibling — none.

## Open Questions

- Exact builder API: a terminal `SelectColumns(cols ...dal.Column) StructuredQuery` vs. a chainable `Columns(...)` before an existing `Select*` terminal — settle at spec time.
- Output key when a column has neither an alias nor a `FieldRef` name (only relevant if non-FieldRef columns are ever admitted) — out of scope for the MVP, which is FieldRef-only.
- Whether a future columnar `ExecuteQueryToRecordsetReader` should share this selection logic, and how a typed-target projection (if ever needed) would be expressed.

---
*This document follows the https://specscore.md/idea-specification*
