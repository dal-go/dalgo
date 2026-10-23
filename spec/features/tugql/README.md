---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: TugQL — YAML serialization of dal.StructuredQuery

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/tugql?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/tugql?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/tugql?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/tugql?op=request-change) |
**Status:** Approved
**Date:** 2026-06-05
**Owner:** alex
**Source Ideas:** —
**Supersedes:** —
**Grade:** A

## Summary

TugQL is a 1:1, lossless, human-readable **YAML serialization of dalgo's `dal.StructuredQuery`**. This Feature adds a top-level `tugql` package that (de)serializes between `dal.StructuredQuery` and a TugQL YAML document for the core relational read-only subset, documents the YAML shape, and proves a lossless round-trip in both directions. It also **publishes TugQL** as a public artifact: a JSON Schema (generated from the Go types) and a set of example documents, served as a documented site at **https://dal-go.github.io/tugql/** so TugQL files can be validated by any tool and read by humans. It serves any dalgo consumer that needs to save, hand-edit, diff, and reload a structured query as text — first among them DataTug's serve-brokered query builder. Realizes the cross-repo Idea `specscore:idea/tugql-datatug-query-language@github.com/datatug/datatug`.

## Problem

dalgo models structured queries (`dal.StructuredQuery` — `From`/`Where`/`OrderBy`/`Columns`/`Limit`/`Offset` over `Expression`, `Condition`, `Column`, `FromSource` nodes) but has **no textual, human-readable, round-trippable form** for them. A consumer that wants to persist a query, edit it by hand, diff it in version control, or move it across a wire must reach into Go structs or invent an ad-hoc encoding. DataTug's serve-brokered query builder already assumes such a form ("TugQL YAML") exists and is lossless (`REQ:tugql-yaml-form`), but nothing defines it. This Feature defines TugQL for the core relational subset so the serialization is one canonical thing in dalgo rather than per-consumer reinvention.

## Behavior

### Representation

TugQL is plain YAML over the existing `dal` query model — no bespoke grammar, no new AST.

#### REQ: serialize-structuredquery

The `tugql` package MUST provide serialization from an in-scope `dal.StructuredQuery` to a TugQL YAML document, and deserialization from a TugQL YAML document back to a `dal.StructuredQuery`. TugQL YAML MUST be plain YAML produced and consumed by a standard YAML library — no bespoke grammar or hand-written parser.

#### REQ: covered-subset

TugQL MUST represent the core relational read-only subset of `dal.StructuredQuery`: a single `From` whose `Joins()` is empty, over a **root** `dal.CollectionRef` base (a flat named recordset, `Parent() == nil`); the selected `Columns`; the `Where` `Condition` (both `Comparison` nodes and And/Or `GroupCondition` trees); `OrderBy` expressions; and `Limit`/`Offset`. The expression nodes in scope are field references (`FieldRef`), constants (`Constant`), and constant arrays (`Array`, for `In` membership). The comparison operators in scope are `==`, `In`, `>`, `>=`, `<`, `<=`; the group operators are `And`/`Or`. Literal values are carried **inline** as `Constant`/`Array` expressions — `dal.StructuredQuery` has no separate parameter list (`QueryArg` belongs to `dal.TextQuery`, which is out of scope). Each in-scope `dal` node MUST have a defined YAML representation.

#### REQ: null-tests

TugQL MUST represent `dal.IsNullCondition` as a condition form of its own: `isNull: <expression>` for `x IS NULL` and `isNotNull: <expression>` for `x IS NOT NULL`, each holding exactly one expression. A condition mapping contains exactly one of comparison (`op`/`left`/`right`), `and`, `or`, `exists`, `notExists`, `isNull` or `isNotNull`; a document that sets two of them MUST be rejected with an error that names the forms. A null test is valid wherever a condition is (`where`, `having`, inside `and`/`or`, inside a nested query) and MUST be rejected in a join's `on` list, which stays equality-only. Its operand is a field, a literal, arithmetic over those, or a scalar subquery; an aggregate is valid only in `having`, and a `values` list, `star` and `param` MUST be rejected at parse, with the path of the null test in the error, in both the Go and the TypeScript engine. Serialization and deserialization of a null test MUST round-trip structurally and canonically, and the generated schema MUST accept it.

#### REQ: reject-out-of-scope

Serialization MUST reject (with a clear, descriptive error) any `dal.StructuredQuery` outside the covered subset — a `From` with a non-empty `Joins()` (joins), a base that is a `CollectionGroupRef` or a parented `CollectionRef` (`Parent() != nil`), a `GroupBy`, an aggregate or scalar function, a cursor/`StartFrom`, or a comparison/group operator outside the in-scope set — rather than silently dropping it. TugQL MUST NOT emit a document that loses query semantics.

### Round-trip fidelity

A TugQL document and the `StructuredQuery` it encodes must be interconvertible without loss, and serialization must be stable enough to diff.

#### REQ: round-trip-structural

For any in-scope `dal.StructuredQuery` `q`, `deserialize(serialize(q))` MUST reconstruct a `StructuredQuery` that is structurally equal to `q` — identical `From`, `Columns`, `Where` condition tree (including inline `Constant`/`Array` values), `OrderBy`, `Limit`, and `Offset`.

#### REQ: round-trip-canonical

Serialization MUST be canonical: for any valid in-scope TugQL YAML document `d`, `serialize(deserialize(d))` MUST produce byte-identical YAML (stable key ordering and formatting), so saved queries diff cleanly across edits.

### Errors and validation

#### REQ: invalid-input-errors

Deserializing malformed or schema-invalid TugQL YAML — unknown keys, wrong value types, missing required fields, or an unknown operator — MUST return a descriptive error identifying the problem and MUST NOT return a partially-populated `StructuredQuery`.

### Package and documentation

#### REQ: package-location

TugQL MUST live in a new top-level package `github.com/dal-go/dalgo/tugql` that imports `dal`. It MUST NOT add a YAML dependency to the `dal` package.

#### REQ: documented-shape

The package MUST document the TugQL YAML shape — the mapping from each in-scope `dal` node (`From` / root `CollectionRef`, `Column`, `Comparison`, `GroupCondition`, `IsNullCondition`, `OrderExpression`, `FieldRef`, `Constant`, `Array`, `Operator`) to its YAML representation — versioned in-repo alongside the code.

### Published schema and examples

TugQL is published as a public, tool-validatable artifact so consumers in any language can validate a `.tugql.yaml` and humans can read the shape.

#### REQ: machine-checkable-schema

The package MUST provide a **JSON Schema (draft 2020-12)** describing the in-scope TugQL YAML, **generated from the `tugql` Go types** (the types are the single source of truth). It MUST be emitted in both serializations — `schema.json` (the canonical `$id`, `https://dal-go.github.io/tugql/schema.json`) and `schema.yaml` (identical content) — and CI MUST fail if the committed schema is stale relative to the Go types (regenerate-and-diff).

#### REQ: schema-accepts-serializer-output

Every TugQL document produced by `serialize` for an in-scope `dal.StructuredQuery` MUST validate against the generated schema. A test MUST assert this over a representative set of in-scope queries, so the schema (generated from the types) and the serializer's actual output cannot drift.

#### REQ: example-documents

The package MUST provide a set of example TugQL documents that together exercise the in-scope subset (source, columns, comparison + And/Or group filters, ordering, limit/offset). Each example MUST be a valid TugQL document (it deserializes to a `dal.StructuredQuery`) **and** MUST validate against the schema; CI MUST enforce both.

#### REQ: published-site

The schema (both serializations), the example documents, and a human-readable, styled index page documenting the node→YAML mapping MUST be published at **https://dal-go.github.io/tugql/** (served from the `dal-go.github.io` repository) and MUST be kept in sync with the `tugql` package — the publish MUST be driven from the generated artifacts, not hand-maintained separately.

### Additive TugQL authoring adapter

TugQL has two forms of one language: the structured **TugQL YAML** form, whose `Serialize` and `Deserialize` APIs stay plain YAML over `dal.StructuredQuery`, and the **TugQL text** form, an additive authoring adapter that parses into a versioned TugQTree and resolves supported authoring constructs into that same existing query model. A query saved by a tool has one type, `TugQL`; the text body is `<id>.query.tugql` and the structured YAML body is `<id>.query.tugql.yaml`.

#### REQ:tugql-authoring-adapter

The `tugql` package MUST expose additive `ParseTugQL`, `ResolveTugQL`, and `FormatTugQL` APIs with supporting document, tree, context, option, result, and diagnostic types. Every executable `TugQLDocument` MUST carry `sourceMetadata: {format: "tugql", version: 1}`; parsing source happens before missing-tree checks so malformed source diagnostics are not masked. Human approval was recorded verbatim: **Alex (2026-10-10): “Approve these additive APIs for TugQL parsing, resolution, and formatting”.** The text profile MUST preserve parameters, ordered `WITH` CTE/import definitions, select-list blocks, scalar subqueries, joins, and source-positioned diagnostics in a versioned TugQTree document; syntax parsing MUST remain independent of schema authorization. Resolution MUST bind declared parameters, require caller-authorized schemas and revision-pinned imports, validate exact relationship metadata before expanding omitted `ON`, and lower supported constructs through the existing TugQL deserializer into `dal.StructuredQuery`. Import `USING` maps support typed scalar literals and parameters declared by the importing query; row-field or arbitrary expression mappings MUST fail closed until lateral/evaluated import semantics are specified. An omitted projection MUST merge joined key fields only for complete authorized exact-typed equality relationships on INNER joins; LEFT joins preserve distinct keys because null extension can distinguish them. Scalar subqueries MUST retain existing engine cardinality behavior. It MUST NOT add a second executor, perform ambient filesystem or network reads, return an executable partial query, or change existing TugQL YAML behavior. Unsupported execution semantics MUST fail closed with a diagnostic.

The version 1 parameter catalog is exactly `integer`, `decimal`, `string`, `boolean`, `date`, `datetime`, and `timestamp` (case-insensitive type spelling, with no abbreviated aliases). Decimal defaults and bindings use finite exact decimal text; date literals use real `YYYY-MM-DD` dates; datetime and timestamp literals use RFC3339 text. Resolution MUST fail closed for decimal and datetime/timestamp execution until the existing engine can prove numerically exact decimal and instant-aware timestamp comparison semantics.

For output type inference, the version 1 adapter accepts the existing schema type spellings `integer`, `int`, `int32`, `int64`, `decimal`, `numeric`, `float`, `float32`, `float64`, and `number`; `real` and unknown types fail closed for computed numeric outputs. Supported binary arithmetic and `SUM`/`AVG` report the adapter's generic `number` metadata type, matching float64-based evaluation on the generic non-money executor path. TugQL does not enforce a non-money execution mode; this metadata is not a width-preserving or exact-arithmetic promise, and money-configured behavior remains outside this authoring profile. `SUM`/`AVG` require numeric operands. Exact `decimal`/`numeric` arithmetic, comparison, ordering, grouping, distinct, and `SUM`/`AVG`/`MIN`/`MAX` operations fail closed until exact executor semantics are available, though raw projection and `COUNT` remain supported. `date` values use canonical ISO dates and may be compared/ordered; `datetime` and `timestamp` operations requiring comparison or ordering fail closed. Ordered comparisons require compatible operand types; cross-family string/numeric ordering is rejected. Computed `ORDER BY` expressions fail closed until supported by the existing query model.

Formatting MUST preserve comments and normalize reserved keyword casing and structural indentation according to the configured precedence. The shared versioned corpus MUST pin equivalent TugQTree semantic objects, diagnostic codes/spans, and execution outcomes across Go and TypeScript; YAML emitters MUST preserve equivalent YAML semantics, while ordering and quoting may differ by language. Shared fixture files themselves MUST be byte-identical and protected by `manifest.sha256`. Resolution MUST enforce a 128-level structural-depth limit and a 5,000-node semantic budget before recursive validation or lowering. The aggregate semantic-node charge is the authored root document plus each parsed pinned import occurrence once (repeated imports count once per occurrence), plus any positive difference between the final expanded root query and the authored root query. Independently, every expanded CTE, import, and scalar-query body MUST remain within 5,000 canonical semantic nodes and 128 levels before another recursive walk or copy; this also applies to unused definitions because version 1 eagerly validates and expands every declared definition. Import expansion is capped at 128 documents. These per-body checks do not replace the aggregate budget.

The version 1 text grammar does not include multiline `EXISTS` condition blocks. Such source is rejected as invalid condition syntax; exact diagnostic parity for this unsupported form is not part of the shared-language contract. A caller-created TugQTree may use the supported canonical EXISTS condition shape, which is validated and resolved through the ordinary authorization and resource-limit paths.

### Comments

#### REQ:tugql-comments

The TugQL text form MUST support two end-of-line comment markers, `--` and `//`. Each starts a comment that runs to the end of the line, and each is recognised only outside string literals (`'…'`) and quoted identifiers (`"…"`, `` `…` ``), so `'https://example.com/x'` and `"a//b"` contain no comment. `#` is NOT a comment in the text form. The formatter MUST keep each comment and the author's marker (`//` stays `//`, `--` stays `--`). Comments live on the parsed TugQTree (`TugQLTree.Comments`), never on `dal.StructuredQuery`, and MUST NOT influence resolution, authorization or generated SQL: no `//` (or any comment text) may reach emitted SQL outside a literal or quoted identifier, and an emitter that carries a comment writes it as `--`.

The structured TugQL YAML form carries comments as YAML `#` comments. Text to YAML turns both markers into `#`: comment lines above a clause become the head comment above that clause key, and a comment after code on a clause's line becomes its same-line comment. YAML to text turns `#` into `--`. A text, YAML, text round trip therefore keeps every comment and its placement, with one documented and accepted loss: a `//` comment returns as `--`. A comment with no clean anchor (inside a multi-line `PARAMETERS`, `WITH` or `SELECT` block, or on a YAML node that is not a clause) becomes a leading comment of the nearest following clause; a comment after the last clause becomes a document-end comment. A comment is never dropped. The exported conversion surface is `TugQLToYAML`, `YAMLToTugQL` and `PrintTugQL` plus the `TugQLComments`, `TugQLCommentGroup` and `TugQLComment` types and the `TugQLTree.Comments` field. Constructs that have no text spelling fail `YAMLToTugQL`/`PrintTugQL` closed with a diagnostic.

Human approval was recorded verbatim: **Alexander Trakhimenok (2026-10-23): “let's support -- in text form and translate this comments to # in yaml form”** and **“I think we should support // form as well - this is what a lot of developers know and use instinctevely. I always missed // form in SQL”**. The shared Go/TypeScript conformance vectors are `tugql/testdata/tugql/v1/comments.json`, listed in `manifest.sha256`; for those vectors both implementations MUST emit byte-identical YAML.

## Acceptance Criteria

### AC: serialize-and-back (verifies REQ:serialize-structuredquery)

**Given** an in-scope `dal.StructuredQuery` built in Go
**When** it is serialized with the `tugql` package and the resulting document is deserialized
**Then** a `dal.StructuredQuery` is returned and the document is plain YAML parseable by a standard YAML library.

### AC: subset-nodes-represented (verifies REQ:covered-subset)

**Given** a `StructuredQuery` with a single `From` (no joins) over a root `CollectionRef`, selected `Columns`, a `Where` combining `Comparison` (with inline `Constant`/`Array` values) and And/Or `GroupCondition`, `OrderBy`, `Limit`, and `Offset`
**When** it is serialized to TugQL YAML
**Then** the YAML contains a defined representation for each of those nodes (source, columns, where tree with inline constants, order, limit, offset) with no node omitted.

### AC: null-tests-round-trip (verifies REQ:null-tests, REQ:round-trip-canonical, REQ:schema-accepts-serializer-output)

**Given** TugQL documents that use `isNull` and `isNotNull` in `where`, in `having`, inside `and`/`or` groups, over a field, an arithmetic expression and (in `having`) an aggregate
**When** each is deserialized, serialized, and validated against the generated schema
**Then** the output is byte-identical to the input, the schema accepts it, and the same document read as JSON gives a structurally equal query.

### AC: null-test-operands-and-forms-rejected (verifies REQ:null-tests, REQ:invalid-input-errors)

**Given** a null test whose operand is a `values` list, `star`, a `param`, or an aggregate in `where`, a null test inside a join's `on`, and a condition that sets `isNull` together with `isNotNull`, `op`, `and`, `or`, `exists` or `notExists`
**When** each document is deserialized
**Then** a descriptive error is returned (naming the null test's path, or the forms that were mixed) and no `StructuredQuery` is produced.

### AC: out-of-scope-rejected (verifies REQ:reject-out-of-scope)

**Given** a `StructuredQuery` whose `From` has a non-empty `Joins()` (or a `CollectionGroupRef` / parented `CollectionRef` base, a `GroupBy`, a function, or a cursor)
**When** it is passed to serialization
**Then** serialization returns a descriptive error naming the unsupported construct and produces no TugQL document.

### AC: structural-round-trip (verifies REQ:round-trip-structural)

**Given** an in-scope `StructuredQuery` `q`
**When** `deserialize(serialize(q))` is computed
**Then** the resulting query is structurally equal to `q` across `From`, `Columns`, `Where` (including inline `Constant`/`Array` values), `OrderBy`, `Limit`, and `Offset`.

### AC: canonical-round-trip (verifies REQ:round-trip-canonical)

**Given** a valid in-scope TugQL YAML document `d`
**When** `serialize(deserialize(d))` is computed
**Then** the output is byte-identical to `d` (stable key ordering and formatting).

### AC: invalid-yaml-rejected (verifies REQ:invalid-input-errors)

**Given** a TugQL YAML document with an unknown key, a wrong value type, or an unknown operator
**When** it is deserialized
**Then** a descriptive error is returned and no partially-populated `StructuredQuery` is produced.

### AC: lives-in-tugql-package (verifies REQ:package-location)

**Given** the dalgo module
**When** the TugQL code is located
**Then** it resides in package `github.com/dal-go/dalgo/tugql`, imports `dal`, and the `dal` package has gained no YAML dependency.

### AC: shape-documented (verifies REQ:documented-shape)

**Given** the `tugql` package
**When** a reader looks for the TugQL YAML shape
**Then** in-repo documentation maps each in-scope `dal` node to its YAML representation.

### AC: schema-generated-and-fresh (verifies REQ:machine-checkable-schema)

**Given** the `tugql` Go types and the committed `schema.json` / `schema.yaml`
**When** the schema is regenerated from the types in CI
**Then** a JSON Schema (draft 2020-12) is produced in both serializations with a canonical `$id` of `https://dal-go.github.io/tugql/schema.json`, and CI fails if the regenerated schema differs from the committed one.

### AC: serialized-validates-against-schema (verifies REQ:schema-accepts-serializer-output)

**Given** a representative set of in-scope `dal.StructuredQuery` values
**When** each is serialized to TugQL YAML and the output is validated against the generated schema
**Then** every serialized document passes schema validation (the serializer and schema agree).

### AC: examples-valid (verifies REQ:example-documents)

**Given** the set of example TugQL documents
**When** CI runs
**Then** each example deserializes to a `dal.StructuredQuery` and validates against the schema, and together they exercise source, columns, comparison + And/Or group filters, ordering, and limit/offset.

### AC: site-published (verifies REQ:published-site)

**Given** the generated schema, examples, and index page
**When** the publish runs
**Then** `https://dal-go.github.io/tugql/` serves `schema.json`, `schema.yaml`, the example documents, and a styled index page documenting the node→YAML mapping, sourced from the generated artifacts (not hand-maintained).

### AC: tugql-authoring-adapter (verifies REQ:tugql-authoring-adapter)

**Given** a valid TugQL document using parameters, ordered CTE/import definitions, select-list blocks, scalar subqueries, or joins, plus an explicitly supplied authorized schema and pinned import contents where resolution needs them
**When** it is parsed, formatted, and resolved
**Then** parsing returns one complete versioned TugQTree whose semantic JSON model matches the shared Go/TypeScript corpus. The shared corpus files and expected source/YAML fixtures are byte-identical; language YAML emitters may differ in key order and quoting while decoding to the same TugQTree semantics. Resolution returns a `dal.StructuredQuery` with ordered typed output lineage and dependency/relationship receipts or a positioned diagnostic with no query. For scalar outputs, Go's legacy query representation places the alias on the containing column (`as` beside `query`), while TypeScript's recursive query model places it inside the nested query (`query.as`); both retain the same scalar result name and cardinality. Formatting preserves comments and applies effective style. The legacy TugQL YAML API and format remain unchanged.

### AC: tugql-comments (verifies REQ:tugql-comments)

**Given** TugQL text containing `--` and `//` comments (leading, trailing, blank-line separated, consecutive, mixed markers, with no clean anchor, and document-trailing) and the same markers inside string literals and quoted identifiers
**When** it is lexed, formatted, converted to TugQL YAML, converted back to text, and resolved
**Then** only the real comments are recognised, the formatter keeps each marker, the YAML carries them as `#` head and same-line comments, the round-trip text carries them as `--`, and the resolved query is identical to the one resolved without comments, with no `//` outside literals.

## Rehearse Integration

Most ACs have a concrete, pure-function Go surface (`serialize`, `deserialize`, round-trip composition, error returns, package import graph) and are directly unit-testable with table tests. The schema, examples, and publish ACs (`schema-generated-and-fresh`, `examples-valid`, `site-published`) are CI-enforced checks (regenerate-and-diff, schema validation, publish output). Stub scaffolding under `_tests/` is deferred to the Plan phase so the stub set tracks the final task/test breakdown rather than being authored twice — consistent with the approach used by the serve-brokered query-builder Features that consume TugQL.

## Architecture and Components

- **`tugql` package (new, module root).** Depends on `dal`. Exposes (de)serialization entry points between `dal.StructuredQuery` and TugQL YAML, plus the canonicalization used by `round-trip-canonical`.
- **YAML encoding.** Uses a standard Go YAML library, isolated to the `tugql` package so `dal` stays dependency-free.
- **Subset gate.** A single place that classifies a `StructuredQuery` as in-scope or rejects it (`reject-out-of-scope`), so the lossless guarantee is enforced in one spot.
- **Shape doc.** An in-repo document (e.g. `tugql/README.md` or a doc comment) describing the node→YAML mapping (`documented-shape`).
- **Schema generator.** A tool that emits the JSON Schema (`schema.json` + `schema.yaml`) from the `tugql` Go types; run in CI with a regenerate-and-diff staleness check (`machine-checkable-schema`).
- **Examples.** A directory of example `.tugql.yaml` documents, validated in CI against the schema and against the deserializer (`example-documents`).
- **Publish step.** CI that copies the generated schema, examples, and a styled `index.html` into the `dal-go.github.io` repo under `/tugql/` (`published-site`). Cross-repo: `dalgo` is the source of truth; `dal-go.github.io` is the serving surface.

## Data Flow

`dal.StructuredQuery` → subset gate → TugQL YAML (serialize). TugQL YAML → validate → `dal.StructuredQuery` (deserialize). Round-trip tests compose the two in both directions. Separately, the `tugql` Go types → schema generator → `schema.json` / `schema.yaml`; the schema + example documents + a styled `index.html` are published by CI to `dal-go.github.io/tugql/`, and examples are validated against the schema before publish.

## Not Doing / Out of Scope

- Joins (a non-empty `From.Joins()`), `GroupBy`/aggregates, scalar functions, cursor/`StartFrom`, and `CollectionGroupRef` / parented `CollectionRef` sources — deferred to follow-ons; serialization rejects them (`REQ:reject-out-of-scope`).
- The native-text form (`dal.TextQuery`) and its `QueryArg` parameters — TugQL serializes the *structured* side only; `dal.StructuredQuery` carries literal values inline as `Constant`/`Array` and has no parameter list.
- A bespoke grammar for TugQL YAML, and adopting an existing text language (PRQL/Malloy) — TugQL YAML remains plain YAML over the existing AST. The separately approved TugQL text adapter is additive and does not alter the `Serialize`/`Deserialize` API or the YAML document format; the only YAML-side addition is comment conversion (`REQ:tugql-comments`), which leaves comment-free documents byte-identical.
- Per-driver rendering of the AST to a native SQL dialect or document query — owned by dalgo drivers and the serve-brokered daemon, not this package.
- The saved-`.tugql.yaml` project-file UX — a DataTug consumer concern, not dalgo's.
- A general docs/site framework for the `dal-go.github.io` site — TugQL publishes a `/tugql/` subtree into the existing static site; restyling or re-platforming that site is out of scope.

## Assumption Carryover

From the cross-repo Idea `tugql-datatug-query-language`:

- **dalgo owns a query AST for TugQL to serialize (Must-be-true, confirmed)** — satisfied by the existing `dal.StructuredQuery`; encoded as `REQ:serialize-structuredquery` + `REQ:covered-subset`.
- **A 1:1 lossless AST↔YAML round-trip is achievable with off-the-shelf YAML (Must-be-true)** — encoded as `REQ:round-trip-structural` + `REQ:round-trip-canonical`, validated by their ACs.
- **TugQL YAML is human-readable / hand-editable (Should-be-true)** — supported by `REQ:documented-shape` and canonical formatting; full confirmation needs dogfooding, deferred.
- **One AST renders to multiple native dialects (Should-be-true)** — out of scope here (rendering is not TugQL's job); carried by the dalgo drivers / serve-brokered daemon.

## Open Questions

- The acronym TugQL expands to "DataTug Query Language", but the format now lives in the general-purpose dalgo library. Keep the established TugQL name (used across the serve-brokered specs) or adopt a neutral gloss in dalgo? Kept as-is for cross-repo consistency this cycle.
- The publish target is the `dal-go.github.io` repo (cross-repo). Does the `/tugql/` site warrant its own small Feature in `dal-go.github.io`, or is owning the publish from `dalgo` CI sufficient? Treated as a `dalgo`-owned publish this cycle.
- Mechanism for the cross-repo publish (CI push with a deploy token vs. a `dal-go.github.io` workflow that pulls generated artifacts from a `dalgo` release) — a Plan/CI detail.
- Should the cross-repo source link to the datatug Idea be formalized in `**Source Ideas:**` once tooling resolves cross-repo idea references, instead of being stated in prose?
- Exact structural-equality mechanism for `round-trip-structural` (a `dal`-provided equality helper vs. a `tugql`-local comparator) — a Plan/implementation detail.
- How far the relational subset later extends (`GroupBy`, joins, functions) before nested/document shapes — tracked for the next TugQL cycle.

## Sidekick Seeds Generated

- [dalgo-needs-first-class-join-support-in-its-query-model](../../ideas/seeds/dalgo-needs-first-class-join-support-in-its-query-model.md) — captured 2026-06-05 by user

---
*This document follows the https://specscore.md/feature-specification*
