# Feature: GROUP BY with aggregation in the query builder, executed by dalgo2memory

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/query-group-by-aggregation?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/query-group-by-aggregation?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/query-group-by-aggregation?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/query-group-by-aggregation?op=request-change) |
**Status:** Stable
**Date:** 2026-06-05
**Owner:** alex
**Source Ideas:** query-group-by-aggregation
**Supersedes:** —
**Grade:** A

## Summary

Adds `GroupBy` and `Having` to `dal.QueryBuilder` (plus a `COUNT(*)` star expression with a `Count()` alias), and teaches `dalgo2memory` to execute grouped queries: partition matched rows into groups, emit one aggregated row per group, evaluate `SUM`/`COUNT`/`MIN`/`MAX`/`AVG` with standard SQL null handling, and filter groups with `HAVING` — for both single-source and join queries. An empty `GroupBy()` keeps today's behavior unchanged.

## Problem

The grouping infrastructure is half-built and unreachable. `dal.StructuredQuery` already exposes `GroupBy() []Expression`, `structuredQuery` carries the `groupBy` field and getter, and `String()` already renders a `GROUP BY` clause — but `IQueryBuilder`/`QueryBuilder` has no setter, so `structuredQuery.groupBy` is never assigned from normal code (the same dead-code trap `query-column-projection` found with `columns`). The aggregate `Column` builders (`SumAs`/`CountAs`/`MinAs`/`MaxAs`/`AverageAs`) exist too, but `dalgo2memory` never reads `q.GroupBy()` and its `validateColumns` actively rejects any non-`FieldRef` column, so those aggregates cannot execute against the in-memory adapter. There is no `Having()` anywhere, and no way to express `COUNT(*)`. This Feature makes GROUP BY a first-class builder capability and gives it a real executor, reusing the source-aware field resolver the join `WHERE`/`ORDER BY` already use.

## Behavior

### Expressing grouped queries (the `dal` builder)

#### REQ: group-by-builder

`dal.QueryBuilder` MUST provide a `GroupBy(expressions ...dal.Expression) dal.IQueryBuilder` method (also on the `IQueryBuilder` interface) that appends the given expressions to the query's group-by list, readable via `GroupBy()`, chainable like `OrderBy`. A query on which `GroupBy` is never called MUST have an empty `GroupBy()`.

#### REQ: having-builder

`dal.StructuredQuery` MUST expose `Having() dal.Condition`, backed by a new `having` field on `structuredQuery`. `dal.QueryBuilder` MUST provide a `Having(conditions ...dal.Condition) dal.IQueryBuilder` method (also on `IQueryBuilder`) that records the condition, AND-combining multiple conditions exactly as `Where` does. `structuredQuery.String()` MUST render the `HAVING` clause after the `GROUP BY` clause. A query on which `Having` is never called MUST have a nil `Having()`.

#### REQ: count-star

The `dal` query model MUST provide a way to express `COUNT(*)` — counting all rows in a group regardless of nulls — via a star/row expression, with a `Count()` builder defined as an alias for `Count(*)`. The expression MUST render as `COUNT(*)` in `String()`. The existing `CountAs(field, alias)` MUST retain its field-count (skip-nulls) semantics, distinct from `COUNT(*)`.

### Executing grouped queries (`dalgo2memory`)

#### REQ: partition-into-groups

When `q.GroupBy()` is non-empty, `dalgo2memory` MUST, after applying `WHERE`, partition the matched rows into groups keyed by the ordered tuple of the group-by expression values — each resolved via the same per-source resolver used by `WHERE`/`ORDER BY` (empty `Source()` → `From` base; non-empty → the recordset whose `Alias()`/`Name()` matches) — and emit exactly one result row per distinct group, for both single-source and join queries.

#### REQ: aggregate-evaluation

For a grouped query, each selected aggregate function column (`SUM`, `COUNT`, `MIN`, `MAX`, `AVG`, and `COUNT(*)`) MUST be evaluated over the rows of its group using standard SQL null handling: `NULL` inputs are skipped; `AVG` divides the sum by the non-null count; `SUM`/`AVG`/`MIN`/`MAX` over a group with no non-null value yield `NULL`; `COUNT(<field>)` counts non-null values; `COUNT(*)` counts all rows in the group. Numeric inputs reuse the adapter's existing numeric coercion. A selected group-by column resolves to that group's key value.

#### REQ: select-grouping-rule

When `q.GroupBy()` is non-empty, a selected column that is neither an aggregate function expression nor one of the group-by expressions MUST produce a descriptive error and yield no result rows, before any group row is emitted — consistent with the eager-validation, hard-reject behavior `validateColumns` already applies.

#### REQ: having-filter

When `q.Having()` is non-nil, `dalgo2memory` MUST evaluate it as a post-aggregation filter over each group's aggregated row and drop the groups for which it is false. A `HAVING` operand MUST resolve an aggregate referenced **either** by its aggregate expression (e.g. `SUM(amount)`) **or** by the SELECT alias bound to that aggregate (e.g. `total`); both forms MUST yield the same per-group value. An aggregate referenced only in `HAVING` (not selected) MUST still be computed over the group and MUST NOT appear in the output row.

#### REQ: grouped-order-limit

For a grouped query, `ORDER BY`, `LIMIT`, and `OFFSET` MUST be applied to the post-`HAVING` group rows, not to the pre-grouping input rows.

#### REQ: empty-groupby-unchanged

A query with an empty `GroupBy()` MUST behave exactly as before this Feature — the grouping/aggregation path is not entered, and the existing projection / full-record / keys-only behavior is untouched.

## Acceptance Criteria

### AC: group-by-recorded (verifies REQ:group-by-builder)

**Given** a `dal.QueryBuilder`
**When** `GroupBy` is called with the field `category`
**Then** the resulting query's `GroupBy()` returns that one expression, while a query on which `GroupBy` was never called returns an empty `GroupBy()`.

### AC: having-recorded-and-rendered (verifies REQ:having-builder)

**Given** a query grouped by `category`
**When** `Having` is called with a condition `COUNT(*) > 2`
**Then** `Having()` returns that condition and `String()` renders a `HAVING` clause positioned after the `GROUP BY` clause.

### AC: count-star-expressible (verifies REQ:count-star)

**Given** the `dal` query model
**When** a column is built via `Count()`
**Then** it equals the `Count(*)` star form, renders as `COUNT(*)`, and is distinct from `CountAs("amount", "n")` which counts a field.

### AC: single-source-grouping-with-aggregates (verifies REQ:partition-into-groups, REQ:aggregate-evaluation)

**Given** a single-source collection with three rows in category `A` (amounts `10`, `20`, `null`) and one row in category `B` (amount `5`), grouped by `category`, selecting `category`, `Count(*)` as `n`, `SumAs(amount,"total")`, and `AverageAs(amount,"avg")`
**When** it is executed by `dalgo2memory`
**Then** exactly two rows are returned — `A` with `n=3`, `total=30`, `avg=15` (the null amount skipped, so avg divides by 2); `B` with `n=1`, `total=5`, `avg=5`.

### AC: join-grouping-qualified (verifies REQ:partition-into-groups)

**Given** an INNER join of `users u` and `orders o` grouped by `u.country`, selecting `u.country` and `Count(*)` as `orders`
**When** it is executed
**Then** exactly one row per distinct country is returned, each carrying the count of joined `u`/`o` rows for that country.

### AC: non-grouped-select-column-errors (verifies REQ:select-grouping-rule)

**Given** a query grouped by `category` that selects a non-aggregate column `name` which is not in the `GROUP BY` list
**When** it is executed
**Then** it returns a descriptive error and yields no result rows.

### AC: all-null-group-aggregates-null (verifies REQ:aggregate-evaluation)

**Given** a single-source collection where every row of group `A` has a `null` `amount`, grouped by `category`, selecting `SumAs(amount,"total")`, `MaxAs(amount,"hi")`, and `Count(*)` as `n`
**When** it is executed
**Then** group `A`'s row has `total=null` and `hi=null` while `n` equals the row count of the group.

### AC: having-filters-by-alias (verifies REQ:having-filter)

**Given** the rows grouped by `category` (group `A` total `30`, group `B` total `5`), `SumAs(amount,"total")` selected, with `HAVING total > 10`
**When** it is executed
**Then** only group `A` is returned; group `B` is dropped.

### AC: having-filters-by-aggregate-expression (verifies REQ:having-filter)

**Given** the same grouping with `HAVING SUM(amount) > 10` (the aggregate-expression form)
**When** it is executed
**Then** the result is identical to the alias form — only group `A` is returned.

### AC: having-on-unselected-aggregate (verifies REQ:having-filter)

**Given** rows grouped by `category` selecting only `category` and `Count(*)` as `n`, with `HAVING SUM(amount) > 10` where `SUM(amount)` is not selected
**When** it is executed
**Then** group `A` (sum `30`) is returned and group `B` (sum `5`) is dropped, and each output row contains only `category` and `n` — no `SUM` value is added to the output.

### AC: grouped-order-and-limit (verifies REQ:grouped-order-limit)

**Given** rows producing three groups with `Count(*)` values `5`, `3`, and `1`, grouped and ordered by `Count(*)` descending with `LIMIT 2`
**When** it is executed
**Then** exactly the two highest-count groups are returned, in descending count order.

### AC: empty-groupby-unchanged (verifies REQ:empty-groupby-unchanged)

**Given** a query with no `GroupBy` call (empty `GroupBy()`)
**When** it is executed by `dalgo2memory`
**Then** it returns records exactly as before this Feature, with no grouping or aggregation applied.

## Architecture & Components

- **`dal` builder.** New chainable `GroupBy(expressions ...Expression) IQueryBuilder` and `Having(conditions ...Condition) IQueryBuilder` on `QueryBuilder` (and `IQueryBuilder`); `GroupBy` appends to `structuredQuery.groupBy`, `Having` records onto a new `having Condition` field (AND-combined like `Where`'s `conditions`). `StructuredQuery` gains `Having() Condition`; `String()` renders `HAVING` after the existing `GROUP BY` block. A new star/row `Expression` plus a `Count()` builder (alias for `Count(*)`) extend `q_functions.go` alongside the existing `CountAs`.
- **`dalgo2memory` grouping engine.** When `q.GroupBy()` is non-empty, both `ExecuteQueryToRecordsReader` (single-source) and `executeJoinQuery` (join) route through a grouping pass: validate selected columns (each must be an aggregate `function` or a group-by expression, else error before rows), partition WHERE-matched rows by the group-key tuple via the shared per-source resolver, evaluate each selected aggregate per group (reusing the existing `number()` coercion), build the aggregated output row keyed by alias/field-name, apply `Having()` over that row, then apply `ORDER BY`/`LIMIT`/`OFFSET` to the surviving group rows. Empty `GroupBy()` bypasses the pass entirely.
- **HAVING resolver.** A small operand resolver that maps a `HAVING` operand to a per-group value by matching either the SELECT alias→value map or an aggregate expression evaluated over the group (computing it on demand when it is not in SELECT), then feeds the existing `Condition` evaluator (`matchesWhere`-style).

## Data Flow

`GroupBy(exprs...)`/`Having(conds...)` record on the `StructuredQuery` → `dalgo2memory` checks `q.GroupBy()`; if empty it follows the existing path. If non-empty: validate selected columns (aggregate-or-group-key, else error) → scan/join to WHERE-matched rows → partition rows into groups by the group-key tuple → per group, resolve group-key columns and evaluate aggregate columns (null-skipping, `number()` coercion) into one output row → evaluate `Having()` over each group row (operand resolved by alias or aggregate expression) and drop failing groups → apply `ORDER BY`/`LIMIT`/`OFFSET` to the group rows → records with map data.

## Error Handling & Failure Modes

- Selected column that is neither an aggregate nor a group-by expression → descriptive error, no rows (`REQ:select-grouping-rule`).
- A group-by or aggregate `FieldRef` naming a non-empty source that matches no recordset → descriptive error, no rows — consistent with the existing `WHERE`/`ORDER BY`/projection unresolvable-source behavior.
- An aggregate over a group with no non-null value yields a `null` cell (not an error); `COUNT` of such a group yields `0`/the row count, never `null`.
- A `HAVING` operand referencing an alias/aggregate that resolves to nothing computable → descriptive error, no rows.

## Testing Strategy

Table tests in `dal` (the `GroupBy`/`Having` methods record and `String()` renders them in order; `Count()` equals `Count(*)` and renders `COUNT(*)`) and in `dalgo2memory` (single-source grouping with `COUNT(*)`/`SUM`/`AVG` and a null amount; all-null group yields null aggregates; join grouping by a qualified key; the non-grouped-select-column error; `HAVING` by alias, by aggregate expression, and on an unselected aggregate; grouped `ORDER BY` + `LIMIT`; empty-`GroupBy()` unchanged). `dalgo2memory` MUST remain at 100% statement coverage, exercising every branch of the grouping, aggregation, validation, and `HAVING` paths.

## Rehearse Integration

All ACs are testable through pure Go — `dal` builder calls and `dalgo2memory` execution over in-memory collections — so they map directly to table tests (see `## Testing Strategy`). Per-AC Rehearse stub files are deferred to the Plan, where each AC becomes a concrete `*_test.go` case; the rehearsal surface is the Go test suite.

## Out of Scope

- DTQL serialization of `GROUP BY`/`HAVING` — `dtql/serialize.go` already rejects `GroupBy` as unsupported; left unchanged.
- SQL adapters (`dalgo2sql`/`dalgo2sqlite`) — separate repos, unblocked by the shared `dal` capability.
- `DISTINCT`, `GROUPING SETS`/`ROLLUP`/`CUBE`, and window functions — only flat `GROUP BY` is in scope.
- New aggregate functions beyond the existing `SUM`/`COUNT`/`MIN`/`MAX`/`AVG` (plus the `COUNT(*)` star form) — no other function builders are added.
- The columnar `ExecuteQueryToRecordsetReader` — stays `ErrNotSupported`; aggregation lands in the `RecordsReader` path as map records.

## Assumption Carryover

From the source Idea `query-group-by-aggregation`:

- **Carried (Must):** `GroupBy`/`Having` terminals fit `dal.QueryBuilder` without disrupting the existing terminals — validated by `AC:group-by-recorded`, `AC:having-recorded-and-rendered`.
- **Carried (Must):** the source-aware resolver computes group-key tuples and aggregate inputs over single-source and join rows — validated by `AC:single-source-grouping-with-aggregates`, `AC:join-grouping-qualified`.
- **Carried (Must):** `validateColumns`/`projectRow` extend to evaluate aggregate `function` expressions and enforce the group-key-or-aggregate SELECT rule — validated by `AC:single-source-grouping-with-aggregates`, `AC:non-grouped-select-column-errors`.
- **Resolved (was Should):** `HAVING` reuses `dal.Condition`, evaluated post-grouping, with operands by alias or aggregate expression — now `REQ:having-filter`, validated by the three `having-*` ACs.
- **Carried (Should):** empty `GroupBy()` preserves today's behavior — validated by `AC:empty-groupby-unchanged`.
- **Resolved decisions:** hard-error on the SELECT-grouping rule; `COUNT(*)` star expression with `Count()` alias; standard-SQL null skipping — now `REQ:select-grouping-rule`, `REQ:count-star`, `REQ:aggregate-evaluation`.
- **Deferred (Might):** whether consumers adopt in-memory aggregation — not validated here; the capability is delivered regardless.

## Open Questions

- Exact name/shape of the star expression and the `Count()`/`CountAll` builder, and the `GroupBy`/`Having` method signatures — implementation details for the Plan.
- Output-key collision when two selected columns resolve to the same alias/name — out of the MVP ACs (distinct keys used); last-write-wins vs. error is a Plan-time decision, consistent with the same open question in `query-column-projection`.
- `MIN`/`MAX` over non-numeric (string) values — the in-scope ACs aggregate numeric fields; ordering semantics for non-numeric aggregates is deferred.

---
*This document follows the https://specscore.md/feature-specification*
