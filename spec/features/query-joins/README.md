# Feature: First-class INNER/LEFT joins in dal's query model

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/query-joins?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/query-joins?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/query-joins?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/query-joins?op=request-change) |
**Status:** Approved
**Date:** 2026-06-05
**Owner:** alex
**Source Ideas:** first-class-query-joins
**Supersedes:** —
**Grade:** A

## Summary

Make a two-table INNER or LEFT **equi-join** a first-class, losslessly-expressible shape in dalgo's `dal.StructuredQuery`, and prove it end-to-end by executing it in the in-memory adapter `dalgo2memory`. This closes the three gaps that forced DTQL (and any other consumer) to defer joins: no join-type distinction, no public way to build a join's `ON` clause, and no source qualifier on a field reference.

## Problem

`dal` can almost represent a join but not losslessly. `JoinedSource` is `RecordsetSource` + an **unexported** `on []Condition` with no public constructor, so a caller outside `dal` cannot populate the `ON` clause. There is no join-type enum, so an `INNER` join is indistinguishable from a `LEFT` join in the AST. And `FieldRef` is `{name, isID}` with no source qualifier, so once two recordsets are in play, every field in `ON`/`WHERE`/`Columns`/`OrderBy` is ambiguous (`u.id` vs `o.userId`). The complementary `dtql` Feature explicitly carves joins out (`REQ:covered-subset` requires "a single `From` whose `Joins()` is empty"); this Feature supplies the model `dtql` and the SQL adapters will later build on.

## Behavior

### Query model (the `dal` AST)

The AST gains the minimum needed to express one INNER or LEFT equi-join between two recordsets, with all field references unambiguously qualified.

#### REQ: join-type-distinction

`dal` MUST provide an exported join-type value that distinguishes an `INNER` join from a `LEFT` join, carried on each `JoinedSource` and readable through its public API. The type MAY leave room for `RIGHT`/`FULL`/`CROSS` as future values, but only `INNER` and `LEFT` are in scope for this Feature.

#### REQ: public-joined-source-constructor

`dal` MUST provide an exported constructor that builds a `JoinedSource` from a `RecordsetSource`, a join type, and one or more `ON` conditions, such that a caller in another package (e.g. `dalgo2memory`) can construct a fully-populated join and read it back via `From().Joins()` and `JoinedSource.On()` without access to unexported fields.

#### REQ: source-qualified-field-ref

`NewFieldRef` MUST accept a source qualifier — its signature becomes `NewFieldRef(src, name string)`. A non-empty `src` names the recordset a field belongs to (matched against a `RecordsetSource`'s `Alias()`, falling back to `Name()`); an empty `src` denotes the single `From` base and preserves today's single-source behavior. This is an intentional breaking signature change; all existing call sites in the module MUST be migrated.

#### REQ: equi-join-on-shape

For this Feature an `ON` clause in scope is one or more equality `Comparison` nodes whose `Left` and `Right` are each a source-qualified `FieldRef`. Non-equality and arbitrary boolean `ON` trees are out of scope and need not be expressible or executable yet.

### In-memory execution (`dalgo2memory`)

`dalgo2memory` learns to run the new shape so the model is proven against real data, not just constructible.

#### REQ: memory-inner-join

`dalgo2memory` MUST execute an `INNER` equi-join between the `From` base and a single joined recordset, returning exactly the rows whose qualified `ON` fields are equal — unmatched rows on either side are excluded.

#### REQ: memory-left-join

`dalgo2memory` MUST execute a `LEFT` equi-join, returning every row of the `From` base, with the joined recordset's fields present-and-matched where the `ON` equality holds and absent/`nil` where no right row matches.

#### REQ: memory-qualified-resolution

When executing a query with joins, `dalgo2memory` MUST resolve a qualified `FieldRef` to the correct recordset — empty `src` to the `From` base, a non-empty `src` to the joined source whose `Alias()`/`Name()` matches — when evaluating `ON`, `Where`, `Columns`, and `OrderBy`. An existing single-source query (all fields empty-`src`) MUST return the same results as before this Feature. A non-empty `src` that matches no recordset in the query MUST produce a descriptive error, not a silent empty or absent value.

#### REQ: memory-rejects-unsupported-join

When `dalgo2memory` encounters a join whose type it does not support (anything other than `INNER` or `LEFT`, including a chained second join), it MUST return a clear, descriptive error rather than silently producing incorrect rows.

## Acceptance Criteria

### AC: join-type-readable (verifies REQ:join-type-distinction)

**Given** two `JoinedSource` values, one built with the `INNER` type and one with the `LEFT` type
**When** a caller reads each source's join type through the public API
**Then** the two values are distinguishable and each reports the type it was constructed with.

### AC: build-join-from-outside-dal (verifies REQ:public-joined-source-constructor, REQ:equi-join-on-shape)

**Given** a `From` base `users u`, a second recordset `orders o`, and an equality `ON` condition `u.id == o.userId` built from qualified field references
**When** code in the `dalgo2memory` package calls the exported constructor with the joined source, a join type, and the `ON` condition, then reads the join back through the public API (its join type and its `On()` conditions)
**Then** the returned join carries the join type and the exact `ON` condition, with no access to any unexported `dal` field required.

### AC: qualified-field-carries-source (verifies REQ:source-qualified-field-ref)

**Given** the `NewFieldRef(src, name)` signature
**When** a field is built as `NewFieldRef("o", "userId")` and another as `NewFieldRef("", "name")`
**Then** the first reports source `o` and name `userId`, and the second reports an empty source denoting the `From` base.

### AC: existing-single-source-unaffected (verifies REQ:source-qualified-field-ref, REQ:memory-qualified-resolution)

**Given** an existing single-source `dalgo2memory` query whose fields use an empty `src`
**When** it is executed after the `NewFieldRef` signature change and the join-resolution logic are in place
**Then** it returns the same records, in the same order, as before this Feature (the full existing `dal` + `dalgo2memory` + `dtql` suites stay green).

### AC: inner-join-matches-only (verifies REQ:memory-inner-join)

**Given** in-memory `users` `[{id:1},{id:2}]` and `orders` `[{userId:1},{userId:1},{userId:9}]`
**When** `dalgo2memory` executes an `INNER` join `users u JOIN orders o ON u.id == o.userId`
**Then** the result contains exactly the two `u.id==1 / o.userId==1` pairings and excludes `users.id==2` and `orders.userId==9`.

### AC: left-join-keeps-unmatched-left (verifies REQ:memory-left-join)

**Given** the same `users` and `orders` data
**When** `dalgo2memory` executes a `LEFT` join `users u LEFT JOIN orders o ON u.id == o.userId`
**Then** the result includes the two matched pairs for `u.id==1` **and** one row for `u.id==2` with the `o` fields absent/`nil`.

### AC: qualified-resolution-in-where (verifies REQ:memory-qualified-resolution)

**Given** a `LEFT` join of `users u` and `orders o` and a `Where` referencing both `u`-qualified and `o`-qualified fields
**When** `dalgo2memory` evaluates the query
**Then** each qualified field is read from its own recordset (an `o.`-qualified predicate on an unmatched left row sees an absent value, not a `u` field of the same name).

### AC: unsupported-join-errors (verifies REQ:memory-rejects-unsupported-join)

**Given** a query whose `From` carries a join of an unsupported type (e.g. a reserved `RIGHT` value, or a second chained join)
**When** `dalgo2memory` is asked to execute it
**Then** it returns a descriptive error naming the unsupported shape and yields no result rows.

### AC: unresolvable-source-errors (verifies REQ:memory-qualified-resolution)

**Given** a `LEFT` join of `users u` and `orders o` and a field qualified with a `src` (`x`) that names neither recordset
**When** `dalgo2memory` evaluates the query
**Then** it returns a descriptive error naming the unresolved source and yields no result rows, rather than treating the field as empty.

## Architecture & Components

- **`dal` (query model).** New: a join-type value (enum), an exported `JoinedSource` constructor accepting `(src RecordsetSource, joinType, on ...Condition)`, and the `NewFieldRef(src, name)` signature change. `FromSource.Join`/`Joins` and `JoinedSource.On` already exist and are reused. No new dependencies.
- **`dalgo2memory` (executor).** Extends the current single-collection scan (today it reads only `q.From().Base().Name()` and ignores `Joins()`) with a nested-loop join over the in-memory maps and a qualified-field resolver keyed on source `Alias()`/`Name()` with empty-`src` → base.
- **Consumers unblocked (out of scope here):** `dtql` join serialization and the SQL adapters (`dalgo2sql`/`dalgo2sqlite`) build on the same AST later.

## Data Flow

`NewFieldRef(src,name)` + `NewJoinedSource(...)` build a `FromSource` with one `JoinedSource` → `QueryBuilder` produces a `StructuredQuery` → `dalgo2memory` reads `From().Base()` and `From().Joins()`, runs a nested-loop join on the two map collections, resolves each qualified `FieldRef` against the matching recordset for `ON`/`Where`/`Columns`/`OrderBy`, and emits the joined records (right side `nil` for unmatched `LEFT` rows).

## Error Handling & Failure Modes

- Unsupported join type or a chained/second join → descriptive error from `dalgo2memory`, no rows (`REQ:memory-rejects-unsupported-join`).
- A qualified `src` matching no recordset in the query → descriptive error rather than silent empty/wrong result.
- Empty-`src` fields in a multi-source query are ambiguous only if intended for a joined source; they resolve to the base by definition, which is the documented single-source rule.

## Testing Strategy

Table tests in `dalgo2memory` over two small in-memory collections cover INNER (matches only), LEFT (unmatched-left retained with `nil` right), qualified resolution in `Where`, the unsupported-join error, and the unresolvable-`src` error. `dal`-level tests cover the join-type round-trip and the public constructor producing a populated `On()` from outside the package. The `NewFieldRef(src, name)` signature change has only two constructor-call surfaces in the module — `dal` itself (the `Field`/`AscendingField`/`DescendingField` wrappers and `dal` tests) and `dtql/deserialize.go`; `dalgo2memory` consumes `FieldRef` by type assertion rather than the constructor. The migration is verified by the full existing `dal` + `dtql` + `dalgo2memory` suites returning green.

## Out of Scope

- `RIGHT` / `FULL` / `CROSS` joins — the type may reserve them, but only `INNER`/`LEFT` are supported.
- Non-equality or arbitrary boolean `ON` trees — equi-join only.
- SQL rendering/execution (`dalgo2sql`, `dalgo2sqlite`) — separate repos, unblocked by this Feature.
- Multiple/chained joins (3+ recordsets) — single join (two recordsets) only.
- DTQL serialization of joins — a later `dtql` cycle.
- Requiring a non-empty `src` on every field — empty `src` stays valid and means the base.

## Assumption Carryover

From the source Idea `first-class-query-joins`:

- **Carried (Must):** the `NewFieldRef(src, name)` change is a bounded, mechanical migration of all in-module call sites — validated by the suites going green.
- **Carried (Must):** an equi-join is buildable and executable purely through public `dal` constructors — validated by `AC:build-join-from-outside-dal`.
- **Carried (Should):** a nested-loop join in `dalgo2memory` is sufficient to prove correctness including `LEFT` null-filling — validated by the INNER/LEFT ACs.
- **Carried (Should):** qualification can reuse `RecordsetSource.Alias()`/`Name()` and empty `src` → base — now a requirement (`REQ:memory-qualified-resolution`).
- **Deferred (Might):** equality-only `ON` covers the dominant need — encoded as `REQ:equi-join-on-shape`; richer `ON` stays out of scope.

## Rehearse Integration

All nine ACs are testable through pure Go surfaces — `dal` constructor/accessor calls and `dalgo2memory` query execution over in-memory maps — so they map directly to table tests in `dal/` and `dalgo2memory/` (see `## Testing Strategy`). Per-AC Rehearse stub files are intentionally **deferred to the Plan**, where each AC becomes a concrete `*_test.go` case; the rehearsal surface is the Go test suite rather than standalone scenario docs.

## Open Questions

- Exact spelling/exported names of the join-type constants (e.g. `dal.JoinInner`/`dal.JoinLeft`) and the constructor — settle in the Plan.
- Whether a single-arg convenience wrapper for the common empty-`src` field is worth adding alongside `NewFieldRef(src, name)`.
- Whether unsupported join types are rejected at construction in `dal` or only at execution in `dalgo2memory` (this Feature requires at least the execution-time rejection).
- `JoinedSource.On()` currently has a pointer receiver while `From().Joins()` returns values; the Plan must make a join readable by value (a value receiver or returning pointers) so `AC:build-join-from-outside-dal` is achievable as written.

---
*This document follows the https://specscore.md/feature-specification*
