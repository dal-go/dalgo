# Plan: GROUP BY with aggregation in the query builder, executed by dalgo2memory

**Status:** Implemented
**Source Feature:** query-group-by-aggregation
**Date:** 2026-06-05
**Owner:** alex
**Supersedes:** —

## Summary

Decomposes the `query-group-by-aggregation` Feature into eight linear tasks: three `dal` builder additions (`GroupBy`, `Having`, `COUNT(*)`), then the `dalgo2memory` grouping engine built up incrementally — single-source aggregation, the SELECT-grouping hard-error, `HAVING` evaluation, grouped ordering, and finally the join path. All twelve acceptance criteria are covered by a task; none are deferred.

## Approach

The Feature is a builder capability plus an executor that consumes it, so the order is build-the-input then consume-it, and the executor is grown one concern at a time to keep each task in one focused session. Tasks 1–3 add the `dal` surface (`GroupBy`, the new `Having` clause, and the `COUNT(*)` star expression) — pure builder/`String()` work with no execution semantics. Task 4 introduces the single-source grouping path in `dalgo2memory` (partition by group-key tuple, evaluate aggregates with SQL null-skipping) gated behind a non-empty `GroupBy()`, which also establishes the empty-`GroupBy()` passthrough. Task 5 adds the up-front hard-error validation the grouping path needs. Task 6 adds `HAVING` evaluation (alias and aggregate-expression operand forms, including an aggregate used only in `HAVING`). Task 7 applies `ORDER BY`/`LIMIT`/`OFFSET` to the grouped rows. Task 8 routes the join executor through the same grouping pass. Task 4 depends on Tasks 1+3; Task 6 depends on Tasks 2+4; Tasks 5, 7, and 8 each depend on Task 4's grouping path.

## Tasks

### Task 1: GroupBy builder method on dal.QueryBuilder

**Verifies:** query-group-by-aggregation#ac:group-by-recorded
**Status:** done

Add a chainable `GroupBy(expressions ...dal.Expression) dal.IQueryBuilder` to `QueryBuilder` and the `IQueryBuilder` interface, appending to `structuredQuery.groupBy` exactly as `OrderBy` appends to `orderBy`, so `GroupBy()` returns the expressions while a query on which `GroupBy` was never called reports an empty `GroupBy()`.

### Task 2: Having clause — getter, field, builder, and String() rendering

**Verifies:** query-group-by-aggregation#ac:having-recorded-and-rendered
**Status:** done

Add `Having() dal.Condition` to `StructuredQuery`, a `having Condition` field on `structuredQuery`, and a `Having(conditions ...dal.Condition) dal.IQueryBuilder` builder method that AND-combines multiple conditions like `Where`; render the `HAVING` clause in `structuredQuery.String()` immediately after the `GROUP BY` block.

### Task 3: COUNT(*) star expression and Count() builder

**Verifies:** query-group-by-aggregation#ac:count-star-expressible
**Status:** done

Add a star/row `Expression` to the `dal` query model and a `Count()` builder defined as an alias for `Count(*)`, rendering as `COUNT(*)` in `String()`, while the existing `CountAs(field, alias)` keeps its field-count semantics.

### Task 4: dalgo2memory single-source grouping and aggregate evaluation

**Verifies:** query-group-by-aggregation#ac:single-source-grouping-with-aggregates, query-group-by-aggregation#ac:all-null-group-aggregates-null, query-group-by-aggregation#ac:empty-groupby-unchanged
**Depends-On:** 1, 3
**Status:** done

When `q.GroupBy()` is non-empty, partition the WHERE-matched rows into groups keyed by the ordered group-key tuple (resolved via the shared per-source resolver) and emit one row per group, evaluating `SUM`/`COUNT`/`MIN`/`MAX`/`AVG`/`COUNT(*)` with standard SQL null handling (skip nulls; `AVG` divides by non-null count; all-null `SUM`/`AVG`/`MIN`/`MAX` yield `null`; `COUNT(*)` counts all rows) reusing the existing `number()` coercion; an empty `GroupBy()` bypasses the path entirely, leaving today's behavior unchanged.

### Task 5: SELECT-grouping-rule hard-error validation

**Verifies:** query-group-by-aggregation#ac:non-grouped-select-column-errors
**Depends-On:** 4
**Status:** done

Before emitting any group row, reject a grouped query whose SELECT contains a column that is neither an aggregate function expression nor one of the group-by expressions, returning a descriptive error and no rows — consistent with the eager hard-reject `validateColumns` already applies.

### Task 6: HAVING evaluation over aggregated group rows

**Verifies:** query-group-by-aggregation#ac:having-filters-by-alias, query-group-by-aggregation#ac:having-filters-by-aggregate-expression, query-group-by-aggregation#ac:having-on-unselected-aggregate
**Depends-On:** 2, 4
**Status:** done

When `q.Having()` is non-nil, evaluate it as a post-aggregation filter over each group's aggregated row and drop failing groups; resolve a `HAVING` operand by either the SELECT alias bound to an aggregate or the aggregate expression itself (both yielding the same per-group value), computing an aggregate referenced only in `HAVING` over the group without adding it to the output row.

### Task 7: grouped ORDER BY / LIMIT / OFFSET

**Verifies:** query-group-by-aggregation#ac:grouped-order-and-limit
**Depends-On:** 4
**Status:** done

For a grouped query, apply `ORDER BY`, `LIMIT`, and `OFFSET` to the post-`HAVING` group rows rather than to the pre-grouping input rows, so ordering and limiting operate on the aggregated result set.

### Task 8: grouping over the join path

**Verifies:** query-group-by-aggregation#ac:join-grouping-qualified
**Depends-On:** 4
**Status:** done

Route `executeJoinQuery` through the same grouping/aggregation pass so group keys and aggregate inputs can reference qualified sources (e.g. `u.country`), reusing the join's existing source-aware resolver, emitting one aggregated row per distinct group across the joined rows.

## Open Questions

- Exact name/shape of the star expression and the `Count()`/`CountAll` builder, and the `GroupBy`/`Having` method signatures — settled during implementation.
- Output-key collision when two selected columns resolve to the same alias/name — the in-scope ACs use distinct keys; last-write-wins vs. error is settled during implementation, consistent with the same open question in `query-column-projection`.
- `MIN`/`MAX` over non-numeric (string) values — the in-scope ACs aggregate numeric fields; non-numeric ordering semantics are deferred.

---
*This document follows the https://specscore.md/plan-specification*
