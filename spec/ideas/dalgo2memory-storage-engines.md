---
format: https://specscore.md/idea-specification
status: Specified
---

# Idea: dalgo2memory Pluggable Storage Engines

**Status:** Specified
**Date:** 2026-06-06
**Owner:** alex
**Promotes To:** columnar-mixed-mode-maps, columnar-storage, serialized-storage, storage-engine-seam
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we let dalgo2memory offer alternative in-memory storage engines behind a common seam — so arbitrary multi-field WHERE filtering reads measurably faster than today's JSON-bytes engine, while preserving the adapter's serialization-fidelity contract by default?

## Context

dalgo2memory currently stores every record as JSON bytes (collection -> id -> []byte) and json.Unmarshal's on every read/scan. DataTug analytics and larger in-memory datasets make multi-field WHERE scans the hot path. User wants a pluggable storage-engine seam with variants: type-copy rows, columnar typed slices, and per-column bitmap storage. Fidelity (ref-breaking + reject-non-serializable) must stay default-on, disableable per-schema or per-collection.

## Recommended Direction

The target API is:

```go
dalgo2memory.WithSchema(false,
    dalgo2memory.WithCollection[User]("users", nil,
        dalgo2memory.WithColumnarStorage(
            bitmap4dalgo2memory.WithBitmapColumn("role"),
        ),
    ),
)
```

This implies a **two-level seam**, both in core:

1. **Per-collection storage engine.** `WithCollection[T]` gains variadic options that select how that collection is stored (rather than introducing a new `Collection[T]` name). The current `map[string]map[string][]byte` JSON-bytes representation stays the **default engine** (no option needed), unchanged in behavior — it keeps the serialization-fidelity contract (ref-breaking via marshal/unmarshal, `DisallowUnknownFields` rejection) for free. `WithColumnarStorage(...)` selects a **columnar engine** that stores each column in its own typed slice — the layout that wins on scans and multi-field filtering.

2. **Per-column storage strategy inside the columnar engine.** `WithColumnarStorage` accepts per-column options. The default strategy for a column is a plain typed slice. Core defines and **exports a `ColumnStrategy` interface** so a column can be backed by something else — and that "something else" can live in a *separate package*. `bitmap4dalgo2memory.WithBitmapColumn("role")` is exactly that: an external package that returns a column option implementing core's `ColumnStrategy`, backing the `role` column with a bitmap index.

The dependency direction is the whole point: **core owns the columnar engine and the `ColumnStrategy` interface; the bitmap library is imported only by `bitmap4dalgo2memory`, never by core.** `dalgo2memory` stays foundational and dependency-free, while the bitmap index — which delivers the stated multi-field-WHERE speedup — ships out of core and composes in through the exported interface.

`ColumnStrategy` needs two capabilities so a bitmap column can both stay current and accelerate reads: a **write side** (record a column value as rows are inserted/updated/deleted) and a **read side** for equality predicates (given `column == value`, return the matching row-IDs, or "no opinion" → fall back to scanning that column). The query engine intersects/unions those row-ID sets across a multi-field `WHERE` before materializing rows.

Fidelity stays default-on and is relaxable only by an explicit per-schema or per-collection opt-out, so the test-double contract is never weakened silently. A type-copy ("decoded row") engine remains a possible future variant behind the same per-collection seam.

## Alternatives Considered

- **Bitmap index built into core.** The most direct route to the read-speedup metric, but rejected as a core deliverable: `dalgo2memory` is a foundational library and the owner does not want a bitmap (e.g. roaring) dependency in it. Designed for — via the exported `ColumnStrategy` interface — but shipped later as the out-of-core `bitmap4dalgo2memory` package.
- **Ship the speedup in the MVP (bitmap column strategy in the first cut).** Tempting, but it puts the hardest unknown last. Columnar storage with a stable per-row slot and a sound concurrency model is the foundation everything else (including bitmap) sits on; getting *that* right is the MVP's job. Speed comes after correctness + the seam.
- **Type-copy ("decoded row") engine as the MVP.** Store the deep-copied decoded value (JSON round-trip to break refs) so reads copy instead of unmarshal. Small and localized — but it does nothing for the stated multi-field-WHERE win (a full scan still touches every row) and sidesteps the concurrency model the owner wants ironed out. Kept as a documented future variant behind the same per-collection seam.
- **Replace the default engine outright (drop JSON bytes).** Rejected: the JSON round-trip is the adapter's serialization-fidelity boundary. Removing it weakens the test-double contract for every existing dalgo2memory user. The seam must add engines, not delete the faithful one.

## MVP Scope

A spike that ships **columnar storage (one typed slice per column) for a typed collection**, and uses it to iron out the **concurrency / multithreading model** — that is the hard, foundational problem worth solving first. Bitmap is *designed for* (the `ColumnStrategy` interface is exported) but *not built* in the MVP.

1. `WithCollection[T]` extended with variadic options, with the current JSON-bytes representation as the default engine, behavior-identical to today (existing tests stay green — the regression gate).
2. `WithColumnarStorage(...)`: store each column in its own typed slice, with a stable per-row index shared across all of a collection's columns. Deletes use **tombstones with later bulk compaction** (mark slot dead, reuse on insert, compact when the dead fraction crosses a threshold). Cover the full lifecycle — insert, update, delete, and read/scan — and make the existing query path (WHERE / projection / GROUP BY / joins / ORDER BY) correct against columnar storage. Correctness first; the speedup ships with the bitmap extension.
3. **Concurrency: keep the existing single global write lock** (`db.mu` RWMutex — one writer at a time, readers share). No deadlocks, no rollback. The MVP's concurrency job is therefore narrow but real: prove that columnar slot allocation, in-place updates, tombstones, and compaction are correct and race-free *under that lock*. Race-detector tests (`go test -race`) interleaving reads with writes/deletes/compaction are the gate. Per-collection two-phase locking with transactional rollback is explicitly deferred.
4. The exported **`ColumnStrategy` interface** (write side + equality read side) that `WithColumnarStorage` consumes, with the default typed-slice strategy as its first implementation — proving an out-of-core package like `bitmap4dalgo2memory` can later supply `WithBitmapColumn` without a core dependency.

Success: columnar collections pass the existing suite plus `-race`, the concurrency model is documented and defended by tests, and the `ColumnStrategy` seam is exercised by at least the default strategy.

## Not Doing (and Why)

- Per-collection two-phase locking + transactional rollback — deferred; the MVP keeps the single global write lock (no deadlocks, no rollback). Concurrent writes across different collections is a future concern, not an MVP goal
- Shipping the bitmap index in core — deferred to the out-of-core `bitmap4dalgo2memory` package to keep `dalgo2memory` dependency-free; the `ColumnStrategy` seam is designed to host it, but it is not built here
- Persistence/durability — engines stay in-memory; this is not a storage backend rewrite
- Replacing the default JSON-bytes engine — columnar is opt-in per collection via `WithColumnarStorage`; the faithful default stays
- Composite/range read-side acceleration in the `ColumnStrategy` interface — MVP covers equality predicates only
- Auto-selecting which columns get a non-default strategy — strategies are declared explicitly via column options, not inferred

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | Columnar slot allocation, updates, tombstones, and compaction are correct and race-free under the existing single global write lock (no deadlocks, no rollback). | Implement columnar storage; defend with `go test -race` interleaving reads against writes/deletes/compaction. Compaction reindexing under the write lock must not be observable to concurrent readers. |
| Must-be-true | Columnar storage can back the full existing query surface (WHERE / projection / GROUP BY / joins / ORDER BY / Get / Set) without changing observable behavior. | Run the full existing test suite against columnar collections; all pass unchanged. |
| Must-be-true | The exported `ColumnStrategy` interface (write side + equality read side) is sufficient for an *external* package to back a column (e.g. bitmap) without forking core or adding a core dependency. | Implement the default typed-slice strategy against the interface; sketch `bitmap4dalgo2memory.WithBitmapColumn` and confirm its import graph points only inward. |
| Should-be-true | Per-column slice deletes (tombstone vs. compaction) keep row-IDs stable enough that strategies and the query path stay correct and cheap. | Prototype both tombstone and compaction; measure correctness and cost under churn. |
| Should-be-true | Engine and per-column options compose cleanly through `Collection[T](name, ...)` / `WithColumnarStorage(...)` without a config explosion, and fidelity opt-out reads naturally alongside the current `WithSchema(allowUndefined, ...)` shape. | Sketch the API surface end to end and review for ergonomics. |
| Might-be-true | A type-copy engine would add further gains for point-Get-heavy workloads beyond columnar. | Defer — only profile after columnar lands and a workload shows residual read cost. |


## SpecScore Integration

- **New Features this would create:** a `storage-engine-seam` Feature (the internal interface + exported extension hooks + `jsonbytes` default). The bitmap index is a *separate out-of-core extension*, tracked as its own future Idea/repo, not a Feature of core. Columnar and type-copy engines are deferred candidate Features behind the same seam.
- **Existing Features affected:** the dalgo2memory query path that backs `query-column-projection`, `query-group-by-aggregation`, and `first-class-query-joins` — all must keep passing through the seam unchanged.
- **Dependencies:** builds on the existing `WithSchema`/`WithCollection` schema mechanism; no dependency on other in-flight Ideas. Core takes on **no** new third-party dependency.

## Open Questions

- What is the minimal exported shape of `ColumnStrategy` (write side + equality read side) that lets `bitmap4dalgo2memory` plug in without leaking core internals?
- Compaction trigger: what dead-slot fraction (or absolute count) should kick off bulk compaction, and does it run inline on the next write or as a separate maintenance call?
- Naming: the chosen direction extends `WithCollection[T]` with options rather than introducing the `Collection[T]` constructor from the original sketch — confirm this trade (less churn) over the shorter name at spec time.
