---
format: https://specscore.md/plan-specification
status: Draft
---

# Plan: Recursive DTQL subqueries in Go and TypeScript

**Status:** Draft
**Source Feature:** dtql-subqueries
**Date:** 2026-09-21
**Owner:** alex
**Supersedes:** —

## Summary

Specify and release one recursive DTQL subquery model across DALgo Go and DALgo-JS. Build from the landed recursive JOIN contract, prove semantics with shared executable fixtures, and land the affected repositories in dependency order with exact remote and package receipts.

## Approach

The user saves a DTQL document that derives customer rows in FROM, joins aggregate invoice statistics, filters with a correlated EXISTS, and selects a correlated scalar count; both Go and TypeScript validate it, execute it, and show the same ordered rows. A good result at each stage is observable: one canonical document passes schema validation, both parsers bind the same scopes, both executors return identical rows and errors, adapters either execute a verified plan or reject it before output, and released packages contain the landed revisions. The JOIN owner landed Go PR #180 at `c999c7372fd944c57e89dad7e75f9f42c8afaae1` and DALgo-JS PR #3 at `16e58cbfc17ebefc539e66e0eee27311b54c9a6b`; both SHAs were independently verified as exact remote `main` heads. Their ordered `joins[].hints.algorithms` stays a shared wire field in both runtimes. Build compatible model nodes and execution on those commits, run cross-language and adapter acceptance, and exercise the saved document end to end using released packages in a clean consumer workspace. Public API implementation waits for the explicit owner rationale required by `dalgo/AGENTS.md`. Grouped rewrites and SQL lateral optimization are deferred unless needed for correctness; the generic path has bounded work and documents per-distinct-binding cost.

## Tasks

### Task 1: Freeze the recursive contract and review the spec

**Verifies:** dtql-subqueries#ac:recursive-documents, dtql-subqueries#ac:scope-errors
**Status:** in_progress

Finish the SpecScore Feature after an independent adversarial review. Record the owner's verbatim public-API reason before any exported API edit. Settle canonical syntax against Go's current `columns`-last YAML and existing `op` comparisons: `columns: [{query: {...}}]`, `from: {query: {...}}`, JOIN `from.query`, `right.query` under `In`/`NotIn`, and `exists.query`/`notExists.query`. Preserve ordered `joins[].hints.algorithms` from the JOIN contract in both runtimes. Define scalar and relation output names, zero-row behavior, shape errors, lexical scopes, and compatibility. The JOIN PRs are merged; verify the recorded exact main SHAs again when rebasing this task's isolated worktrees.

### Task 2: Create shared semantic fixtures and version their bytes

**Verifies:** dtql-subqueries#ac:recursive-documents, dtql-subqueries#ac:scalar-cardinality, dtql-subqueries#ac:membership-null-table, dtql-subqueries#ac:existence, dtql-subqueries#ac:scope-errors, dtql-subqueries#ac:complex-composition, dtql-subqueries#ac:nested-pipeline
**Status:** planning

Add checked-in YAML and JSON documents, normalized expected rows, and structured expected errors under the Go DTQL tree. Include old flat/JOIN documents with hint lists; scalar zero/NULL/one/many rows and columns; derived FROM/JOIN with aggregates; the inner pipeline with ordering on an unselected local field and separate OFFSET/LIMIT cases; the full IN/NOT IN three-valued table; EXISTS short circuit; shadowing/ambiguity/multi-level correlation; and the complete customer/invoice journey. Pin copied fixture digests in DALgo-JS. Keep fixtures usable by independent CI without sibling checkouts.

### Task 3: Add Go query nodes, scope binding, and serialization

**Verifies:** dtql-subqueries#ac:recursive-documents, dtql-subqueries#ac:scope-errors, dtql-subqueries#ac:scalar-cardinality, dtql-subqueries#ac:derived-sources
**Status:** planning

Add new optional expression, condition, and source node types and constructors; do not extend `StructuredQuery`, `QueryExecutor`, `FromSource`, or `RecordsetSource` method sets. Build one recursion-aware scope binder and contextual shape validator. Preserve old query output, add cycle detection and path-specific diagnostics, update the Go YAML/JSON schema and examples, and run existing DTQL/JOIN/aggregation suites. Treat derived query aliases as relation names and preserve `query.as` as the result name.

### Task 4: Execute nested queries in Go through a validated plan

**Verifies:** dtql-subqueries#ac:scalar-cardinality, dtql-subqueries#ac:derived-sources, dtql-subqueries#ac:membership-null-table, dtql-subqueries#ac:existence, dtql-subqueries#ac:adapter-matrix, dtql-subqueries#ac:complex-composition, dtql-subqueries#ac:policy-preservation, dtql-subqueries#ac:read-routing
**Status:** planning

Reuse the existing generic JOIN and aggregate stages for relation production and the validated DB/transaction records, recordset, and Select entrypoints. Add an inspectable query-specific native/generic/unsupported decision; unknown adapter capabilities remain conservative. Bind serialized field names into internal `(lexical depth, source identity, field identity)` references and execute with stacked row environments. Evaluate uncorrelated nodes once, cache safe repeated outer bindings, short-circuit EXISTS, and preserve access-policy enforcement for every leaf scan. Share a root budget of 10,000 fetched rows, 10,000 materialized result rows, 100,000 candidate evaluations, and 16 MiB retained data across all nesting; return `query_limit` with path/counter before partial output. Implement relational TRUE/FALSE/UNKNOWN composition and scalar cardinality in shared evaluator paths. Test scan counts, policy denials/masks, cancellation, every read route, all four bounds, and returned rows.

### Task 5: Add matching TypeScript model, parser, and executor

**Verifies:** dtql-subqueries#ac:recursive-documents, dtql-subqueries#ac:scalar-cardinality, dtql-subqueries#ac:derived-sources, dtql-subqueries#ac:membership-null-table, dtql-subqueries#ac:existence, dtql-subqueries#ac:scope-errors, dtql-subqueries#ac:complex-composition
**Status:** planning

Start from DALgo-JS merged main `16e58cbfc17ebefc539e66e0eee27311b54c9a6b` and reuse its `JoinedDTQLQuery`, relation, and ordered hint contract. Keep existing flat `StructuredQuery<T>` callers working; add one recursive DTQL query type and an additive execution entrypoint over `QueryExecutor` leaf scans. Apply the same scopes, stages, NULL behavior, root-wide limits, memoization, and diagnostics as Go. Run shared fixture tests plus TypeScript lint, typecheck, build, and package tests. Do not count parser-only acceptance as execution.

### Task 6: Verify adapters and native/fallback boundaries

**Verifies:** dtql-subqueries#ac:adapter-matrix, dtql-subqueries#ac:derived-sources, dtql-subqueries#ac:complex-composition, dtql-subqueries#ac:policy-preservation
**Status:** planning

Inspect each active Go and TypeScript adapter's actual query behavior. Prove native whole-query support with end-to-end tests before advertising it; otherwise use bounded generic scans only where semantics and access policy are preserved, or reject the plan before output. Include SQLite, the existing SQL adapter, and in-use browser adapters in a capability matrix. Add adapter changes to this WB task only when tests show they are needed for a safe route. Document any correlated per-distinct-binding cost.

### Task 7: Adversarial code review and regression

**Verifies:** dtql-subqueries#ac:recursive-documents, dtql-subqueries#ac:scalar-cardinality, dtql-subqueries#ac:derived-sources, dtql-subqueries#ac:membership-null-table, dtql-subqueries#ac:existence, dtql-subqueries#ac:scope-errors, dtql-subqueries#ac:complex-composition, dtql-subqueries#ac:adapter-matrix, dtql-subqueries#ac:policy-preservation, dtql-subqueries#ac:read-routing, dtql-subqueries#ac:nested-pipeline
**Status:** planning

Have an independent reviewer try to break scope binding, mixed NULL logic, alias rules, JOIN/aggregate stage order, adapter claims, compatibility, and unbounded N+1 behavior. Fix or explicitly reject findings in the review record. Run focused and affected regression gates, add concise examples for all eight query uses and a nested combination, and verify package/source compatibility. Prepare the clean-consumer acceptance harness here, then run it against released packages in Task 8.

### Task 8: Land and release with exact receipts

**Verifies:** dtql-subqueries#ac:recursive-documents, dtql-subqueries#ac:complex-composition, dtql-subqueries#ac:adapter-matrix, dtql-subqueries#ac:released-journey
**Status:** planning

Commit and push the approved source branches, create/update PRs, wait for required checks on exact heads, and use `wb worktree land` once per repository in dependency order. Inspect existing version/tag workflows rather than guessing a version. Publish Go module and npm artifacts through the repositories' normal release workflows; verify remote target SHAs, CI checks, tags, package registry artifacts, canonical checkout synchronization, and WB cleanup. After both packages are published, run the standalone clean-consumer acceptance command using only their released versions: load the same saved canonical query and fixture data, parse/bind/execute through each package, and compare normalized output without sibling checkouts. Report any release step that cannot be evidenced as unfinished.

## Open Questions

- The owner's public-API justification and SpecScore human reviewer gate are pending.
- The JOIN owner landed Go PR #180 and DALgo-JS PR #3. Exact remote-main receipts are recorded above; the isolated subquery worktrees still need to incorporate them before source changes. The shared hint field is retained.
- Native adapter coverage is test-dependent. No adapter may claim subquery support solely because it accepts the new AST types.

---
*This document follows the https://specscore.md/plan-specification*
