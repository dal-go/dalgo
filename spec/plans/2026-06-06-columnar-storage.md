# Plan: Columnar storage engine (dalgo2memory)

**Status:** Approved
**Source Feature:** columnar-storage
**Date:** 2026-06-06
**Owner:** alex
**Supersedes:** —

## Summary

Decomposes the `columnar-storage` Feature into nine linear tasks: engine selection, the typed-slice + slot layout, hybrid write with the fidelity opt-out, behavioral parity with Serialized, deletes + compaction, read/query reassembly + Serialized-equality, the `ColumnStrategy` seam + default strategy, wiring the equality `WHERE` to the strategy, and the `-race` concurrency gate. All fifteen acceptance criteria are covered by a task; none are deferred. This is the third of three coordinated plans and depends on the seam (and Serialized, as the behavioral reference) being implemented first.

## Approach

The columnar engine is the largest piece, so it is built bottom-up: representation first, then behavior, then the strategy seam, then the query wiring that uses it, then the concurrency gate. Task 1 adds `WithColumnarStorage(...)` selection (typed collections only, schemaless errors). Task 2 builds the typed column slices (reflection over `T`, `[]any` fallback) over a stable shared row-slot model. Task 3 adds the hybrid write (scalar direct / ref-bearing round-trip) and the schema-wide/per-collection fidelity opt-out. Task 4 establishes behavioral parity with the Serialized engine. Task 5 adds tombstone deletes + slot reuse + threshold compaction under the global lock. Task 6 reassembles rows for `Get`/query and proves columnar query results equal Serialized's. Task 7 defines and exports the `ColumnStrategy` interface + default typed-slice strategy. Task 8 wires the single supported equality `WHERE` predicate to the strategy with a scan fall-back. Task 9 is the `-race` gate over interleaved operations. The order respects dependencies: layout precedes everything; the strategy (7) precedes its WHERE wiring (8); the race gate (9) exercises the finished engine including compaction (5).

## Tasks

### Task 1: WithColumnarStorage selection (typed-only)

**Verifies:** columnar-storage#ac:columnar-requires-typed-collection

Add the `WithColumnarStorage(...ColumnOption)` `CollectionOption` that selects the columnar engine for a schema-registered `WithCollection[T]` collection, and make selection on a schemaless/undefined collection fail with a descriptive error.

### Task 2: Typed column slices over a stable row slot

**Verifies:** columnar-storage#ac:columns-are-typed-slices, columnar-storage#ac:slot-stable-across-columns
**Depends-On:** 1

Derive one column per JSON-serializable field of `T` via reflection, storing each in a slice typed to the field's Go type (`[]any` fallback for interface/heterogeneous fields), all indexed by a single shared per-row slot; map each record id to a slot that stays stable for the record's lifetime.

### Task 3: Hybrid write and fidelity opt-out

**Verifies:** columnar-storage#ac:write-breaks-refs-by-default, columnar-storage#ac:fidelity-opt-out-toggles-ref-breaking
**Depends-On:** 2

On write, store scalar/value columns by direct assignment and deep-copy reference-bearing columns via a serialization round-trip so stored data shares no references with caller values by default; add a schema-wide option and a per-collection option to disable ref-breaking (per-collection overriding schema-wide), defaulting to faithful.

### Task 4: Behavioral parity with the Serialized engine

**Verifies:** columnar-storage#ac:parity-with-serialized-ops
**Depends-On:** 2

Ensure `Set` overwrites, `Insert` on an existing id errors without overwriting, `Get`/`Update` on an absent id return not-found, and an `Update` naming a field undefined on `T` is rejected — matching the Serialized engine's outcomes exactly (representational differences only).

### Task 5: Tombstone deletes and compaction

**Verifies:** columnar-storage#ac:delete-tombstones-and-hides, columnar-storage#ac:compaction-preserves-live-records
**Depends-On:** 2

`Delete` marks a slot dead and frees it for reuse while keeping other slots stable, and a tombstoned slot never appears in `Get`/`Exists`/queries; when the dead-slot fraction crosses a threshold, compaction reclaims dead slots under the global write lock, preserving every live record and its values without exposing partial state to readers.

### Task 6: Row reassembly and Serialized-equality of query results

**Verifies:** columnar-storage#ac:get-and-query-reassemble, columnar-storage#ac:columnar-query-matches-serialized
**Depends-On:** 3, 5

Reassemble each live row from its per-column slot values into a `map[string]any` field view and a typed materialization for `Get`/`IntoRecord`/factory targets, with no shared references between rows; verify that the supported single equality `WHERE` predicate plus projection and `ORDER BY`/`LIMIT` (and join) return results identical in content and order to an equivalent Serialized collection.

### Task 7: ColumnStrategy interface and default typed-slice strategy

**Verifies:** columnar-storage#ac:strategy-interface-exported, columnar-storage#ac:default-strategy-scans-column
**Depends-On:** 2

Define and export a `ColumnStrategy` interface (write side to set/clear a value at a slot; equality read side returning matching live slots or "no opinion") and a per-column option to supply one; implement the default typed-slice strategy that answers equality by scanning comparable columns (Go `==`, as in `matchesWhere`) and may return "no opinion" for non-comparable `[]any` columns — proving an external package can plug in without a core dependency.

### Task 8: Wire equality WHERE to the column strategy

**Verifies:** columnar-storage#ac:where-uses-strategy, columnar-storage#ac:where-falls-back-on-no-opinion
**Depends-On:** 6, 7

For the single `field == value` predicate the adapter supports, consult the target column's `ColumnStrategy` and use its returned slot set to select candidate rows (the read side returns a set so future multi-predicate `AND` can intersect), falling back to scanning on "no opinion"; the result is identical to the Serialized engine's for the same query.

### Task 9: Concurrency race gate

**Verifies:** columnar-storage#ac:columnar-passes-race
**Depends-On:** 5, 8

Run the columnar engine under the existing single global write lock with `go test -race`, interleaving reads with writes, deletes, and a compaction, confirming no data race is reported and results remain correct.

## Open Questions

- The exact compaction trigger (dead-slot threshold; inline-on-write vs separate maintenance call) and the `ColumnStrategy` slot-set representation / rebuild-on-compaction signal — settled during implementation; both satisfy the ACs.

---
*This document follows the https://specscore.md/plan-specification*
