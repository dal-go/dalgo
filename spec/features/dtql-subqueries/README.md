---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: Recursive DTQL subqueries across DALgo Go and TypeScript

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dtql-subqueries?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dtql-subqueries?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dtql-subqueries?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dtql-subqueries?op=request-change) |
**Status:** Implementing
**Source Ideas:** —

## Summary

One recursive DTQL query contract for scalar projection, derived FROM and JOIN sources, IN/NOT IN, and EXISTS/NOT EXISTS. The same serialized query has executable semantics in DALgo Go and DALgo-JS, including correlation, cardinality, and SQL-style NULL behavior.

## Problem

DTQL currently serializes a single query whose `from` may contain recursive JOINs, but a query cannot itself be a source, expression, or predicate operand. Go has a richer `dal.StructuredQuery` and generic JOIN/aggregation paths; DALgo-JS currently exposes a flat `StructuredQuery` and a Phase 1 DTQL parser that does not execute queries. Adding only YAML syntax would leave consumers with documents that parse but cannot run, and adding separate query variants would make composition and parity fragile.

DALgo's repository rule requires a human's explicit, verbatim reason before adding exported query APIs. Alex approved the following reason on 2026-09-21: “First-class subqueries are needed for composable DTQL across DALgo Go and TypeScript; additive public query APIs are acceptable while existing queries remain compatible.” Existing exported interfaces and method signatures must remain source compatible.

## Behavior

### Recursive representation

#### REQ: one-query-shape

Every nested query MUST use the same recursive clause set as a top-level DTQL query: `as`, `from`, `where`, `groupBy`, `having`, `orderBy`, `limit`, `offset`, and final `columns`. The canonical Go serializer MUST retain `columns` at the end. Existing documents MUST remain parseable with equivalent semantics, and canonical output for pre-existing queries MUST remain byte-identical to the old serializer's output; original user formatting need not be retained. `as` belongs inside a nested query when its result is named; table aliases retain the current canonical `alias` field and accepted input synonym `as`. A nested query MUST NOT be represented by a reduced context-specific AST.

#### REQ: contextual-shape

Context determines query result shape. `from.query` and `join.from.query` provide relations and require a nonempty result alias. A `columns` item with `query` is scalar and its `query.as` is the output name. The right operand of `op: In` or `op: NotIn` with `query` provides a one-column set. `exists.query` and `notExists.query` test row presence and MAY omit `columns`. Every query form MUST retain the ordinary query clauses, subject only to contextual shape checks. A derived relation with omitted `columns` expands a wildcard only when schema metadata makes every output name known; a scalar or IN query requires exactly one explicit named or field-derived column; EXISTS ignores projected values.

#### REQ: wire-discriminators

A query mapping orders keys as `as`, `from`, `where`, `groupBy`, `having`, `orderBy`, `limit`, `offset`, `columns`. A source mapping contains exactly one of `name` (table) or `query` (derived result), plus optional outer `joins`; `schema` and table `alias`/`as` are legal only with `name`. Derived result alias is `query.as`, not a sibling of `query`, and `query.as` must not collide with another alias in its enclosing scope. A column/expression mapping contains exactly one of its existing discriminators or `query`; a scalar column's output name is `query.as`, while a non-query column keeps its existing `as`. A predicate contains exactly one of comparison `op`+`left`+`right`, `and`, `or`, `exists`, or `notExists`. `exists` and `notExists` each contain only `query`. `In`/`NotIn` require one `right` discriminator: existing `values` or new `query`; other comparisons retain their existing RHS forms. Unknown keys, mixed discriminators, missing required parts, or recursive YAML aliases are errors at their exact path. JSON keys have the same spelling and shape.

Canonical examples (additional normal clauses may appear inside every `query`):

```yaml
from:
  name: Customer
  as: c
where:
  and:
    - op: In
      left: {field: CustomerId, source: c}
      right:
        query:
          from: {name: Invoice, as: i}
          columns: [{field: CustomerId, source: i}]
    - exists:
        query:
          from: {name: Invoice, as: i}
          where:
            op: ==
            left: {field: CustomerId, source: i}
            right: {field: CustomerId, source: c}
columns:
  - field: CustomerId
    source: c
  - query:
      as: InvoiceCount
      from: {name: Invoice, as: i}
      where:
        op: ==
        left: {field: CustomerId, source: i}
        right: {field: CustomerId, source: c}
      columns: [{aggregate: {function: count, args: [{star: true}]}}]
```

`NotIn` uses the same comparison shape. `notExists` uses the same unary shape. A derived base is `from: {query: {as: stats, from: ..., columns: [...]}}`; a derived JOIN uses the same `query` source under `joins[].from`, with ordinary `type` and `on` siblings. JOIN edges may also carry the existing ordered `hints.algorithms` preference list; it has the same accepted values, snapshot behavior, and canonical emission in both runtimes after the JOIN hint releases land. Canonical emission uses `alias` for table sources, although `as` remains accepted input.

#### REQ: recursive-serialization

Go and TypeScript MUST read equivalent YAML/JSON documents, reject unknown or conflicting discriminator keys, detect recursive YAML alias cycles, and produce stable canonical documents. Existing flat and JOIN documents MUST continue to deserialize unchanged. `dal-go/dalgo/dtql/testdata/subqueries/` owns versioned fixture inputs, expected rows, and expected errors; DALgo-JS vendors their exact bytes with a SHA-256 manifest that CI verifies. Normalized rows are ordered JSON arrays of field maps: absent fields stay absent, SQL NULL/Go nil/JS null become JSON null, finite numbers compare by numeric value, and output names remain case-sensitive. Expected errors carry stable `category` and `path` plus an explanatory message fragment; human wording may vary. Go and JS CI run the same case list independently without sibling checkouts.

### Expressions, sources, and scopes

#### REQ: scalar-projection

A scalar `columns` query MUST select exactly one output column and produce at most one row. Zero rows yield NULL, one row yields its value including NULL, and more than one row produces a contextual cardinality error. Existing aggregate implementations apply inside nested queries without a special `COUNT(query)` form.

#### REQ: derived-relations

A derived query MAY be the base `from` or any recursive JOIN source. Its projected names form the visible relation schema under its result alias. A field projection keeps its field name unless aliased; a computed/aggregate projection in a derived relation requires a name; duplicate names are rejected. Wildcards expand only with a known schema and reject duplicate output names. INNER and LEFT JOIN semantics, nested ON scope, grouping, HAVING, ordering, and projection must compose with derived sources. Query sources MUST use the ordinary source abstraction, with additive public API only after the repository's human-justification requirement is met.

Each query node executes its own logical pipeline: build FROM/JOIN relation, apply WHERE, group and aggregate, apply HAVING, order, apply OFFSET then LIMIT, and finally project columns. An inner query completes that pipeline before its relation or scalar result enters the enclosing query. An inner ORDER BY may use an unprojected local field; LIMIT/OFFSET affect its result before the outer JOIN. EXISTS honors WHERE, grouping, HAVING, ordering, offset, and limit for row existence while omitting value projection.

#### REQ: lexical-correlation

Field resolution MUST search the innermost query scope first, then each enclosing scope in order. Within a scope, an unqualified field with multiple candidate sources is ambiguous; a missing field or qualifier is an error with its query path. A local alias shadows an outer alias, and qualified references never skip a matching local alias. Multi-level outer references are valid, including in nested scalar, IN, and EXISTS queries. JOIN right operands see only aliases legally available at their position. The serialized AST retains `field`/`source` names; a non-serialized compiled binding records lexical depth, source identity, and resolved field identity. Evaluation receives a stack of row environments, so a depth-2 binding reads the second enclosing row without rewriting strings. The binder records free outer bindings for correlation/caching and rejects cyclic query object graphs before recursion.

### Predicates and NULL

#### REQ: in-not-in

`op: In` and `op: NotIn` MUST accept a `right.query` with exactly one selected column, including correlated and nested queries. Membership uses relational three-valued logic: a match is TRUE; no match with a NULL comparison is UNKNOWN; otherwise FALSE. Empty RHS makes IN FALSE and NOT IN TRUE, even for a NULL LHS. NOT IN negates the three-valued result, not a host-language boolean. The subquery evaluator uses one internal TRUE/FALSE/UNKNOWN value: NOT maps UNKNOWN to UNKNOWN; AND is FALSE if either input is FALSE, TRUE only if both are TRUE, otherwise UNKNOWN; OR is TRUE if either is TRUE, FALSE only if both are FALSE, otherwise UNKNOWN. Comparisons involving NULL yield UNKNOWN; `IN` follows the stated match/NULL/empty rules. JOIN ON, WHERE, and HAVING keep only TRUE. The new evaluator MUST use this truth model for all conditions in a query it executes, while pre-existing queries on their old execution route retain observed behavior. Shared fixtures cover every AND/OR/NOT combination used by nested predicates.

#### REQ: exists-not-exists

`exists.query` and `notExists.query` MUST accept correlated and non-correlated queries. They test row presence irrespective of projection values, so a row containing NULL satisfies EXISTS. When `columns` is omitted, the executor MUST avoid projecting values, and generic execution MUST stop after the first qualifying row. NOT EXISTS is the logical complement of row existence.

### Execution and compatibility

#### REQ: execution-routing

The supported Go framework entrypoints are `dal.DB.ExecuteQueryToRecordsReader`, `dal.DB.ExecuteQueryToRecordsetReader`, and `Select` on the validated DB plus validated read and read-write transactions. The TypeScript path is a new additive DTQL execution entrypoint over the existing leaf `QueryExecutor`; legacy `query(StructuredQuery<T>)` remains unchanged. Direct raw-adapter calls must explicitly reject an unknown nested AST, and are not covered by the framework planner. A provider MAY claim whole-query native execution only for shapes it implements faithfully via a query-specific optional capability probe; a coarse `QueryCapabilities` flag or a JOIN-only claim is insufficient. Otherwise DALgo MAY execute through bounded relation scans using the same authorized session that accepted the outer query. Policy is checked for every leaf relation; row filters and masks apply before a row can join, correlate, or become a scalar result. If the current adapter/session cannot prove those semantics, the plan is unsupported before any output. Unknown capability MUST never imply native support. SQLite/SQL and in-use browser adapters need actual positive or negative execution tests, not structural typing alone.

#### REQ: correlation-cost

The planner MUST mark correlated query nodes and identify their outer bindings. Uncorrelated inner results MUST be evaluated once per query; repeated binding tuples SHOULD reuse results where safe. Generic execution uses one root-query budget across all nesting: at most 10,000 fetched leaf rows, 10,000 materialized result rows, 100,000 candidate evaluations, and 16 MiB retained data. A cached result charges fetched rows and retained bytes once; each use still charges candidate work. Exceeding any counter returns `query_limit` with the responsible nested query path and counter name before partial output. Cancellation stops all nested work, closes readers, and returns the context/abort error without partial rows. Native whole-query execution is preferred when verified. Safe grouped rewrites/batching are optional optimizations; their absence and any remaining per-distinct-binding cost MUST be documented.

#### REQ: aggregate-and-existing-behavior

Nested queries MUST reuse existing COUNT, COUNT DISTINCT, SUM, AVG, MIN, MAX, FIRST, LAST, grouping, HAVING, and ordering semantics. Ordinary flat DTQL, existing recursive JOINs, negative projection, and aggregation MUST keep their serialized and runtime behavior. Additions MUST NOT extend existing exported Go interfaces or require existing TypeScript callers to supply new fields. Go adds optional concrete nodes/accessors; TypeScript extends the JOIN branch's distinct `JoinedDTQLQuery` route into one recursive DTQL query AST, while keeping legacy flat `StructuredQuery<T>` and `QueryExecutor.query` intact. A new execution function handles recursive DTQL so old adapters do not silently accept unsupported trees. Changes to public types/functions await the required human rationale, and version/release notes must explain all additive surfaces.

#### REQ: diagnostics

Validation MUST distinguish syntax, scope, shape, cardinality, capability, and work-limit errors, with the nested query path and expected shape. It MUST reject structurally invalid nesting, alias conflicts, one-column violations, scalar multi-row results, and unsupported adapter plans without silently truncating or choosing an arbitrary row.

## Acceptance Criteria

### AC: recursive-documents (verifies REQ:one-query-shape, REQ:wire-discriminators, REQ:recursive-serialization)

Given existing flat and recursive JOIN DTQL plus nested examples in YAML and JSON
When Go and TypeScript parse and serialize them
Then old documents retain their meaning and canonical output, while every nested context round-trips through the same recursive query shape.

### AC: scalar-cardinality (verifies REQ:scalar-projection, REQ:diagnostics)

Given independent, correlated, aggregate, nested, zero-row, NULL, one-row, two-row, and two-column scalar queries
When each is executed in both runtimes
Then values and NULLs agree, and invalid cardinalities return path-specific errors rather than a selected arbitrary value.

### AC: derived-sources (verifies REQ:derived-relations, REQ:lexical-correlation)

Given simple, aggregate, and nested derived FROM queries plus INNER and LEFT JOINs to query sources
When each is executed with aliases, filters, ordering, and projected derived columns
Then both runtimes return the same relation and preserve LEFT unmatched rows.

### AC: nested-pipeline (verifies REQ:derived-relations)

Given derived queries that respectively order on an unprojected local field, aggregate, and apply OFFSET/LIMIT before becoming outer JOIN sources
When both runtimes execute the document
Then each applies the inner pipeline before joining and returns the same ordered outer rows.

### AC: membership-null-table (verifies REQ:in-not-in)

Given matching, missing, empty, NULL-left, NULL-right, and mixed-NULL one-column query results, including correlation and AND/OR compositions
When IN and NOT IN are evaluated
Then both runtimes implement the same TRUE/FALSE/UNKNOWN table and reject a multi-column query.

### AC: existence (verifies REQ:exists-not-exists)

Given empty and nonempty queries, a NULL-valued row, nested and multi-level correlated queries
When EXISTS and NOT EXISTS run with or without `columns`
Then presence alone determines the result and generic execution stops once presence is established.

### AC: scope-errors (verifies REQ:lexical-correlation, REQ:diagnostics)

Given shadowed aliases, ambiguous unqualified fields, unresolved qualifiers, invalid forward JOIN references, and valid multi-level outer references
When each query is validated
Then both runtimes either bind the same scope or return equivalent category and path errors.

### AC: complex-composition (verifies REQ:one-query-shape, REQ:derived-relations, REQ:aggregate-and-existing-behavior)

Given one query with a derived FROM, aggregate query JOIN, correlated EXISTS filter, and correlated scalar projection
When it is executed against a checked-in fixture
Then Go and TypeScript produce identical ordered normalized rows and the existing JOIN, aggregation, grouping, HAVING, filter, and serialization suites still pass.

### AC: adapter-matrix (verifies REQ:execution-routing, REQ:correlation-cost, REQ:diagnostics)

Given native-capable, generic-scan-capable, and unsupported adapters
When a correlated or independent nested query is planned and executed
Then each uses only a verified route, uncorrelated work is not repeated, each root-wide budget counter and cancellation fail before partial output with stable diagnostics, and neither adapter claims unimplemented semantics.

### AC: policy-preservation (verifies REQ:execution-routing)

Given an authorized session with row denial, field masks, and a source that cannot offer authorized scans
When a nested query reads each source through native or generic execution
Then denied rows and masked fields never influence output, the unscannable plan is rejected before output, and raw adapter paths reject unknown nested nodes.

### AC: read-routing (verifies REQ:execution-routing)

Given a nested query through each supported validated DB and transaction records, recordset, and Select entrypoint
When execution starts
Then every entrypoint uses the same validation/plan and returns equivalent results or equivalent unsupported diagnostics.

### AC: released-journey (verifies REQ:recursive-serialization, REQ:execution-routing)

Given the same saved canonical customer/invoice DTQL document and fixture data in a clean consumer workspace containing only released DALgo Go and DALgo-JS packages
When a standalone acceptance command parses, binds, and executes the query through both packages
Then it compares their normalized rows and errors with the checked-in expected output without using sibling source checkouts.

## Open Questions

- Alex approved the public-API reason quoted in the Problem section on 2026-09-21. Record it verbatim in the PR description before exporting Go constructors/types or TypeScript query contracts.
- Native SQL, SQLite, and browser adapter capability coverage must be decided from adapter implementation tests. Generic fallback is required only where scans preserve policy and work limits.

## Review resolution

The independent review found six blocking gaps and four serious gaps in the first draft. The first revision defined the wire discriminator matrix, resolved binding representation, complete truth-value model, authorized-scan boundary, exact framework entrypoints, result naming, fixture ownership/normalization, and adapter verification matrix. The second review found a moving JOIN hint contract, missing inner pipeline and work-budget detail, and a missing released-package journey. This revision includes ordered JOIN hints in the shared wire contract, defines the per-query pipeline and root-wide budgets, and adds the standalone released-package acceptance. The TypeScript migration builds on the active JOIN AST while preserving legacy flat query consumers. The independent final review resolved its findings, and Alex approved the feature and plan on 2026-09-21.

---
*This document follows the https://specscore.md/feature-specification*
