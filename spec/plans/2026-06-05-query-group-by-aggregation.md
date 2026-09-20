---
format: https://specscore.md/plan-specification
status: Implemented
---

# Plan: Provider-independent GROUP BY and aggregation

**Status:** Implemented
**Source Feature:** query-group-by-aggregation
**Date:** 2026-09-20
**Owner:** alex
**Supersedes:** —

## Summary

Extend the existing DALgo grouping feature across the YAML codec, validation,
capability planning, generic ordered/hash execution, SQLite native rendering,
parity tests, and public reference documentation.

## Approach

Preserve the existing StructuredQuery surface and AggregateFunc interface.
Add optional expression/capability extensions, central validation and an
inspectable physical plan. Intercept only aggregate queries in `dal.NewDB`, so
ordinary reads remain unchanged. Keep SQL as one provider implementation, not
as the source of DTQL semantics.

## Tasks

### Task 1: Extend expression and validation model

**Verifies:** query-group-by-aggregation#ac:all-functions, query-group-by-aggregation#ac:invalid-selection, query-group-by-aggregation#ac:aliases, query-group-by-aggregation#ac:deterministic-first-last
**Status:** complete

Add DISTINCT, FIRST/LAST, star introspection, arithmetic expressions, grouping
validation, aliases and deterministic-order validation without breaking the
existing AggregateFunc interface.

### Task 2: Extend canonical DTQL YAML

**Verifies:** query-group-by-aggregation#ac:yaml-round-trip, query-group-by-aggregation#ac:omitted-projection
**Depends-On:** 1
**Status:** complete

Add recursive aggregate/binary shapes, `groupBy`, `having`, schema generation,
structural equality, validation and canonical `columns`-last examples.

### Task 3: Add granular planning

**Verifies:** query-group-by-aggregation#ac:native-plan, query-group-by-aggregation#ac:streaming-plan, query-group-by-aggregation#ac:hash-plan
**Depends-On:** 1
**Status:** complete

Add provider capability structs and an inspectable native/ordered/hash physical
strategy with conservative defaults.

### Task 4: Implement generic local aggregation

**Verifies:** query-group-by-aggregation#ac:all-functions, query-group-by-aggregation#ac:streaming-plan, query-group-by-aggregation#ac:hash-plan, query-group-by-aggregation#ac:unsafe-limit
**Depends-On:** 3
**Status:** complete

Build per-aggregate states, typed composite group keys, ordered incremental
finalization, hash fallback, DISTINCT limits, aliases, HAVING, final ordering,
pagination, and implicit empty grouping for records and recordsets.

### Task 5: Preserve logical stage order

**Verifies:** query-group-by-aggregation#ac:stage-order, query-group-by-aggregation#ac:unsafe-limit, query-group-by-aggregation#ac:aliases
**Depends-On:** 4
**Status:** complete

Rewrite local source queries to retain WHERE, clear result-stage operations,
request grouping order only for streaming, and apply HAVING/order/offset/limit
to finalized group rows.

### Task 6: Implement SQLite native aggregation

**Verifies:** query-group-by-aggregation#ac:native-plan, query-group-by-aggregation#ac:aliases
**Depends-On:** 1, 3
**Status:** complete

Render aggregate and arithmetic expressions, GROUP BY, HAVING, alias rewrites,
DISTINCT and final ordering; suppress row-identity projection for aggregate
results and advertise only the supported native subset.

### Task 7: Verify lifecycle and parity

**Verifies:** query-group-by-aggregation#ac:cancellation-errors, query-group-by-aggregation#ac:streaming-plan, query-group-by-aggregation#ac:hash-plan, query-group-by-aggregation#ac:native-plan, query-group-by-aggregation#ac:strategy-parity
**Depends-On:** 4, 6
**Status:** complete

Test cancellation and mid-stream failures, one identical dataset/query across
native SQLite, ordered streaming and hash execution, plus the
requested-order-versus-group-order case.

### Task 8: Publish semantics and limitations

**Verifies:** query-group-by-aggregation#ac:yaml-round-trip, query-group-by-aggregation#ac:all-functions, query-group-by-aggregation#ac:deterministic-first-last
**Depends-On:** 2, 4, 6
**Status:** complete

Document YAML examples, null/empty/type semantics, execution strategies,
resource safeguards, collation caveats, and deferred aggregate-local ordering.

## Open Questions

- Add a portable decimal scalar before offering decimal-preserving SUM/AVG.
- Add aggregate-local order expressions before expanding deterministic
  FIRST/LAST pushdown.

---
*This document follows the https://specscore.md/plan-specification*
