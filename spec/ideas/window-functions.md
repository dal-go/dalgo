---
format: https://specscore.md/idea-specification
status: Draft
---

# Idea: Window functions in DTQL

**Status:** Draft
**Date:** 2026-10-04
**Owner:** alex
**Promotes To:** —
**Supersedes:** —
**Related Ideas:** depends_on:query-group-by-aggregation, depends_on:first-class-query-joins

## Problem Statement

How might DTQL express window functions (ranking, running totals, lag and lead) so
that a server which supports them computes them natively, and DALgo computes them in
memory for providers that do not?

## Context

The founder's words, 2026-10-04: "We also probably should add support for native
analytical window functions", "Window furncrioms should be supported by DTQL" and
"I'd also add window functions on dalgo side for providers that do not support them
natively". These three sentences are the founder's words in this idea; the first is
hedged ("probably"). Everything else below is the PostgreSQL launch plan's design or
this idea's own proposal, marked as such.

Today DTQL has no window functions, and `query-group-by-aggregation` lists them as out
of scope. Every DTQL feature exists in two engines: native in the SQL compiler and in
the bounded generic engine in DALgo. Aggregation functions already run natively on
servers that support them (founder, 2026-10-04: "Aggregation functions should be run
natively on servers that support them."); windows follow the same rule.

## Recommended Direction

Use one spelling for an order inside an expression: DTQL `orderBy` items carry
an expression and optional `desc`, and the Go model uses `[]dal.OrderExpression`.
The aggregate-local order and a future window order share that shape; a window
adds `partitionBy` without introducing a second order syntax. A future NULL
placement option belongs on the shared order item.

Founder rulings: window functions are part of DTQL, and DALgo computes them in memory
for providers without native support. Native execution where the server supports
windows rests on the founder's hedged "We also probably should add support for native
analytical window functions" and on the aggregation rule; it is not an unhedged ruling.

Sequencing (the plan's design, not a founder ruling): after PostgreSQL parity. The
launch window ends 2026-10-25 and the document format is being frozen; adding
document nodes now would change it just before the freeze.

Seam now (the plan's design): the typed SQL compiler in dalgo2sql reserves one window
hook on its dialect and a planned capability flag. No window syntax is added to DTQL
yet.

Delivery, in order (the plan's design):

1. Seam: reserved dialect hook and capability flag; this idea. About half a day.
2. Native: document nodes, model, schema and validation; compilation in both
   dalgo2sql compilers (SQLite and the typed one); an explicit "unsupported" error
   from the generic engine and the TypeScript engine; parity cases. 9 to 12
   agent-days across dalgo, dalgo2sql and dalgo-js.
3. In memory: a window executor in the DALgo generic engine and in the TypeScript
   engine (dalgo-js) for providers without native support, within the same row bounds
   as joins and aggregation. Not sized separately; the plan sizes native plus
   in-memory in both engines together at three to four weeks.

## Alternatives Considered

- **Native only, no in-memory fallback.** Cheapest, but a provider without windows
  would simply fail. Overruled by the founder's third sentence (window functions on the DALgo side for providers without native support).
- **Full support (native plus in-memory in the Go and TypeScript engines) before launch.** Three to four weeks; the launch
  window is three weeks and focus is the priority. Rejected on schedule.
- **No seam until later.** Costs a hook change in a compiler written weeks earlier.

## MVP Scope

The seam only: a reserved dialect method in dalgo2sql and this written idea. First
useful slice after launch: ROW_NUMBER, RANK, SUM OVER and LAG over one partition and
one ordering, native on PostgreSQL and SQLite (Proposal: this idea's own, not in the
plan).

## Not Doing (and Why)

- Window syntax in DTQL before the PostgreSQL launch — the format is being frozen and
  the work is sequenced after parity.
- Proposal: frame clauses (ROWS or RANGE BETWEEN) in the first slice — most uses need
  only partition and order.
- Proposal: window functions inside access-policy conditions — policies are row-level
  filters.

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | The in-memory executor fits the generic engine's row bounds without a new bound. | Prototype ROW_NUMBER over the existing bounded row set. |
| Must-be-true | Native results match in-memory results for the same query. | Shared parity fixtures run on both paths. |
| Must-be-true | A window whose PARTITION BY, ORDER BY or argument names a field hidden by a field-restricted policy is denied on both the native and in-memory paths; otherwise ranking leaks the hidden field. | Negative cases in the access conformance suite before the first slice ships. |
| Should-be-true | One dialect hook is enough for the SQL spelling differences. | Compare PostgreSQL and SQLite window syntax when the first slice starts. |

## SpecScore Integration

- **New Features this would create:** a DTQL window-functions feature and a generic
  engine window executor feature, to be specified after PostgreSQL parity.
- **Existing Features affected:** `query-group-by-aggregation`, `query-joins`.
- **Dependencies:** the PostgreSQL typed compiler in dalgo2sql.

## Open Questions

- Which window functions ship first, and are frame clauses needed for launch users?
- Does the in-memory executor share the aggregation row bound or get its own?

---
*This document follows the https://specscore.md/idea-specification*
