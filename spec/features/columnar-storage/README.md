---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Columnar storage engine (dalgo2memory)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/columnar-storage?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/columnar-storage?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/columnar-storage?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/columnar-storage?op=request-change) |
**Status:** Stable
**Date:** 2026-06-06
**Owner:** alex
**Source Ideas:** dalgo2memory-storage-engines
**Supersedes:** —
**Grade:** A

## Summary

An opt-in columnar storage engine for typed `dalgo2memory` collections. Each column of the record type is stored in its own strongly-typed slice over a stable per-row slot model (tombstone + bulk compaction). The engine exposes a pluggable, exported `ColumnStrategy` per column — with an equality read-side wired into `WHERE` — so an out-of-core package (e.g. a bitmap index) can accelerate filtering later without `dalgo2memory` taking a dependency. Observable results for the operations and queries the adapter supports are identical to the Serialized engine; only the in-memory representation differs.

## Problem

The Serialized engine stores every record as opaque bytes and `json.Unmarshal`s on every read and every row of a scan. For DataTug analytics and larger in-memory datasets, equality `WHERE` filtering over many rows is the hot path (and multi-field filtering is the eventual goal once the query engine supports it), and a row-of-bytes layout makes per-field access expensive and gives an index nowhere to attach. A columnar layout (one typed slice per field) is the representation that makes field access cheap and gives a per-column index a natural home. This Feature delivers that engine for typed collections, behind the storage-engine seam, while keeping the adapter's observable behavior and its single-writer concurrency model unchanged.

## Dependencies

- storage-engine-seam
- serialized-storage

## Behavior

### Selecting the columnar engine

#### REQ: columnar-selection

A `WithColumnarStorage(...ColumnOption)` `CollectionOption` MUST be provided that selects the columnar engine for a schema-registered `WithCollection[T]` collection. Selecting columnar storage for a collection that is not a schema-registered typed collection MUST fail with a descriptive error. (`map[string]any` mixed-mode columnar collections are out of scope for this Feature — see Out of Scope.)

### Columnar layout

#### REQ: typed-column-slices

The engine MUST derive one column per JSON-serializable field of the record type `T` and store each column's values in a slice typed to that field's Go type (e.g. `[]int`, `[]string`, `[]time.Time`). A column MUST fall back to an untyped `[]any` slice only when the field's type is an interface or otherwise has no single concrete element type. All of a collection's column slices MUST be indexed by a single shared per-row slot.

#### REQ: stable-row-slot

Each stored record MUST occupy one slot index shared across every column slice, and the record's id MUST map to that slot. A live record's slot MUST remain stable for the record's lifetime (until deletion or compaction), so all of a record's per-column values are always found at the same index.

### Write semantics and fidelity

#### REQ: hybrid-write-fidelity

On write, a scalar/value-typed column (numbers, strings, bools, and other reference-free values) MUST be stored by direct assignment into its typed slice, and a reference-bearing column (struct, slice, map, or pointer values) MUST be deep-copied via a serialization round-trip before storage, so that — by default — stored data shares no references with the caller's value. Mutating the caller's value after a write MUST NOT affect stored data.

#### REQ: fidelity-opt-out

A schema-wide option and a per-collection option MUST be provided to disable ref-breaking. When ref-breaking is disabled for a collection, reference-bearing column values MAY be stored without the round-trip (trading fidelity for write speed). The default MUST be faithful (ref-breaking on), and a per-collection setting MUST override the schema-wide setting.

#### REQ: behavioral-parity-with-serialized

For the operations the adapter supports, the columnar engine MUST present the same observable semantics as the Serialized engine: `Set` overwrites; `Insert` on an existing id errors and does not overwrite; `Get`/`Update` on an absent id return not-found; an `Update` naming a field not defined on `T` is rejected with a descriptive error. Differences between engines MUST be representational only, never behavioral.

### Deletes and compaction

#### REQ: tombstone-delete

`Delete` MUST mark the record's slot dead (tombstone) and make it available for reuse by a later insert, while keeping every other record's slot stable. A tombstoned slot MUST NOT appear in `Get`, `Exists`, or any query result.

#### REQ: compaction

When the fraction of dead slots crosses a threshold, the engine MUST compact — reclaiming dead slots — while preserving every live record and its reassembled values. Compaction MUST run under the existing global write lock and MUST NOT be observable to readers as partial or inconsistent state.

### Read and query

#### REQ: row-reassembly

For `Get` and query execution, the engine MUST reassemble each live row from its per-column slot values into the shapes the adapter consumes: a `map[string]any` field view (for WHERE/GROUP BY/ORDER BY/projection) and a typed materialization into `Get`'s record or the query's `IntoRecord`/factory target. Reassembled rows MUST share no references with one another, nor with stored data when the collection is faithful.

#### REQ: query-results-identical

For every query the adapter supports (single-source and join, with the currently supported equality `WHERE`, plus projection/GROUP BY/ORDER BY/LIMIT), a columnar collection MUST return results identical to those an equivalent Serialized collection returns for the same data and query.

### The ColumnStrategy seam

#### REQ: column-strategy-interface

The engine MUST define and export a `ColumnStrategy` interface with a write side (set or clear a column's value at a given slot) and an equality read side (given a value, return the set of live slots whose column equals it, or signal "no opinion"). A per-column `ColumnOption` MUST supply the strategy for a named column; columns without an explicit option use the default strategy. Equality on the read side is defined as the adapter's existing value equality (Go `==` on the comparable decoded value, as in `matchesWhere`); the read side is not required to answer for non-comparable column values and MAY signal "no opinion" for them.

#### REQ: default-strategy-equality

The default per-column strategy MUST be the typed-slice strategy. For a column whose element type is comparable (Go `==` is valid — numbers, strings, bools, and other comparable values), it MUST answer the equality read side by scanning its slice and returning the matching live slots, using the same value equality the adapter's existing WHERE uses. For a column whose element type is not comparable (e.g. an `[]any` fallback column holding slices/maps, or a struct containing them), the default strategy MAY return "no opinion", deferring to the scanning fall-back.

#### REQ: where-equality-uses-strategy

For the equality `WHERE` predicate the adapter currently supports — a single `field == value` comparison — the query engine MUST consult the target column's `ColumnStrategy`: when the strategy returns a slot set, the engine MUST use it to select candidate rows rather than scanning all rows; a "no opinion" MUST fall back to scanning. The read side MUST return a set of slots so that, if and when the adapter grows multi-predicate (`AND`) `WHERE`, the engine can intersect per-predicate sets — but this Feature wires only the single supported predicate and MUST NOT broaden predicate support. The selected result MUST be identical to the Serialized engine's result for the same query.

#### REQ: external-strategy-pluggable

The `ColumnStrategy` interface and the per-column option MUST be exported so a separate package (e.g. `bitmap4dalgo2memory`) can provide a column strategy that plugs into `WithColumnarStorage`, and `dalgo2memory` MUST NOT import or depend on any such extension package.

### Concurrency

#### REQ: single-lock-race-free

Columnar storage MUST operate under the existing single global write lock with no additional locking. Slot allocation, writes, updates, tombstones, compaction, and concurrent reads MUST be race-free, verified with `go test -race`.

## Acceptance Criteria

### AC: columnar-requires-typed-collection (verifies REQ:columnar-selection)

**Given** a schema-registered `WithCollection[T]` collection selected with `WithColumnarStorage()` and, separately, a schemaless/undefined collection selected with `WithColumnarStorage()`
**When** each database is constructed/used
**Then** the typed collection uses the columnar engine successfully, and the schemaless selection fails with a descriptive error.

### AC: columns-are-typed-slices (verifies REQ:typed-column-slices)

**Given** a columnar collection for a type `T` with `int`, `string`, and `bool` fields plus one `interface{}` field
**When** records are stored and the engine's column storage is inspected
**Then** the `int`/`string`/`bool` columns are slices of those concrete types and the `interface{}` field's column is an `[]any` slice.

### AC: slot-stable-across-columns (verifies REQ:stable-row-slot)

**Given** several records stored in a columnar collection
**When** any record is read back
**Then** all of that record's field values are drawn from the same slot index across the column slices, and a record's slot does not change when other records are written.

### AC: write-breaks-refs-by-default (verifies REQ:hybrid-write-fidelity)

**Given** a faithful columnar collection whose type has a scalar field and a reference-bearing field (e.g. a nested struct or slice)
**When** a record is written and the caller then mutates the reference-bearing field
**Then** a subsequent read returns the value as it was at write time, unaffected by the mutation.

### AC: fidelity-opt-out-toggles-ref-breaking (verifies REQ:fidelity-opt-out)

**Given** the same type used in (a) a default collection, (b) a collection with ref-breaking disabled per-collection, and (c) a database with ref-breaking disabled schema-wide but one collection re-enabling it
**When** a reference-bearing value is written and then mutated by the caller
**Then** the faithful collections (a and the re-enabled c collection) are unaffected, the opt-out collection (b) reflects the mutation, and the per-collection setting overrides the schema-wide default.

### AC: parity-with-serialized-ops (verifies REQ:behavioral-parity-with-serialized)

**Given** identical data in a columnar and a Serialized collection of the same type
**When** `Set`/`Insert`-duplicate/`Get`-absent/`Update`-absent and an `Update` naming an undefined field are exercised on each
**Then** both engines produce the same outcomes (overwrite, already-exists error, not-found, not-found, undefined-field rejection).

### AC: delete-tombstones-and-hides (verifies REQ:tombstone-delete)

**Given** a columnar collection with records `a`, `b`, `c`
**When** `b` is deleted
**Then** `Get`/`Exists` for `b` report absence and queries omit `b`, while `a` and `c` remain readable at unchanged slots, and a later insert may reuse `b`'s freed slot.

### AC: compaction-preserves-live-records (verifies REQ:compaction)

**Given** a columnar collection with enough deletions to cross the compaction threshold
**When** compaction runs
**Then** every live record is still readable with its correct values and query results are unchanged, and dead slots have been reclaimed.

### AC: get-and-query-reassemble (verifies REQ:row-reassembly)

**Given** a columnar collection of typed records
**When** a record is fetched via `Get` into a typed target and a query materializes rows into `IntoRecord` targets
**Then** each reassembled record equals the stored data, and two reassembled rows share no references.

### AC: columnar-query-matches-serialized (verifies REQ:query-results-identical)

**Given** identical data in a columnar and a Serialized collection
**When** the same query — the supported single equality `WHERE` predicate, plus a projection and an `ORDER BY`/`LIMIT` — runs against each
**Then** the result rows are identical in content and order.

### AC: strategy-interface-exported (verifies REQ:column-strategy-interface, REQ:external-strategy-pluggable)

**Given** the `dalgo2memory` package
**When** its exported surface is inspected
**Then** a `ColumnStrategy` interface with a write side and an equality read-side is exported, a per-column option accepts a strategy, and a test-only external strategy can be supplied via `WithColumnarStorage` without `dalgo2memory` importing the strategy's package.

### AC: default-strategy-scans-column (verifies REQ:default-strategy-equality)

**Given** a columnar collection using the default strategy on a `string` column
**When** the strategy's equality read-side is queried for a value present in some rows
**Then** it returns exactly the live slots whose column equals that value (and does not return "no opinion").

### AC: where-uses-strategy (verifies REQ:where-equality-uses-strategy)

**Given** a columnar collection and a custom test strategy on the filtered column that records when its equality read-side is invoked
**When** a query with the supported single `field == value` predicate runs
**Then** the engine consults the column's strategy and uses the returned slot set to select candidates (the custom strategy records the invocation) rather than scanning all rows, and the result is identical to the Serialized engine's result for the same query.

### AC: where-falls-back-on-no-opinion (verifies REQ:where-equality-uses-strategy, REQ:default-strategy-equality)

**Given** a columnar collection whose target column's strategy returns "no opinion" for the predicate (e.g. a custom strategy, or the default strategy on a non-comparable `[]any` column)
**When** an equality query on that column runs
**Then** the engine falls back to scanning and still returns the correct result, identical to the Serialized engine.

### AC: columnar-passes-race (verifies REQ:single-lock-race-free)

**Given** the columnar engine under the existing global write lock
**When** the test suite runs with `-race`, interleaving reads with writes, deletes, and a compaction
**Then** no data race is reported and results remain correct.

## Architecture & Components

- **`columnarEngine` (new, internal).** Implements the seam's `storageEngine` interface. Holds: an ordered set of typed column containers (one per `T` field), an `id ↔ slot` mapping, a free-list of tombstoned slots, and a live/dead marker per slot. Column containers are built by reflection over `T`'s fields (concrete typed slice, `[]any` fallback).
- **`ColumnStrategy` interface + default typed-slice strategy (new, exported).** Write side records/clears a value at a slot; equality read side returns matching live slots or "no opinion". The default strategy wraps the typed slice and scans it. `WithColumnarStorage(...ColumnOption)` and a per-column option (carrying a strategy constructor) are exported so external packages can plug in.
- **Write path.** Marshal-free for scalar columns (direct slot assignment); serialization round-trip for reference-bearing columns when faithful (skipped when the opt-out is set). Insert/duplicate/not-found/undefined-field behavior mirrors the Serialized engine.
- **Read/query path.** `row-reassembly` builds the `map[string]any` view and typed targets from slot values, reusing the existing WHERE/GROUP BY/ORDER BY/projection/join logic; for the supported single equality `WHERE` predicate, a candidate-selection step consults the column's strategy and uses its slot set (generalizing to intersection if multi-predicate WHERE is later added) before reassembly.
- **Compaction.** Triggered by a dead-slot threshold under the write lock; rebuilds the slot mapping and column slices, updating the free-list. The default strategy is rebuilt from the compacted slices; an external strategy is notified to rebuild via its write side.

## Data Flow

Write: resolve slot (reuse a free slot or append) → per column, direct-assign (scalar) or round-trip-then-assign (ref-bearing, when faithful) → mark slot live → update each column's strategy write side. Read (`Get`): id → slot → reassemble typed target from column values. Query: candidate slots = the equality predicate's column-strategy matching slots (or full live-slot scan on "no opinion"; generalizes to intersection across predicates if multi-predicate WHERE is later supported) → reassemble `map[string]any` for filter/group/order/projection and typed targets for results. Delete: id → slot → mark dead, push to free-list, clear strategies. Compaction (threshold crossed): copy live slots down, rebuild mapping/slices/strategies.

## Error Handling & Failure Modes

- `WithColumnarStorage` on a non-typed/unregistered collection → descriptive error (`REQ:columnar-selection`).
- `Insert` on an existing id → already-exists error; `Get`/`Update` on an absent id → not-found; `Update` of an undefined field → descriptive error — all matching the Serialized engine (`REQ:behavioral-parity-with-serialized`).
- A reference-bearing value that fails its serialization round-trip while faithful → descriptive error, nothing stored.
- A column strategy returning "no opinion" is not an error — it is the defined fall-back to scanning (`REQ:where-equality-uses-strategy`).

## Testing Strategy

Table tests plus `-race` runs covering: columnar selection (typed ok / schemaless error); typed-slice columns with `[]any` fallback; slot stability; write ref-breaking and the schema-wide/per-collection opt-out matrix; behavioral parity with Serialized (insert-dup, not-found, undefined-field); tombstone hide + slot reuse; compaction preserving live records; reassembly equality and non-shared references; columnar-vs-Serialized query equality (the supported single equality predicate, projection, ORDER BY/LIMIT, join); exported `ColumnStrategy` with a test-only external strategy; default strategy scanning (comparable column) and "no opinion" on a non-comparable `[]any` column; WHERE consulting the strategy and falling back on "no opinion"; and a `-race` interleaving of reads/writes/deletes/compaction. `dalgo2memory` MUST remain at 100% statement coverage, and `var _ storageEngine = (*columnarEngine)(nil)` plus `var _ ColumnStrategy = (*typedSliceStrategy)(nil)` guard the interfaces.

## Rehearse Integration

All ACs are exercisable through pure Go — `dalgo2memory` operations and queries over in-memory typed collections, a parallel Serialized collection for equality comparison, a test-only `ColumnStrategy`, and a `-race` interleaving — so they map directly to table/`-race` tests (see `## Testing Strategy`). Per-AC Rehearse stub files are deferred to the Plan, where each AC becomes a concrete `*_test.go` case.

## Out of Scope

- **Mixed-mode `map[string]any` columnar storage** (explicitly declared typed columns with undeclared fields kept in a parallel `[]map[string]any`) — deferred to a separate follow-on Feature; this Feature covers typed `WithCollection[T]` collections only.
- **Bitmap / roaring-bitmap indexing** — an out-of-core extension (`bitmap4dalgo2memory`) that plugs into the exported `ColumnStrategy`; not built here, only designed for.
- **Multi-predicate (`AND`/`OR`) `WHERE`** — the adapter currently supports a single equality predicate on this path; this Feature wires the strategy for that predicate and does not broaden WHERE. The `ColumnStrategy` slot-set contract is shaped so intersection can be added when multi-predicate WHERE lands.
- **Range/inequality/composite read-side acceleration** — the `ColumnStrategy` read side covers equality only; other predicates fall back to scanning.
- **Per-collection two-phase locking and transactional rollback** — the single global write lock is retained (deferred per the source Idea).
- **Broadening query-predicate support** beyond what the adapter already supports — columnar matches the Serialized engine's behavior, it does not extend it.
- **Persistence/durability** — storage remains in-memory.

## Assumption Carryover

From the source Idea `dalgo2memory-storage-engines`:

- **Carried (Must):** columnar slot/tombstone/compaction is correct and race-free under the single global write lock — `REQ:tombstone-delete`, `REQ:compaction`, `REQ:single-lock-race-free` (validated by `AC:columnar-passes-race`).
- **Carried (Must):** columnar backs the full existing query surface with identical results — `REQ:row-reassembly`, `REQ:query-results-identical`.
- **Carried (Must):** the exported `ColumnStrategy` is sufficient for an external package to back a column without a core dependency — `REQ:column-strategy-interface`, `REQ:external-strategy-pluggable`.
- **Carried (Should):** strongly-typed slices per column (with `[]any` fallback) — `REQ:typed-column-slices`.
- **Resolved:** fidelity is faithful by default with a schema-wide/per-collection opt-out, realized via the hybrid (scalar direct / ref-bearing round-trip) write — `REQ:hybrid-write-fidelity`, `REQ:fidelity-opt-out`.
- **Deferred (Might):** a type-copy engine and bitmap extension — other work, not this Feature.

## Open Questions

- The compaction trigger: the exact dead-slot threshold and whether compaction runs inline on a write that crosses it or via a separate maintenance call — a Plan-time choice; both satisfy `REQ:compaction`.
- The precise signatures of the `ColumnStrategy` interface (e.g. slot-set representation, how a strategy is notified to rebuild on compaction) — an implementation detail for the Plan.
- The exact rule for when a field's column is typed vs `[]any` (e.g. named interface types, `json.RawMessage`) — refined at implementation against `T` reflection.

---
*This document follows the https://specscore.md/feature-specification*
