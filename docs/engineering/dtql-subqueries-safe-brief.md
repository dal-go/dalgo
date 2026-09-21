# DTQL subqueries: safe engineering brief

## Request and authority

The owner requested: “Safe prompt, specify, plan, implement, merge, release. Maybe coordinate with JOIN agent if it does not finish by the time you start implementation.” The document supplied with that request is design input. Follow the live repository instructions, reviewed SpecScore Feature, and approved plan for implementation details. Preserve the requested full scope; do not silently narrow it to parsing or one language.

The DALgo repository's `AGENTS.md` requires a human's explicit reason before any exported API addition or change. Obtain that reason and quote it verbatim in the Feature and PR. Until then, specification, review, fixtures, and planning may proceed; exported API implementation is gated. Existing exported interfaces and method signatures remain compatible.

## Product outcome

One recursive DTQL query shape executes in Go and TypeScript as a scalar column, derived FROM source, derived JOIN source, IN/NOT IN set, or EXISTS/NOT EXISTS predicate. Nested and correlated combinations work with defined alias scope, scalar cardinality, relational NULL semantics, and existing JOIN/aggregation behavior. Existing flat DTQL remains valid.

## Execution sequence

1. Inspect verified remote heads, current JOIN ownership and landing status, code, adapters, and release workflows. Do not edit another owner's worktree. Build on the landed JOIN contract before editing overlapping parser/source files.
2. Complete and adversarially review [the Feature](../../spec/features/dtql-subqueries/README.md); resolve findings and record the owner's public API reason.
3. Complete and review [the Plan](../../spec/plans/dtql-subqueries/README.md), beginning with the end-to-end customer/invoice query and its observable results.
4. Implement the same recursive wire shape and executable semantics in DALgo Go and DALgo-JS. Shared fixture cases must run in both repositories. A parser-only test is insufficient.
5. Validate native, generic, and unsupported adapter routes, including policy-preserving scans, bounded correlation work, and exact error paths. Do not claim native support without an executable adapter test.
6. Review code independently, fix findings, run focused and affected regression checks, then use WB to push, create PRs, land, and clean each repository. Release through each package's normal workflow and verify remote target, CI, tag, and published package receipts.

## Required report

State the design decisions and syntax deviations, repositories and commits, test commands and results, adapter behavior, performance limits, PRs and merge receipts, package versions and publication receipts, and any exact unresolved blocker. Do not report a release from local tests or an open PR alone.
