---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Recursive DTQL JOINs across DALgo Go and TypeScript

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dtql-recursive-joins?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dtql-recursive-joins?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dtql-recursive-joins?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dtql-recursive-joins?op=request-change) |
**Status:** Draft
**Source Ideas:** —

## Summary

DTQL defines an ordered recursive `from` relation tree with `inner` and `left` joins. DALgo Go and DALgo-JS parse, validate, and execute the same tree without changing existing single-source documents. Per-JOIN ordered algorithm hints express execution preferences without changing results. The supplied [JOIN brief](../../../docs/engineering/dtql-recursive-joins-brief.md) and [algorithm hint brief](../../../docs/engineering/dtql-join-algorithm-hints-brief.md) record the originating design intent.

## Problem

The Go AST and memory adapter support one flat join; Go DTQL rejects joins. DALgo-JS has a flat query model, and its unlanded browser parser supports only single-source DTQL. A query cannot express the required nested Invoice/Customer/Employee relation, and adapters could otherwise ignore unsupported joins. This Feature extends the authoritative DTQL contract and requires explicit execution or rejection.

## Behavior

### Declarative relation tree

#### REQ: recursive-from

`from` MUST be one relation node containing a required root collection identity (`name`, optional `schema`), optional alias, and ordered `joins` sequence. Parented collections and collection groups remain outside DTQL's current subset. Each JOIN MUST contain a recursively shaped `from`, whose children may themselves have multiple ordered joins. There is no top-level `joins` key or textual SQL syntax. Missing/empty `joins` preserves existing single-source behavior. The grammar leaves room for a future `from.ref` and cross-source identity without implementing them now.

#### REQ: join-shape

Each JOIN has optional `type`, required `from`, and nonempty `on` sequence. Omitted type means `inner`; only `inner` and `left` execute in MVP. Each ON item has `left`, `op`, `right`; both operands MUST be qualified field references from the own subtree or the parent-visible scope, in either orientation. Equality is required and multiple items are ANDed. Fields use Go DTQL's canonical `{field: CustomerId, source: c}` form. `op: '=='` is canonical output and `op: eq` an accepted input alias. Existing `alias` remains canonical output, and `as` is an accepted input alias; both spellings together are invalid. DTQL lowercase join types map to existing uppercase Go `dal.JoinType` values without changing them. Unsupported types/operators fail.

#### REQ: round-trip-and-schema

Go DTQL MUST serialize one stable canonical YAML form, deserialize it losslessly, accept equivalent YAML/JSON, update its generated JSON Schema and examples, and preserve existing documents. Go and TypeScript MUST agree on defaults, type names, ON, aliases, and error **category and path**, though human-readable wording may differ. Stable categories are `join_shape`, `join_type`, `join_operator`, `join_scope`, `join_field`, `join_cycle`, `join_key_type`, and `join_plan`; paths use zero-based notation such as `from.joins[1].from.joins[0].on[0]`. Recursive YAML aliases or cyclic in-memory/JSON-like objects MUST fail before unbounded traversal; finite nesting has no arbitrary depth cap.

### Scope and semantics

#### REQ: ordered-scope

At a relation node, begin with parent-visible aliases and the node's own alias. Evaluate child joins in sequence. A child's ON may reference aliases from its own joined subtree and those already visible in its parent scope. Once a child completes, its subtree aliases become visible to later siblings. A child and its nested ON MUST NOT see later siblings or a relation outside this lexical chain. All explicit and implicit aliases MUST be unique across the tree. A predicate whose two sides use only one scope is valid; execution evaluates it for each candidate combination, subject to the generic executor's row and byte bounds. Unknown, duplicate, and forward aliases produce path-specific errors.

#### REQ: field-resolution

Qualified fields use `{field: <name>, source: <alias-or-unaliased-name>}`. ON always requires qualification. WHERE, GROUP BY, HAVING, ORDER BY, and explicit projection may use an unqualified field only when schema metadata available at validation proves exactly one visible owner; without it, validation fails before execution. Existing Go API calls with empty `FieldRef.Source()` retain base-source meaning for compatibility; DTQL validation is stricter. Schema-known missing fields or safely detectable incompatible key types fail before execution. An unknown qualifier never silently resolves to the base.

#### REQ: nested-semantics

A JOIN's right subtree is evaluated as a unit against each current left combination, then the enclosing ON filters combinations. INNER emits all matching pairs and drops unmatched left combinations. LEFT emits all matching pairs or one left combination with every alias in the right subtree absent. Thus `A LEFT (B INNER C)` preserves unmatched A, while `(A LEFT B) INNER C` may remove it. Sources merge in depth-first, left-to-right order: for `A LEFT (B LEFT C)`, visit A, B, C; if B has no match, B and C are absent together. Null, absent, and JS `undefined` ON keys never match, including null-to-null. Portable key values are strings, booleans, and finite numbers; any mathematically integral numeric value must be within the JavaScript safe integer range (±(2^53−1)). Integer and floating representations of the same numeric value compare equal, but numbers never equal strings or booleans. Unsafe integers, NaN, infinity, dates, bytes, arrays, and objects raise `join_key_type` unless a future typed coercion contract is added. Every fetched row's referenced ON keys are validated before JOIN construction and before query WHERE, even if that row later produces no result; an absent null-extended side is exempt. Composite ON keys compare componentwise. Parent-visible aliases may be read by nested ON without one provider query per parent row.

#### REQ: results-and-pipeline

Joined rows retain a per-alias map for expression evaluation. Explicit `columns` flatten into the existing record map: column `as` determines the output key, otherwise a field's name does. Every non-field expression in a JOIN query (aggregate, arithmetic, constant, parameter) MUST have `as`. Duplicate output keys are invalid, including collisions after wildcard expansion; a projected field from an unmatched LEFT side is present with null (`nil` in Go, `null` in JSON/JS). In JOIN queries a wildcard exclusion MUST name its source. It expands that source's schema fields in schema order, drops exclusions (including absent names), and fails with `join_plan` if the source schema is unavailable. Without columns, preserve Go's flat merged output: later depth-first sources overwrite earlier duplicate names and unmatched-side keys are absent. Non-JOIN results are unchanged. Go result records retain the base record key even for one-to-many matches, so repeated keys are legal; row/recordset readers MUST preserve every result row. Logical order is relation construction, WHERE, GROUP/aggregates, HAVING, ORDER BY, OFFSET/LIMIT, then final projection. `COUNT(*)` counts joined combinations; `COUNT(field)` ignores null/absent values; DISTINCT uses selected joined values. Ordering is stable for ties.

### Execution and adapters

#### REQ: generic-execution

DALgo MUST execute supported INNER/LEFT trees when a provider lacks native JOINs but can provide bounded relation scans, with reads per relation rather than per left row. Equality matching SHOULD use a hash index over the portable key domain. The plan MUST preserve multiplicity, LEFT null extension, filtering, pagination, and aggregation. Go and TypeScript generic execution use the same default caps: 10,000 total fetched rows, 16 MiB retained data, 10,000 result rows, and 100,000 candidate ON evaluations; a native adapter may have its own provider limits. A provider that cannot scan a relation, cannot supply required schema for wildcard projection, or exceeds declared resource bounds may reject before output with `join_plan`. Capability checks are explicit; unsupported mixed plans fail before partial output. An adapter MUST NOT silently ignore joins.

#### REQ: algorithm-hints

Each JOIN MAY contain `hints: {algorithms: [...]}`. `algorithms` is a nonempty ordered sequence of distinct, case-sensitive identifiers from exactly `hash`, `merge`, `lookup`, `batchedLookup`, and `nestedLoop`. Go DTQL and DALgo-JS MUST preserve its order and each JOIN's independent value through parse/serialize/parse; omission remains omission and does not change existing unhinted serialization. Empty lists, duplicates, unknown identifiers, and malformed hint shapes fail before execution with `join_algorithm` at the offending `from.joins[i].hints.algorithms` path (including `[i]` for an invalid entry). The same validation applies to direct in-memory model queries before provider reads. A defensive copy of the list MUST be exposed to callers. The exported Go model addition is justified by the brief's requirement to retain structured per-edge preferences for future policy while keeping the existing builders valid.

The identifiers describe physical strategies: `hash` indexes one input's equality keys in memory; `merge` walks both inputs ordered by the key and correctly handles duplicate key groups; `lookup` asks the joined source for each distinct key where efficient; `batchedLookup` asks it for bounded batches and respects source request limits; `nestedLoop` evaluates each candidate pair. A naive `nestedLoop` costs approximately O(N × M) predicate evaluations (100,000 by 100,000 can require billions), so it SHOULD only be selected deliberately for small relations, nonindexable predicates, testing, or debugging. Existing row, memory, result, and candidate bounds remain mandatory for all strategies.

An executor MUST validate the vocabulary, then consider the ordered list for each JOIN independently. It MAY honor, ignore, warn about, or reject a recognized preference according to its documented capabilities; a policy rejection of a valid preference is `join_plan` at that JOIN path. It MUST NOT claim to have honored an algorithm it did not run. The initial Go and JS generic executors honor `hash` when a direct cross-side equality key admits their existing index and honor bounded `nestedLoop` when requested. They skip currently unavailable `merge`, `lookup`, and `batchedLookup`; if no listed algorithm is applicable, they use their ordinary unhinted strategy (`hash` where applicable, bounded candidate evaluation otherwise). A preferred `nestedLoop` deliberately bypasses an otherwise applicable hash index; its candidate bound may reject an expensive query before output, even when the same unhinted query succeeds under `hash`. Resource limits therefore can change success versus rejection, but every successful strategy MUST return equivalent logical rows in the same order. The executor MUST NOT silently retry another algorithm after a selected strategy exhausts a bound. SQL/SQLite native execution may ignore physical hints while preserving logical semantics. No hint may change matching, multiplicity, LEFT extension, output order, projections, diagnostics for invalid keys, or query clause stages. Future source- and environment-specific honor/ignore/warn/reject policies are outside this Feature; retain the structured hints for them.

#### REQ: native-and-mixed-plans

An adapter MAY advertise complete-subtree native execution only when translation passes the same semantic tests. SQLite/SQL MUST translate ordinary uncorrelated same-database subtrees, such as `A LEFT (B INNER C ON b.cId=c.id) ON a.id=b.aId`, with parentheses preserving the right subtree. Native ON translation MUST enforce typed equality (numeric 1 equals 1.0; number 1 differs from text '1' and boolean true), null rules, and invalid-key rejection, or decline native execution in favor of generic execution. Shared fixtures cover those boundaries. A correlated nested ON such as `A LEFT (B INNER C ON c.aId=a.id) ON a.id=b.aId` is outside ordinary parenthesized SQL JOIN translation; it MUST use generic execution or fail with `join_plan`. The planner MUST leave a seam for mixed native plus DALgo composition; mixed execution may be deferred with an explicit diagnostic.

#### REQ: diagnostics-and-compatibility

Validation MUST name the JOIN path/index, alias, field, type, or operator involved. It rejects missing JOIN `from`, empty ON, malformed predicate, unsupported type/operator, unknown/duplicate aliases, forward references, ambiguous fields, cycles, unsupported plans, and detectably incompatible key types. Existing single-source YAML, builders, and adapter behavior stay valid. Go exported API changes MUST be additive. The human justification is the brief's statement: “This recursive shape is intentional. It enables nested joins, multiple joins at every node, reusable relation trees, future FROM references, future cross-source joins, and clean DALGO execution planning.”

The error contract is category plus path, with wording free to differ by language:

| Case | Category | Path example |
|---|---|---|
| missing `from`/ON, malformed predicate | `join_shape` | `from.joins[0].on` |
| unsupported type/operator | `join_type` / `join_operator` | `from.joins[0].type` / `from.joins[0].on[0].op` |
| duplicate, unknown, or forward alias | `join_scope` | `from.joins[1].on[0].right.source` |
| missing or ambiguous field | `join_field` | `columns[1].field` |
| recursive model/YAML reference | `join_cycle` | `from.joins[0].from` |
| invalid scanned key value | `join_key_type` | `from.joins[0].on[0].left` |
| incapable adapter or resource bound | `join_plan` | `from.joins[0]` |
| invalid algorithm list or identifier | `join_algorithm` | `from.joins[0].hints.algorithms[1]` |

## Dependencies

- query-joins
- dtql

## Acceptance Criteria

### AC: recursive-round-trip (verifies REQ:recursive-from, REQ:join-shape, REQ:round-trip-and-schema)

**Given** a three-level Invoice/Customer/Employee tree and a two-sibling tree in DTQL
**When** Go and TypeScript parse, validate, serialize, and parse each document
**Then** both preserve join order, aliases, ON keys, omitted-inner defaults, and structure, and the generated schema accepts them.

### AC: scopes (verifies REQ:ordered-scope, REQ:field-resolution)

**Given** sibling B then C where C.ON uses B, and nested child ON uses an ancestor
**When** each query is validated
**Then** both are accepted, while forward, unknown, duplicate, ambiguous, and unavailable references fail with JOIN paths.

### AC: inner-left-recursion (verifies REQ:nested-semantics, REQ:generic-execution)

**Given** duplicate matches, null keys, unmatched rows, and `A LEFT (B INNER C)` plus `A LEFT (B LEFT C)` trees
**When** Go and TypeScript execute through non-native sources
**Then** equivalent combinations and null extensions result without per-left-row provider reads.

### AC: composition (verifies REQ:results-and-pipeline)

**Given** joined one-to-many data and unmatched LEFT rows
**When** WHERE, explicit/excluded columns, ORDER BY, OFFSET/LIMIT, GROUP BY, HAVING, COUNT, COUNT DISTINCT, SUM, AVG, MIN, MAX, FIRST, and LAST are applied where supported
**Then** each clause uses the joined relation at the documented stage and single-source tests remain green.

### AC: native-and-unsupported (verifies REQ:native-and-mixed-plans, REQ:diagnostics-and-compatibility)

**Given** an ordinary SQLite tree and an adapter with no native JOIN capability
**When** each executes the same DTQL query
**Then** SQLite returns joined rows and the other executes generically when it can scan the relations; an unscannable source or exceeded bound returns `join_plan` before rows, and neither silently returns unjoined rows.

### AC: malformed-input (verifies REQ:round-trip-and-schema, REQ:diagnostics-and-compatibility)

**Given** missing `from`/ON, unsupported type/operator, cyclic aliases/objects, and a forward alias
**When** parsed or executed
**Then** both implementations reject before output with useful path-specific diagnostics.

### AC: chinook-journey (verifies REQ:recursive-from, REQ:results-and-pipeline, REQ:native-and-mixed-plans)

**Given** Chinook-style Invoice, Customer, and Employee data
**When** the documented nested query filters and selects joined fields, then orders and limits invoices
**Then** Go SQLite and TypeScript generic execution return equivalent rows, including invoices with an absent LEFT Employee.

### AC: algorithm-hints (verifies REQ:algorithm-hints, REQ:generic-execution, REQ:nested-semantics)

**Given** ordered hints on the outer, nested, and sibling JOINs, including an unavailable preferred algorithm followed by `hash` and an explicitly preferred `nestedLoop`
**When** Go and DALgo-JS parse, serialize, validate, and run equivalent data, including duplicate keys and LEFT null extension
**Then** each list retains its order and isolation; a directly tested per-JOIN selector proves the chosen strategy and fallback; successful hinted and unhinted results are identical; unknown or malformed values fail at a JOIN-specific `join_algorithm` path; and bounded nested-loop work fails with `join_plan` before rows without silently retrying `hash`.

## Architecture & Components

- `dal`: additive recursive model, validation, capabilities, and generic execution seam.
- `dtql`: recursive shape, serialization, schema and examples; backend independent.
- `dalgo2memory`: generic reference executor with per-relation reads and equality indexing.
- `dalgo2sql`/`dalgo2sqlite`: native translation for representable same-database trees.
- `dalgo-js`: matching model/parser, validation, generic executor, and explicit adapter behavior.

## Not Doing / Out of Scope

RIGHT, FULL, CROSS, non-equality ON, cost-based optimization, reusable `from.ref`, distributed source routing, or silent fallback after partial provider output.

## Open Questions

- Whether the unlanded DALgo-JS browser parser commit lands first or is incorporated here; its owner confirmed separate worktrees and landing coordination.
- Whether generic JOIN execution can reuse `dal.QueryExecutor` without extending exported interfaces; implementation must choose an additive route.

---
*This document follows the https://specscore.md/feature-specification*
