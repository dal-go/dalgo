---
format: https://specscore.md/idea-specification
status: Implemented
---

# Idea: GROUP BY with aggregation in the query builder, executed by dalgo2memory

**Status:** Implemented
**Date:** 2026-06-05
**Owner:** alex
**Promotes To:** query-group-by-aggregation
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we let a dalgo caller express GROUP BY with aggregate functions and HAVING in the query builder, and have dalgo2memory return one aggregated row per group across single-source and joined queries?

## Context

Direct successor to query-column-projection (Implemented), which made dalgo2memory honor `q.Columns()` for plain FieldRef columns over single-source and join queries. The grouping infrastructure is half-built and currently unreachable: `dal.StructuredQuery` exposes `GroupBy() []Expression`, `structuredQuery` has the `groupBy []Expression` field plus getter, and `structuredQuery.String()` already renders a `GROUP BY` clause (`dal/query.go:38`, `dal/query_struct.go:25,68,153`). Aggregate Column builders already exist too — `SumAs`/`CountAs`/`MinAs`/`MaxAs`/`AverageAs` produce a `function` Expression wrapped in a `Column` (`dal/q_functions.go`). Two gaps make all of this dead code: (1) `IQueryBuilder`/`QueryBuilder` has no `GroupBy(...)` setter, so `structuredQuery.groupBy` is never assigned from normal code (same trap query-column-projection found with `columns`); (2) `dalgo2memory` never reads `q.GroupBy()`, and its `validateColumns` actively *rejects* any non-FieldRef column ("not a field reference"), so the existing aggregate builders cannot even execute against the in-memory adapter. There is also no `Having()` anywhere — not in the interface, struct, builder, or `String()`. The source-aware resolver introduced by qualified-orderby-resolution / first-class-query-joins (sources map: empty `Source()` → base, alias/name → source) is the same machinery that can compute group-key tuples and aggregate inputs.

## Recommended Direction

Deliver GROUP BY end to end against the in-memory adapter, sequenced so each step is independently shippable and testable rather than one big-bang change.

**Step 1 — builder plumbing (dal).** Add a chainable `GroupBy(expressions ...Expression) IQueryBuilder` to `IQueryBuilder`/`QueryBuilder` that appends to `s.q.groupBy`, mirroring `OrderBy` exactly. Add the HAVING clause that does not yet exist: a `Having() Condition` getter on `StructuredQuery`, a `having Condition` field on `structuredQuery`, a `Having(conditions ...Condition) IQueryBuilder` builder method, and `String()` rendering of a `HAVING` clause after `GROUP BY`. HAVING reuses `dal.Condition` — the same type and evaluator as WHERE — the only difference being it runs *after* grouping, over the aggregated row (so its operands reference aggregate aliases). Also add a star/row expression and a `CountAll`/`Count()` builder so `COUNT(*)` is expressible, with `Count()` defined as an alias for `Count(*)` (counts all rows in a group regardless of nulls); the existing `CountAs(field, alias)` keeps its skip-nulls field semantics. No execution semantics yet — just make the query expressible and verify `q.GroupBy()`/`q.Having()` round-trip.

**Step 2 — aggregation engine (dalgo2memory).** When `q.GroupBy()` is non-empty, after WHERE filtering, partition the matched rows into groups keyed by the tuple of group-key expression values (resolved via the existing source-aware resolver). Emit exactly one output row per distinct group. For each selected column: a group-key FieldRef resolves to that group's key value; an aggregate `function` Expression (SUM/COUNT/MIN/MAX/AVG) is evaluated over all rows in the group with **standard SQL null handling** — NULLs are skipped, `AVG` divides by the non-null count, and `SUM`/`AVG`/`MIN`/`MAX` over an all-null group return NULL; `COUNT(<field>)` counts non-null values while `COUNT(*)` counts all rows; numeric inputs reuse the existing `number()` coercion. Extend `validateColumns`/`projectRow` to accept aggregate `function` expressions when grouping is active, and **hard-reject** (before producing rows, consistent with `validateColumns` today) any grouped query whose SELECT has a non-aggregate column not present in `GROUP BY`. Apply `Having()` as a post-aggregation filter over the aggregated row. A HAVING operand may reference an aggregate by **either** form — the aggregate expression itself (`SUM(f2) > 5`) **or** its SELECT alias (`F_2 > 5`) — both resolving to the same per-group computed value; an aggregate referenced only in HAVING (not in SELECT) is still evaluated over the group. After HAVING, ORDER BY / LIMIT / OFFSET operate on the grouped rows.

**Step 3 — joins.** Make the join executor (`join.go`) feed the same grouping/aggregation pass, so group keys and aggregate inputs can reference qualified sources, reusing the resolver the join WHERE/ORDER BY already use.

This reuses three already-proven pieces — the `function` aggregate builders, the source-aware resolver, and the `map[string]any` projected-record output shape — so the new surface is the grouping driver itself plus the HAVING clause type, not a rewrite of the query model.

## Alternatives Considered

- **Grouping only, no aggregate evaluation.** Add `GroupBy(...)` and emit one row per distinct group-key tuple (group-key columns only), deferring aggregates. Smallest code change. Lost because GROUP BY without aggregation is just `DISTINCT` on the keys — and the pre-existing `SumAs`/`CountAs`/… builders make clear aggregation is the actual intent; shipping the keys-only version would leave those builders rejected and useless.
- **Builder plumbing only; dalgo2memory returns `ErrNotSupported` for grouped queries.** Cheapest possible slice. Lost because it recreates exactly the dead-code trap query-column-projection diagnosed: a builder method whose only effect is a query no adapter can run.
- **Defer HAVING and joins to follow-up Ideas; single-source aggregation only as the MVP.** Genuinely the simpler, lower-risk MVP, and it was offered. Lost to an explicit, informed scope decision by the owner to include joins and HAVING now. Recorded here because if the slice proves too large in practice, this is the fallback decomposition — Step 3 (joins) and the HAVING half of Step 2 are the natural cut lines.

## MVP Scope

A build that lands GROUP BY end to end in the in-memory adapter: `dal.QueryBuilder` can express `GroupBy(...)` group keys, aggregate columns (via the existing `SumAs`/`CountAs`/`MinAs`/`MaxAs`/`AverageAs`), and a `Having(...)` filter; and dalgo2memory returns one aggregated row per group. Verified by table tests over in-memory data: a single-source GROUP BY with a COUNT and a SUM per group; a HAVING that drops a group by its aggregate; the same over a two-table join with qualified group keys; an empty `GroupBy()` still returns full/ungrouped records unchanged; and dalgo2memory stays at 100% coverage. Sequenced internally (builder+HAVING clause → single-source aggregation → joins) so each step lands and is verified before the next.

## Not Doing (and Why)

- DTQL serialization of GROUP BY -- dtql/serialize.go already rejects GroupBy as unsupported; left unchanged
- SQL adapters (dalgo2sql, dalgo2sqlite) -- separate repos, unblocked by the shared dal capability
- DISTINCT, GROUPING SETS / ROLLUP / CUBE, window functions -- only flat GROUP BY is in scope
- New aggregate functions beyond the existing SUM/COUNT/MIN/MAX/AVG -- no new function builders added
- The columnar ExecuteQueryToRecordsetReader -- stays ErrNotSupported; aggregation lands in the RecordsReader path

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | A `GroupBy(...)` and a new `Having(...)` terminal can be added to `dal.QueryBuilder` without disrupting the existing `OrderBy`/`Where`/`Select*` terminals, and a new `Having() Condition` getter + `having` field fit `StructuredQuery`/`structuredQuery` cleanly. | Add them, build a grouped query, confirm `q.GroupBy()`/`q.Having()` round-trip and `String()` renders both; existing builder tests stay green. |
| Must-be-true | The source-aware resolver (empty `Source()` → base; alias/name → source) can compute group-key tuples and feed aggregate inputs over both single-source rows and join rows. | Group a single-source set by one field and a join by a qualified field (with a name collision across sources); assert each group key and aggregate input resolves from the right source. |
| Must-be-true | `validateColumns`/`projectRow` can be extended to evaluate `function` aggregate expressions per group and to enforce "every non-aggregate SELECT column appears in GROUP BY", without breaking the ungrouped projection path. | Project a grouped query mixing a group-key column and `CountAs`/`SumAs`; assert values; assert a non-grouped, non-aggregate column is rejected; rerun the existing projection suite unchanged. |
| Should-be-true | `Having()` can reuse `dal.Condition` evaluated over the aggregated/aliased output row (same shape `matchesWhere` already consumes) rather than needing a new clause type. | Add a HAVING that filters on a `COUNT` alias; confirm groups are dropped post-aggregation and the condition evaluator needs no special-casing. |
| Should-be-true | An empty `GroupBy()` preserves today's exact behavior for every existing query (no grouping, full or projected records as before). | Run the full dalgo2memory suite unchanged; only queries built with `GroupBy(...)` take the grouping path. |
| Might-be-true | Consumers actually want aggregation executed by the in-memory adapter (tests, datatug previews) rather than only by real SQL backends. | Review DTQL/datatug for a concrete in-memory aggregation use before widening past the five existing aggregate functions. |


## SpecScore Integration

- **New Features this would create:** a `query-group-by-aggregation` Feature (builder `GroupBy`/`Having` + dalgo2memory grouping/aggregation execution) — exact split TBD at spec time.
- **Existing Features affected:** query-column-projection (projection extended to evaluate `function` aggregate expressions under grouping), first-class-query-joins (grouping over the join path), qualified-orderby-resolution (group-key/aggregate resolution reuses its source-aware resolver).
- **Dependencies:** builds directly on query-column-projection (Implemented); no in-flight blockers.

## Open Questions

Resolved during ideation (recorded here; carry into the Feature spec):

- **HAVING type:** reuse `dal.Condition` — same type and evaluator as WHERE, applied post-grouping over the aggregated row. An operand may reference an aggregate by either the expression form (`SUM(f2) > 5`) or its SELECT alias (`F_2 > 5`); both resolve to the same per-group value, and an aggregate used only in HAVING is still computed over the group.
- **SELECT-rule enforcement:** hard error — reject a grouped query whose SELECT has a non-aggregate column missing from GROUP BY, consistent with `validateColumns` today.
- **COUNT:** add a star/row expression so `COUNT(*)` counts all rows in a group; `Count()` is an alias for `Count(*)`. Existing `CountAs(field, alias)` keeps skip-nulls field-count semantics.
- **Null handling:** standard SQL — aggregates skip NULLs; all-null `SUM`/`AVG`/`MIN`/`MAX` return NULL; numeric inputs reuse the existing `number()` coercion.

None outstanding.

---
*This document follows the https://specscore.md/idea-specification*
