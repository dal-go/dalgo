# Feature: DTQL — YAML serialization of dal.StructuredQuery

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dtql?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dtql?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dtql?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/dtql?op=request-change) |
**Status:** Approved
**Date:** 2026-06-05
**Owner:** alex
**Source Ideas:** —
**Supersedes:** —
**Grade:** A

## Summary

DTQL is a 1:1, lossless, human-readable **YAML serialization of dalgo's `dal.StructuredQuery`**. This Feature adds a top-level `dtql` package that (de)serializes between `dal.StructuredQuery` and a DTQL-YAML document for the core relational read-only subset, documents the YAML shape, and proves a lossless round-trip in both directions. It serves any dalgo consumer that needs to save, hand-edit, diff, and reload a structured query as text — first among them DataTug's serve-brokered query builder. Realizes the cross-repo Idea `specscore:idea/dtql-datatug-query-language@github.com/datatug/datatug`.

## Problem

dalgo models structured queries (`dal.StructuredQuery` — `From`/`Where`/`OrderBy`/`Columns`/`Limit`/`Offset` over `Expression`, `Condition`, `Column`, `FromSource` nodes) but has **no textual, human-readable, round-trippable form** for them. A consumer that wants to persist a query, edit it by hand, diff it in version control, or move it across a wire must reach into Go structs or invent an ad-hoc encoding. DataTug's serve-brokered query builder already assumes such a form ("DTQL-YAML") exists and is lossless (`REQ:dtql-yaml-form`), but nothing defines it. This Feature defines DTQL for the core relational subset so the serialization is one canonical thing in dalgo rather than per-consumer reinvention.

## Behavior

### Representation

DTQL is plain YAML over the existing `dal` query model — no bespoke grammar, no new AST.

#### REQ: serialize-structuredquery

The `dtql` package MUST provide serialization from an in-scope `dal.StructuredQuery` to a DTQL-YAML document, and deserialization from a DTQL-YAML document back to a `dal.StructuredQuery`. DTQL-YAML MUST be plain YAML produced and consumed by a standard YAML library — no bespoke grammar or hand-written parser.

#### REQ: covered-subset

DTQL MUST represent the core relational read-only subset of `dal.StructuredQuery`: a single `From` whose `Joins()` is empty, over a **root** `dal.CollectionRef` base (a flat named recordset, `Parent() == nil`); the selected `Columns`; the `Where` `Condition` (both `Comparison` nodes and And/Or `GroupCondition` trees); `OrderBy` expressions; and `Limit`/`Offset`. The expression nodes in scope are field references (`FieldRef`), constants (`Constant`), and constant arrays (`Array`, for `In` membership). The comparison operators in scope are `==`, `In`, `>`, `>=`, `<`, `<=`; the group operators are `And`/`Or`. Literal values are carried **inline** as `Constant`/`Array` expressions — `dal.StructuredQuery` has no separate parameter list (`QueryArg` belongs to `dal.TextQuery`, which is out of scope). Each in-scope `dal` node MUST have a defined YAML representation.

#### REQ: reject-out-of-scope

Serialization MUST reject (with a clear, descriptive error) any `dal.StructuredQuery` outside the covered subset — a `From` with a non-empty `Joins()` (joins), a base that is a `CollectionGroupRef` or a parented `CollectionRef` (`Parent() != nil`), a `GroupBy`, an aggregate or scalar function, a cursor/`StartFrom`, or a comparison/group operator outside the in-scope set — rather than silently dropping it. DTQL MUST NOT emit a document that loses query semantics.

### Round-trip fidelity

A DTQL document and the `StructuredQuery` it encodes must be interconvertible without loss, and serialization must be stable enough to diff.

#### REQ: round-trip-structural

For any in-scope `dal.StructuredQuery` `q`, `deserialize(serialize(q))` MUST reconstruct a `StructuredQuery` that is structurally equal to `q` — identical `From`, `Columns`, `Where` condition tree (including inline `Constant`/`Array` values), `OrderBy`, `Limit`, and `Offset`.

#### REQ: round-trip-canonical

Serialization MUST be canonical: for any valid in-scope DTQL-YAML document `d`, `serialize(deserialize(d))` MUST produce byte-identical YAML (stable key ordering and formatting), so saved queries diff cleanly across edits.

### Errors and validation

#### REQ: invalid-input-errors

Deserializing malformed or schema-invalid DTQL-YAML — unknown keys, wrong value types, missing required fields, or an unknown operator — MUST return a descriptive error identifying the problem and MUST NOT return a partially-populated `StructuredQuery`.

### Package and documentation

#### REQ: package-location

DTQL MUST live in a new top-level package `github.com/dal-go/dalgo/dtql` that imports `dal`. It MUST NOT add a YAML dependency to the `dal` package.

#### REQ: documented-shape

The package MUST document the DTQL-YAML shape — the mapping from each in-scope `dal` node (`From` / root `CollectionRef`, `Column`, `Comparison`, `GroupCondition`, `OrderExpression`, `FieldRef`, `Constant`, `Array`, `Operator`) to its YAML representation — versioned in-repo alongside the code.

## Acceptance Criteria

### AC: serialize-and-back (verifies REQ:serialize-structuredquery)

**Given** an in-scope `dal.StructuredQuery` built in Go
**When** it is serialized with the `dtql` package and the resulting document is deserialized
**Then** a `dal.StructuredQuery` is returned and the document is plain YAML parseable by a standard YAML library.

### AC: subset-nodes-represented (verifies REQ:covered-subset)

**Given** a `StructuredQuery` with a single join-free `From` over a root `CollectionRef`, selected `Columns`, a `Where` combining `Comparison` (with inline `Constant`/`Array` values) and And/Or `GroupCondition`, `OrderBy`, `Limit`, and `Offset`
**When** it is serialized to DTQL-YAML
**Then** the YAML contains a defined representation for each of those nodes (source, columns, where tree with inline constants, order, limit, offset) with no node omitted.

### AC: out-of-scope-rejected (verifies REQ:reject-out-of-scope)

**Given** a `StructuredQuery` whose `From` has a non-empty `Joins()` (or a `CollectionGroupRef` / parented `CollectionRef` base, a `GroupBy`, a function, or a cursor)
**When** it is passed to serialization
**Then** serialization returns a descriptive error naming the unsupported construct and produces no DTQL document.

### AC: structural-round-trip (verifies REQ:round-trip-structural)

**Given** an in-scope `StructuredQuery` `q`
**When** `deserialize(serialize(q))` is computed
**Then** the resulting query is structurally equal to `q` across `From`, `Columns`, `Where` (including inline `Constant`/`Array` values), `OrderBy`, `Limit`, and `Offset`.

### AC: canonical-round-trip (verifies REQ:round-trip-canonical)

**Given** a valid in-scope DTQL-YAML document `d`
**When** `serialize(deserialize(d))` is computed
**Then** the output is byte-identical to `d` (stable key ordering and formatting).

### AC: invalid-yaml-rejected (verifies REQ:invalid-input-errors)

**Given** a DTQL-YAML document with an unknown key, a wrong value type, or an unknown operator
**When** it is deserialized
**Then** a descriptive error is returned and no partially-populated `StructuredQuery` is produced.

### AC: lives-in-dtql-package (verifies REQ:package-location)

**Given** the dalgo module
**When** the DTQL code is located
**Then** it resides in package `github.com/dal-go/dalgo/dtql`, imports `dal`, and the `dal` package has gained no YAML dependency.

### AC: shape-documented (verifies REQ:documented-shape)

**Given** the `dtql` package
**When** a reader looks for the DTQL-YAML shape
**Then** in-repo documentation maps each in-scope `dal` node to its YAML representation.

## Rehearse Integration

Every AC has a concrete, pure-function Go surface (`serialize`, `deserialize`, round-trip composition, error returns, package import graph) and is directly unit-testable with table tests. Stub scaffolding under `_tests/` is deferred to the Plan phase so the stub set tracks the final task/test breakdown rather than being authored twice — consistent with the approach used by the serve-brokered query-builder Features that consume DTQL.

## Architecture and Components

- **`dtql` package (new, module root).** Depends on `dal`. Exposes (de)serialization entry points between `dal.StructuredQuery` and DTQL-YAML, plus the canonicalization used by `round-trip-canonical`.
- **YAML encoding.** Uses a standard Go YAML library, isolated to the `dtql` package so `dal` stays dependency-free.
- **Subset gate.** A single place that classifies a `StructuredQuery` as in-scope or rejects it (`reject-out-of-scope`), so the lossless guarantee is enforced in one spot.
- **Shape doc.** An in-repo document (e.g. `dtql/README.md` or a doc comment) describing the node→YAML mapping (`documented-shape`).

## Data Flow

`dal.StructuredQuery` → subset gate → DTQL-YAML (serialize). DTQL-YAML → validate → `dal.StructuredQuery` (deserialize). Round-trip tests compose the two in both directions.

## Not Doing / Out of Scope

- Joins (a non-empty `From.Joins()`), `GroupBy`/aggregates, scalar functions, cursor/`StartFrom`, and `CollectionGroupRef` / parented `CollectionRef` sources — deferred to follow-ons; serialization rejects them (`REQ:reject-out-of-scope`).
- The native-text form (`dal.TextQuery`) and its `QueryArg` parameters — DTQL serializes the *structured* side only; `dal.StructuredQuery` carries literal values inline as `Constant`/`Array` and has no parameter list.
- A bespoke DTQL grammar/parser, and adopting an existing text language (PRQL/Malloy) — DTQL is YAML over the existing AST.
- Per-driver rendering of the AST to a native SQL dialect or document query — owned by dalgo drivers and the serve-brokered daemon, not this package.
- A separate machine-checkable schema artifact (e.g. JSON Schema) — this cycle ships the Go (de)serializer plus documented shape; the round-trip tests are the guarantee.
- The saved-`.dtql.yaml` project-file UX — a DataTug consumer concern, not dalgo's.

## Assumption Carryover

From the cross-repo Idea `dtql-datatug-query-language`:

- **dalgo owns a query AST for DTQL to serialize (Must-be-true, confirmed)** — satisfied by the existing `dal.StructuredQuery`; encoded as `REQ:serialize-structuredquery` + `REQ:covered-subset`.
- **A 1:1 lossless AST↔YAML round-trip is achievable with off-the-shelf YAML (Must-be-true)** — encoded as `REQ:round-trip-structural` + `REQ:round-trip-canonical`, validated by their ACs.
- **DTQL-YAML is human-readable / hand-editable (Should-be-true)** — supported by `REQ:documented-shape` and canonical formatting; full confirmation needs dogfooding, deferred.
- **One AST renders to multiple native dialects (Should-be-true)** — out of scope here (rendering is not DTQL's job); carried by the dalgo drivers / serve-brokered daemon.

## Open Questions

- The acronym DTQL expands to "DataTug Query Language", but the format now lives in the general-purpose dalgo library. Keep the established DTQL name (used across the serve-brokered specs) or adopt a neutral gloss in dalgo? Kept as-is for cross-repo consistency this cycle.
- Should the cross-repo source link to the datatug Idea be formalized in `**Source Ideas:**` once tooling resolves cross-repo idea references, instead of being stated in prose?
- Exact structural-equality mechanism for `round-trip-structural` (a `dal`-provided equality helper vs. a `dtql`-local comparator) — a Plan/implementation detail.
- How far the relational subset later extends (`GroupBy`, joins, functions) before nested/document shapes — tracked for the next DTQL cycle.

---
*This document follows the https://specscore.md/feature-specification*
