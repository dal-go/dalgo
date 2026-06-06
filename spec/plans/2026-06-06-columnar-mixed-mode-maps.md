# Plan: Mixed-mode columnar storage for map[string]any collections (dalgo2memory)

**Status:** Completed
**Source Feature:** columnar-mixed-mode-maps
**Date:** 2026-06-06
**Owner:** alex
**Supersedes:** —

## Summary

Decomposes the `columnar-mixed-mode-maps` Feature into four linear tasks that extend the existing `columnarEngine` to `map[string]any` collections: a map-backed selection path with declared-column options and type enforcement, the mixed storage layout (typed declared-column slices + a parallel leftover `[]map[string]any` sharing the slot vector), the read-merge and WHERE routing, and the inherited fidelity/parity guarantees. All nine acceptance criteria are covered by a task; none are deferred. Depends on the already-implemented `columnar-storage`.

## Approach

The columnar engine already has the typed-column slices, slot/free-list model, tombstone+compaction, strategies, and the WHERE candidate path; mixed mode adds a map-backed construction and a leftover store alongside them. Task 1 lifts the struct-only restriction: a `map[string]any` collection with at least one declared column builds columns from the declarations (no declaration → descriptive error), and a declared value of an incompatible type is a descriptive write error. Task 2 adds the storage split — declared keys into typed slices (removed from the record), the remainder into a `leftover []map[string]any` indexed by the same slot, kept paired through delete/slot-reuse/compaction. Task 3 reassembles reads by merging declared columns with the slot's leftover map (no duplicate/dropped key) and routes equality WHERE to a declared column's strategy or a leftover-map scan, matching the Serialized engine. Task 4 confirms fidelity (declared + leftover ref-bearing isolation) and behavioral parity. Order: 2 needs 1's construction; 3 needs 2's layout; 4 needs 2's write path.

## Tasks

### Task 1: Map-backed selection, declared-column option, and type enforcement

**Verifies:** columnar-mixed-mode-maps#ac:mixed-mode-requires-declared-column, columnar-mixed-mode-maps#ac:declared-value-wrong-type-errors
**Status:** done

Lift the struct-only restriction so `WithColumnarStorage` builds a columnar engine for a `map[string]any` collection from explicitly declared columns; selecting it with no declared column fails with a descriptive error. Add the declared-column `ColumnOption` (name + element type) and make a written declared value that cannot be stored as the declared type a descriptive write error that stores nothing for that record.

### Task 2: Mixed storage layout — typed declared columns + leftover map sharing the slot

**Verifies:** columnar-mixed-mode-maps#ac:declared-field-removed-from-leftover, columnar-mixed-mode-maps#ac:leftover-holds-undeclared-fields, columnar-mixed-mode-maps#ac:slot-stays-synced-through-delete-and-compaction
**Depends-On:** 1
**Status:** done

Store each declared column in its typed slice (removing the declared key from the record) and retain every undeclared field in a parallel `leftover []map[string]any` indexed by the same per-row slot. Wire the leftover store into slot allocation, tombstone delete, slot reuse, and compaction so a row's declared cells and its leftover map are always moved or freed together.

### Task 3: Read merge and WHERE routing

**Verifies:** columnar-mixed-mode-maps#ac:read-reconstructs-full-record, columnar-mixed-mode-maps#ac:where-on-declared-column-accelerated, columnar-mixed-mode-maps#ac:where-on-leftover-field-scans
**Depends-On:** 2
**Status:** done

Reassemble `Get`/query rows by overlaying each declared column's slot value onto a copy of the slot's leftover map, yielding the full `map[string]any` with each key exactly once. Route an equality `WHERE` on a declared column through that column's `ColumnStrategy` and an equality `WHERE` on a leftover field through the scan fall-back; both results identical to the Serialized engine.

### Task 4: Fidelity and behavioral parity

**Verifies:** columnar-mixed-mode-maps#ac:mixed-mode-parity-and-fidelity
**Depends-On:** 2
**Status:** done

Confirm declared columns inherit the hybrid write + fidelity opt-out, the leftover map's values are deep-copied under the faithful default (post-write caller mutation does not affect stored data), and `Set`/`Insert`/`Delete`/`Update` outcomes match the Serialized engine for the same `map[string]any` data.

## Open Questions

- The declared-column option signature and the float64/int coercion boundary are inherited Open Questions from the Feature; both are settled at implementation and do not change the task breakdown.

---
*This document follows the https://specscore.md/plan-specification*
