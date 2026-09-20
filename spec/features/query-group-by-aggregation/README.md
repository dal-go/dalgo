---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Provider-independent GROUP BY and aggregation

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/query-group-by-aggregation?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/query-group-by-aggregation?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/query-group-by-aggregation?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/query-group-by-aggregation?op=request-change) |
**Status:** Stable
**Date:** 2026-09-20
**Owner:** alex
**Source Ideas:** query-group-by-aggregation
**Supersedes:** —
**Grade:** A

## Summary

DTQL and DALgo represent, validate, plan, and execute aggregation independently
of any one provider. DTQL uses YAML pipeline order with `columns` last. DALgo
chooses full native execution, ordered streaming, or hash aggregation from
granular provider capabilities. SQLite renders its supported subset natively.

## Behavior

### REQ: yaml-aggregation

DTQL MUST serialize and deserialize `groupBy`, aggregate/binary expressions,
`having`, aliases, and aggregate ordering. Canonical YAML MUST keep `columns`
after filtering, grouping, HAVING, ordering and pagination. With `groupBy` and
no `columns`, grouping expressions MUST be the implicit projection.

### REQ: aggregate-functions

DALgo MUST represent `COUNT(*)`, `COUNT(expr)`, distinct `COUNT`/`SUM`/`AVG`,
ordinary `SUM`/`AVG`, `MIN`, `MAX`, `FIRST`, and `LAST`. Arguments MUST be
expressions, including arithmetic binary expressions. Existing AggregateFunc
implementers MUST remain source-compatible.

### REQ: grouping-validation

An aggregate query MUST reject output expressions that are neither aggregated
nor present in `groupBy`. GROUP BY expressions MUST NOT contain aggregates.
Nested aggregates, unsupported arity, `COUNT(DISTINCT *)`, and DISTINCT on
MIN/MAX/FIRST/LAST MUST produce descriptive errors. Aliases MAY be used by
HAVING and result ORDER BY independently of target SQL alias rules.

### REQ: logical-stages

Execution MUST preserve: source, WHERE, grouping, aggregation, HAVING, requested
result ordering, OFFSET/LIMIT, output. Internal grouping order MUST remain
separate from requested result order. Result OFFSET/LIMIT MUST NOT be pushed
into a raw scan below local grouping.

### REQ: value-semantics

COUNT(*) counts rows and returns int64. Other aggregates ignore null, except
FIRST/LAST where null is a legitimate value. SUM/AVG use finite float64
accumulation; non-numeric dynamic inputs are ignored. Empty implicit grouping
returns one row with count zero and other aggregates null; empty explicit
grouping returns no rows. FIRST/LAST MUST require a declared stable input order
until aggregate-local ordering exists. Arithmetic MUST normalize numeric
operands to float64; non-numeric operands and division by zero evaluate to null.

### REQ: capabilities-plan

DALgo MUST expose additive granular capabilities for GROUP BY, HAVING, result
ORDER BY, stable input order, and each ordinary/distinct aggregate form. The
inspectable plan MUST select native execution only when every required stage is
supported, ordered streaming when grouping keys can be requested as source
order, and hash aggregation otherwise.

### REQ: ordered-streaming

Ordered streaming MUST consume rows incrementally, retain only the active
group's aggregate states, finalize HAVING when the key changes, and emit the
completed row immediately when no downstream result sort requires buffering.
DISTINCT MUST retain only its per-state value set.

### REQ: hash-fallback

Hash execution MUST use collision-safe typed composite keys and retain aggregate
states rather than source rows. It MUST finalize HAVING before result ordering,
OFFSET and LIMIT. Group/distinct entry and retained-key byte limits MUST return
explicit errors.

### REQ: sqlite-native

SQLite MUST render GROUP BY, aggregate expressions, DISTINCT, HAVING, aliases
rewritten to expressions, result ORDER BY and pagination in SQL clause order.
It MUST advertise only operations it executes with DALgo semantics. FIRST/LAST
MUST remain local/unsupported until deterministic aggregate ordering is modeled.

### REQ: lifecycle

Local readers MUST observe context cancellation, propagate mid-stream provider
errors, stop upstream reading when downstream completion permits it, and close
upstream resources.

## Acceptance Criteria

### AC: yaml-round-trip (verifies REQ:yaml-aggregation)

Given grouped YAML with HAVING, DISTINCT, arithmetic and aliases, deserialize
then serialize yields the canonical equivalent with `columns` last.

### AC: omitted-projection (verifies REQ:yaml-aggregation)

Given `groupBy: [country, city]` without `columns`, effective output contains
country and city in that order.

### AC: all-functions (verifies REQ:aggregate-functions, REQ:value-semantics)

Grouped and ungrouped queries cover count star/expression/distinct, sum and avg
ordinary/distinct, min, max, first and last, including nulls and empty input.

### AC: invalid-selection (verifies REQ:grouping-validation)

Grouping by country while selecting city without aggregation fails before any
result row is emitted.

### AC: aliases (verifies REQ:grouping-validation)

HAVING and ORDER BY referencing a SELECT aggregate alias produce the same values
as the equivalent aggregate-expression form.

### AC: stage-order (verifies REQ:logical-stages)

WHERE filters input, HAVING filters finalized groups, requested ordering differs
from internal group order, and OFFSET/LIMIT select aggregate rows rather than raw
rows.

### AC: native-plan (verifies REQ:capabilities-plan, REQ:sqlite-native)

A SQLite query whose required aggregate forms are advertised uses native SQL
with GROUP BY/HAVING/DISTINCT and returns the expected result.

### AC: streaming-plan (verifies REQ:capabilities-plan, REQ:ordered-streaming)

A provider advertising ORDER BY but not GROUP BY receives grouping-key source
ordering and produces correct aggregate rows incrementally.

### AC: hash-plan (verifies REQ:capabilities-plan, REQ:hash-fallback)

A provider advertising neither grouping nor useful ordering produces the same
observable rows as ordered streaming.

### AC: strategy-parity (verifies REQ:capabilities-plan, REQ:value-semantics, REQ:sqlite-native)

One identical dataset and logical query produce equivalent observable rows via
native SQLite, ordered streaming and hash execution, including portable numeric
normalization and binary string equality.

### AC: unsafe-limit (verifies REQ:logical-stages)

Local aggregation clears source OFFSET/LIMIT and applies them only after HAVING
and requested result ordering.

### AC: deterministic-first-last (verifies REQ:value-semantics)

FIRST/LAST include null and execute with declared stable input order; planning
without that guarantee returns an explicit deterministic-order error.

### AC: cancellation-errors (verifies REQ:lifecycle)

Cancellation closes the upstream reader and a provider error after one or more
rows is returned unchanged to the caller.

## Architecture & Components

- `dal/q_functions.go` and `dal/q_binary.go`: backward-compatible aggregate and
  scalar expression model.
- `dal/aggregation_plan.go`: validation, capabilities and inspectable strategy.
- `dal/aggregation_execute.go`: framework wrapper, ordered stream and hash state.
- `dtql`: canonical recursive YAML codec and generated schema.
- `dalgo2sql/sqlite_emit.go`: native SQLite compilation and alias rewriting.

## Error Handling & Failure Modes

Unsupported expression shapes, aggregate combinations, unstable FIRST/LAST,
non-portable group keys, non-finite native or local accumulation and exhausted
resource budgets are explicit errors. Provider/cancellation errors are not
converted into partial success. Native/local collation differences remain provider-visible and MUST be
reflected by conservative capability declarations.

## Out of Scope

JOIN implementation, ROLLUP, CUBE, GROUPING SETS, window functions,
percentiles/statistical aggregates, distributed aggregation and disk spilling.
Qualified fields remain representable for future JOIN work.

## Open Questions

- Aggregate-local ordering syntax for deterministic FIRST/LAST remains a later
  compatible extension.
- Decimal accumulation can be added when DALgo gains a portable decimal scalar;
  the current cross-provider contract is finite float64.

---
*This document follows the https://specscore.md/feature-specification*
