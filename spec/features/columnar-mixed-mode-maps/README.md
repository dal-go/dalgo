---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Mixed-mode columnar storage for map[string]any collections (dalgo2memory)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/columnar-mixed-mode-maps?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/columnar-mixed-mode-maps?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/columnar-mixed-mode-maps?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/columnar-mixed-mode-maps?op=request-change) |
**Status:** Stable
**Date:** 2026-06-06
**Owner:** alex
**Source Ideas:** dalgo2memory-storage-engines
**Supersedes:** —
**Grade:** A

## Summary

Extends the columnar engine to `map[string]any`-backed collections in **mixed mode**: a caller declares which columns to store columnarly; those declared fields are kept in typed columnar slices (and removed from each record's map), while every undeclared field is kept in a parallel `[]map[string]any` "leftover" slice indexed by the same per-row slot. Reads merge the declared columns back with the leftover map to reconstruct the full record. This brings columnar's typed-slice storage and `ColumnStrategy` acceleration to schemaless map records without forcing a Go struct type.

## Problem

The `columnar-storage` Feature requires a Go struct record type `T` — it errors when `WithColumnarStorage` is selected for a `map[string]any` (schemaless) collection, because it derives columns by reflecting over `T`'s fields. But many `dalgo2memory` callers (DataTug query results, dynamic records) use `map[string]any`. They cannot benefit from columnar typed slices or from a per-column index, even when they know which fields are hot. Mixed mode closes that gap: declare the hot columns, store them columnarly with strategies, and keep the open-ended remainder in a per-row map — without giving up the dynamic shape `map[string]any` collections rely on.

## Dependencies

- columnar-storage

## Behavior

### Selecting mixed mode

#### REQ: mixed-mode-selection

`WithColumnarStorage` MUST be selectable for a `map[string]any`-backed collection when at least one column is explicitly declared (see `REQ:declared-column-option`). Selecting columnar storage for a `map[string]any` collection with **no** declared column MUST fail with a descriptive error (the engine cannot infer columns from a dynamic map). (Struct-typed collections are unchanged from `columnar-storage` and out of scope here — see Out of Scope.)

#### REQ: declared-column-option

A `ColumnOption` MUST be provided that declares a columnar column by name together with its element type for a map-backed collection, so the column can be stored in a strongly-typed slice.

#### REQ: declared-value-type-enforced

When a written record carries a declared column whose value cannot be stored as that column's declared element type, the write MUST fail with a descriptive error and store nothing for that record — the value MUST NOT be silently dropped or coerced away. (The exact boundary for numeric values arriving as `float64` from a decoded map is an Open Question; a clearly-incompatible value such as a string for an `int` column is an error.)

### Mixed storage layout

#### REQ: declared-columns-typed-slices

Each declared column MUST be stored in a strongly-typed slice (the same typed-column representation as `columnar-storage`), indexed by the shared per-row slot, and the declared field MUST be removed from the record's stored leftover map so it is not stored twice.

#### REQ: leftover-map-storage

All fields of a stored record that are not declared columns MUST be retained in a parallel leftover store — a `[]map[string]any` (one map per row) indexed by the same per-row slot used by the declared columns. A record with only declared fields stores an empty (or nil) leftover map at its slot; a record with only undeclared fields stores all of them in the leftover map.

#### REQ: shared-slot-across-columns-and-leftover

The declared-column slices and the leftover-map slice MUST share one per-row slot index, and tombstone delete, slot reuse, and compaction MUST move (or free) a row's declared-column cells and its leftover map together so they never desynchronize.

### Reading and querying

#### REQ: read-merges-declared-and-leftover

`Get` and query reassembly MUST reconstruct each record as a `map[string]any` by merging the declared columns' slot values with that slot's leftover map, producing a record equal to what was written. A declared column and a leftover field MUST NOT both supply the same key; the merged record MUST contain each original field exactly once.

#### REQ: where-on-declared-uses-strategy

An equality `WHERE` predicate on a **declared** column MUST use that column's `ColumnStrategy` (default typed-slice strategy, or an external one) to select candidate slots, exactly as `columnar-storage` does. An equality `WHERE` predicate on an **undeclared** (leftover-map) field MUST fall back to scanning the leftover maps. In both cases the result MUST be identical to what the Serialized engine returns for the same `map[string]any` data and query.

### Inherited columnar behavior

#### REQ: inherits-fidelity-and-parity

Mixed-mode collections MUST inherit `columnar-storage`'s write fidelity and behavioral parity: declared scalar columns are stored by direct assignment and declared reference-bearing columns are deep-copied (subject to the existing fidelity opt-out); the leftover map's values are deep-copied on write under the faithful default so post-write caller mutation does not affect stored data; and `Set`/`Insert`/`Delete`/`Update` present the same observable outcomes (overwrite, insert-duplicate error, not-found) as the Serialized engine for the same `map[string]any` data.

## Acceptance Criteria

### AC: mixed-mode-requires-declared-column (verifies REQ:mixed-mode-selection, REQ:declared-column-option)

**Given** a `map[string]any` collection selected with `WithColumnarStorage` and one declared column, and separately a `map[string]any` collection selected with `WithColumnarStorage` and no declared column
**When** each database is constructed/used
**Then** the first uses the mixed-mode columnar engine successfully, and the second fails with a descriptive error stating columns must be declared for a map-backed columnar collection.

### AC: declared-value-wrong-type-errors (verifies REQ:declared-value-type-enforced)

**Given** a mixed-mode collection declaring column `age` as an `int`
**When** a record is written whose `age` value is a clearly-incompatible type (e.g. the string `"old"`)
**Then** the write returns a descriptive error, and no record is stored for that id (the value is neither dropped nor coerced).

### AC: declared-field-removed-from-leftover (verifies REQ:declared-columns-typed-slices)

**Given** a mixed-mode collection declaring column `age` (an `int`) over records that also carry undeclared fields `name` and `email`
**When** a record `{age, name, email}` is stored and the engine's storage is inspected
**Then** `age` is held in a typed `[]int` column at the record's slot and is absent from that slot's leftover map, while `name` and `email` remain in the leftover map.

### AC: leftover-holds-undeclared-fields (verifies REQ:leftover-map-storage)

**Given** a mixed-mode collection declaring only `age`
**When** records with varying undeclared fields are stored (one with `{age, name}`, one with `{age}` only, one with `{age, city, note}`)
**Then** each slot's leftover map contains exactly that record's undeclared fields (empty/nil for the `{age}`-only record), indexed by the same slot as its `age` value.

### AC: slot-stays-synced-through-delete-and-compaction (verifies REQ:shared-slot-across-columns-and-leftover)

**Given** a mixed-mode collection with several records and enough deletions to trigger compaction
**When** records are deleted (freeing slots, later reused) and compaction runs
**Then** every surviving record's declared-column value and its leftover map remain paired at the same slot, and every record reads back with its correct declared and undeclared fields.

### AC: read-reconstructs-full-record (verifies REQ:read-merges-declared-and-leftover)

**Given** a mixed-mode collection declaring `age` over records carrying `age`, `name`, and `email`
**When** a record is fetched via `Get` and via a query
**Then** the reconstructed `map[string]any` equals the originally written record (all of `age`, `name`, `email` present exactly once), and the merge never duplicates or drops a key.

### AC: where-on-declared-column-accelerated (verifies REQ:where-on-declared-uses-strategy)

**Given** a mixed-mode collection declaring `age`, with a custom test `ColumnStrategy` on `age` that records when its equality read-side is invoked, and an equivalent Serialized collection over the same map data
**When** an equality query `age == N` runs against the mixed-mode collection
**Then** the declared column's strategy is consulted to select candidates, and the result rows are identical in content and order to the Serialized collection's result for the same query.

### AC: where-on-leftover-field-scans (verifies REQ:where-on-declared-uses-strategy)

**Given** a mixed-mode collection declaring `age` (so `city` is an undeclared leftover field), and an equivalent Serialized collection over the same map data
**When** an equality query `city == "X"` runs against the mixed-mode collection
**Then** the engine falls back to scanning the leftover maps and returns rows identical in content and order to the Serialized collection's result.

### AC: mixed-mode-parity-and-fidelity (verifies REQ:inherits-fidelity-and-parity)

**Given** a faithful mixed-mode collection declaring a column over `map[string]any` records that include a declared reference-bearing value and an undeclared reference-bearing value
**When** a record is written, the caller mutates both the declared and the undeclared field after the write, and the same `Set`/`Insert`-duplicate/`Get`-absent operations are run against a Serialized collection
**Then** a subsequent read of the mixed-mode record is unaffected by the post-write mutation, and the operation outcomes match the Serialized engine's.

## Architecture & Components

- **Mixed-mode `columnarEngine` path (extended).** The existing `columnarEngine` gains a map-backed construction path: instead of reflecting over a struct `T`, it builds its typed columns from the declared-column options, and adds a `leftover []map[string]any` slice indexed by the same slot vector (`live`/`slotToID`/`idToSlot`/`freeList`). The struct-typed path is unchanged.
- **Declared-column option (new).** A `ColumnOption` (e.g. a `WithDeclaredColumn`-style option carrying name + element type) records declared columns on the existing `columnarConfig`. For a struct collection these are redundant with reflected fields; for a map collection they are the column set. The exact signature (generic `[T]` vs sample-value vs type token) is an implementation detail (see Open Questions).
- **Write split.** On write, the engine partitions the record map into declared keys (→ typed column slices, via the existing hybrid scalar/ref-bearing write) and the remaining keys (→ a deep-copied leftover map at the slot under the faithful default). Insert/duplicate/not-found behavior is shared with the struct path.
- **Read merge.** Reassembly builds `map[string]any` by copying the slot's leftover map and overlaying each declared column's slot value; the `map[string]any` view feeds the existing WHERE/GROUP BY/ORDER BY/projection/join code unchanged, and the typed-materialization path returns the merged map.
- **WHERE routing.** The candidate-row path consults a declared column's `ColumnStrategy` when the predicate targets a declared column; a predicate on a leftover field reports "no opinion" so the existing scan fall-back applies over the merged rows.

## Data Flow

Write (map record at id): resolve slot → for each declared column, hybrid-write its value into the typed slice (removing the key from the remainder) → deep-copy the remaining keys into `leftover[slot]` (faithful default) → mark slot live, update declared columns' strategies. Read (`Get`/query): id → slot → merged map = copy of `leftover[slot]` overlaid with each declared column's slot value. Query: candidate slots from a declared column's strategy when the equality predicate targets it, else full live-slot scan; then reassemble merged rows and apply the existing filter/group/order/projection. Delete/compaction: free or relocate the slot's declared-column cells and its `leftover[slot]` together.

## Error Handling & Failure Modes

- `WithColumnarStorage` on a `map[string]any` collection with no declared column → descriptive error (`REQ:mixed-mode-selection`).
- A written value for a declared column that cannot be stored as the declared element type → descriptive write error, nothing stored for that record (`REQ:declared-value-type-enforced`).
- `Insert` on an existing id → already-exists error; `Get` on an absent id → not-found — matching the Serialized engine (`REQ:inherits-fidelity-and-parity`).
- A query predicate on an undeclared field is not an error — it is the defined leftover-map scan fall-back (`REQ:where-on-declared-uses-strategy`).

## Testing Strategy

Table tests plus a `-race` interleaving, mirroring `columnar-storage`'s suite but over `map[string]any` data: mixed-mode selection (declared ok / no-declared error); declared field removed from leftover while undeclared fields remain; leftover holds exactly the undeclared fields (including empty); slot/leftover stay paired through delete, slot reuse, and compaction; read reconstructs the full record with no duplicate/dropped keys; WHERE on a declared column consults its strategy and matches Serialized; WHERE on a leftover field scans and matches Serialized; fidelity (declared + leftover ref-bearing values isolated after write) and behavioral parity with Serialized. `dalgo2memory` MUST remain at 100% statement coverage, and the mixed-mode engine MUST be clean under `go test -race`. A test-only external `ColumnStrategy` on a declared column proves the extension surface still works in mixed mode.

## Rehearse Integration

All ACs are exercisable through pure Go — `dalgo2memory` operations and queries over in-memory `map[string]any` collections, a parallel Serialized collection for equality comparison, a test-only `ColumnStrategy`, and a `-race` interleaving — so they map directly to table/`-race` tests (see `## Testing Strategy`). Per-AC Rehearse stub files are deferred to the Plan, where each AC becomes a concrete `*_test.go` case.

## Out of Scope

- **Inferring columns from observed data** — columns for a map-backed collection are explicitly declared; no automatic promotion of frequently-seen keys to columns.
- **Range/inequality/composite read-side acceleration** — inherited equality-only limit from `columnar-storage`; leftover-field and non-equality predicates scan.
- **Multi-predicate (`AND`/`OR`) `WHERE`** — unchanged from `columnar-storage`; only the single supported equality predicate is wired.
- **Struct-typed collections** — already covered by `columnar-storage`; this Feature only adds the map-backed path (declared options on a struct collection are accepted but redundant).
- **Per-collection two-phase locking / rollback, persistence** — unchanged; single global write lock, in-memory.

## Assumption Carryover

From the source Idea `dalgo2memory-storage-engines`:

- **Resolved (Should):** mixed-mode `map[string]any` storage — declared columns in typed slices + undeclared fields in a parallel `[]map[string]any` — is realized here, exactly as the Idea's column-set decision described.
- **Carried (Must):** the slot/tombstone/compaction model and single-global-lock concurrency from `columnar-storage` extend to the leftover slice — `REQ:shared-slot-across-columns-and-leftover` (validated under `-race`).
- **Carried (Must):** the exported `ColumnStrategy` seam keeps working for declared columns in mixed mode — `REQ:where-on-declared-uses-strategy`.
- **Carried (Should):** fidelity is faithful by default with the existing opt-out — `REQ:inherits-fidelity-and-parity`.

## Open Questions

- The exact declared-column option signature: generic `WithDeclaredColumn[T](name)` yielding `[]T`, a sample/zero-value form `WithDeclaredColumn(name, zero any)`, or a reflect.Type token — a Plan-time choice; all satisfy `REQ:declared-column-option`.
- Whether JSON-number normalization (declared `int` vs a value arriving as `float64` from a decoded map) is coerced or rejected — settled at implementation against the declared-type contract; `AC:declared-value-wrong-type-errors` pins only the clearly-incompatible (string-into-int) boundary.
- Duplicate declaration of the same column name: the intended resolution is last-declaration-wins, but the exact policy (last-wins vs. construction error) is a Plan-time decision and is not pinned by an AC here.

---
*This document follows the https://specscore.md/feature-specification*
