# Idea: Source-qualified, multi-key ORDER BY resolution in dalgo2memory

**Status:** Draft
**Date:** 2026-06-05
**Owner:** alex
**Promotes To:** —
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we let dalgo2memory order query results by multiple source-qualified ORDER BY keys, resolving each key to the correct recordset, for both single-source and join queries?

## Context

Follow-on to the merged query-joins Feature, which deferred OrderBy/Columns qualifier resolution as an explicit open question. Today dalgo2memory's single-source sortRows sorts by a single bare field name (orderBy[0], ignoring FieldRef.Source()), and the join executor (dalgo2memory/join.go) ignores OrderBy entirely, sorting only by base record id. A join ORDER BY on a source-qualified field (e.g. o.createdAt) therefore has no effect, and a qualified field under a name collision (u.status vs o.status) cannot be ordered correctly because the merged output map overlays one over the other. The qualified-field resolver already exists for WHERE in the join path (resolveJoinExpr over a per-source data map). Columns projection is a separate, larger gap: dalgo2memory never consumes q.Columns() at all (single-source or join), so it is out of scope here and parked as its own idea.

## Recommended Direction

Build one source-aware, multi-key ORDER BY comparator and use it for both paths. Reuse the existing per-source resolution (empty Source() -> From base; non-empty -> the recordset whose Alias()/Name() matches) that WHERE already uses, and reuse the existing compare() helper for value ordering. The comparator walks the OrderBy expressions in order, comparing the resolved value of each key until one differs, honoring Descending() per key, with the base record id as a final deterministic tiebreak. For single-source rows the same comparator resolves an empty-source field against the one base recordset, so sortRows and the join sort collapse into a shared implementation rather than two divergent ones. This completes the query-joins Feature's deferred OrderBy resolution, keeps in-memory behavior a faithful preview of what the SQL adapters will later render, and leaves Columns projection as a clean separate effort.

## Alternatives Considered

- **Join-only ORDER BY, leave `sortRows` untouched.** Smaller blast radius — the working single-source path is not modified. Lost because it leaves two divergent sort implementations (a single-key, name-only one for single-source and a new source-aware one for joins) that will drift; an empty `Source()` already means "the base", so one comparator serves both with no behavior change to single-source.
- **Order the merged output map instead of the per-source data.** The join executor already builds a flat merged map per row, so sorting by `merged[name]` is the least code. Lost because the merge overlays the joined source over the base, so a qualified key under a name collision (`u.status` vs `o.status`) would silently sort by the wrong source's value — exactly the ambiguity source qualification exists to remove.
- **Bundle Column projection in now (resolve OrderBy and Columns together).** Both are the same deferred open question and share the resolver. Lost because `q.Columns()` is unimplemented adapter-wide (not just for joins), so it is a build-a-feature effort, not a resolution one; folding it in triples the scope and delays the small, well-defined OrderBy win.

## MVP Scope

A short spike: dalgo2memory orders both join and single-source results by one or more source-qualified ORDER BY keys, ascending or descending per key, resolving each key to its recordset (empty source -> base; alias/name -> that source), with a deterministic base-id tiebreak. Verified by table tests: a join ordered by a qualified key (including under a u/o name collision), a multi-key order (primary asc, secondary desc), and existing single-source ordering still green via the unified comparator. dalgo2memory stays at 100% coverage.

## Not Doing (and Why)

- Column projection in dalgo2memory — q.Columns() is unimplemented adapter-wide; separate idea (parked as a sidekick seed)
- ORDER BY on non-field expressions (functions, constants, computed) — only FieldRef keys are in scope
- NULLS FIRST/LAST control — absent/zero values fall out of Go's natural compare() ordering
- SQL adapter ordering (dalgo2sql, dalgo2sqlite) — separate repos, unblocked by the shared semantics not the code
- Self-joins / 3+ table ordering — single two-table join, matching the query-joins MVP

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | The per-source resolution WHERE already uses (empty `Source()` -> base; alias/name -> source) can back ORDER BY too, with no new aliasing machinery. | Build the comparator on the existing resolver and order a join by a qualified key under a `u`/`o` name collision; the correct source's value drives the order. |
| Must-be-true | Folding single-source ordering into the source-aware comparator does not change existing single-source results. | Run the existing `dalgo2memory` ordering tests unchanged; an empty-`Source()` field must resolve to the one base recordset and sort identically to today. |
| Should-be-true | `compare()` plus key-by-key short-circuit gives deterministic multi-key ordering across mixed value types. | Table test: order by (numeric asc, string desc); assert stable, total order with the base-id tiebreak. |
| Should-be-true | An ORDER BY key naming no recordset should error, consistent with the join WHERE `unresolvable-source` behavior. | Decide and test: a qualified key with an unknown source returns a descriptive error rather than silently sorting by base id. |
| Might-be-true | Consumers (DTQL, datatug) actually need multi-key join ordering now rather than single-key. | Review intended consumers before committing to multi-key over the simpler single-key match to today's `sortRows`. |


## SpecScore Integration

- **New Features this would create:** A `dalgo2memory` ORDER BY resolution Feature (or a revision extending `query-joins`) covering the unified source-aware, multi-key comparator.
- **Existing Features affected:** `query-joins` — completes its deferred OrderBy open question and replaces the join executor's base-id-only sort; the single-source `sortRows` is unified into the shared comparator.
- **Dependencies:** Upstream — the merged `query-joins` Feature (the qualified-field resolver and join executor). Sibling — the parked Column-projection idea, which shares the same resolver but is independent.

## Open Questions

- Does a qualified ORDER BY key whose source names no recordset error (matching the join WHERE `unresolvable-source-errors` behavior) or fall back to the base-id tiebreak? Lean toward error for consistency.
- For a single-source query, how is a non-empty `Source()` that does not match the base treated — error, or resolve as base? Settle at spec time.
- Where the comparator lives so both `ExecuteQueryToRecordsReader` (single-source) and `executeJoinQuery` share it without exposing internals.

---
*This document follows the https://specscore.md/idea-specification*
