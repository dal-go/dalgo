---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Pluggable per-collection storage engine seam (dalgo2memory)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/storage-engine-seam?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/storage-engine-seam?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/storage-engine-seam?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/storage-engine-seam?op=request-change) |
**Status:** Stable
**Date:** 2026-06-06
**Owner:** alex
**Source Ideas:** dalgo2memory-storage-engines
**Supersedes:** —
**Grade:** A

## Summary

Introduces an internal storage-engine seam in `dalgo2memory` so each collection can choose how its records are stored, and adds the per-collection selection plumbing on `WithCollection[T]`. This Feature delivers only the seam (the engine interface, the selection option, and the routing of all operations through the selected engine) and the **Serialized** default; the Serialized engine's own behavioral contract and the Columnar engine are specified in separate Features.

## Problem

Today `dalgo2memory` hard-codes a single storage representation: `database.collections` is a `map[string]map[string][]byte`, and every `Get`/`Set`/`Insert`/`Update`/`Delete` and query path reads or writes those JSON bytes directly. There is no place to plug in an alternative representation (columnar typed slices, an indexed column, etc.) without editing every operation. Before any alternative engine can exist, the adapter needs a single seam that (a) abstracts per-collection storage behind an interface, (b) routes all operations through the engine selected for a collection, and (c) lets a caller pick that engine per collection — while leaving today's behavior exactly as-is for callers who pick nothing.

## Behavior

### The storage-engine interface

#### REQ: engine-interface

`dalgo2memory` MUST define an internal storage-engine interface that owns, for one collection, the operations the adapter performs on stored records: store a record by id (with an overwrite/insert distinction), test existence by id, load a record by id into a caller-provided `dal.Record`, delete by id, apply field updates by id, and enumerate the collection's rows for query execution. The enumeration contract MUST expose, per row, both a decoded field view (`map[string]any`, as the WHERE/GROUP BY/ORDER BY/projection code consumes today) and the ability to materialize the row into a caller-provided typed target. The interface MUST be representation-neutral: it MUST NOT assume any particular on-disk/in-memory representation (e.g. JSON bytes). In particular, the update member MUST receive decoded field updates (name→value), not marshaled bytes, and each engine MUST own any re-validation it needs (such as unknown-field rejection) against its own representation.

#### REQ: route-through-engine

All `database` and `session` operations (`Exists`, `Get`/`GetMulti`, `Set`/`SetMulti`, `Insert`/`InsertMulti`, `Delete`/`DeleteMulti`, `Update`/`UpdateRecord`/`UpdateMulti`, and both query-execution paths) MUST operate through the storage-engine interface for the target collection rather than reading or writing a byte map directly. The adapter MUST hold no storage representation outside the engine instances.

### Selecting an engine per collection

#### REQ: withcollection-options

`WithCollection[T]` MUST accept a trailing variadic `...CollectionOption` parameter that may carry a storage-engine selection, and a new exported `CollectionOption` type MUST be introduced for that purpose. Existing call sites that pass no option (`WithCollection[T](name, newRecord)`) MUST continue to compile and behave unchanged. A storage-engine selection carried by a `CollectionOption` takes effect only when the collection definition is registered with the database — today that registration channel is `WithSchema` (which already consumes `WithCollection[T]` definitions). The option records the chosen engine on the collection definition; it has no effect on a collection that is never registered.

#### REQ: serialized-default

The **Serialized** engine is the default and the only engine reachable without registration. A collection that is never registered (no collection definition), or whose registered definition carries no storage-engine option, MUST be backed by the Serialized engine, and its observable behavior MUST be identical to the adapter's behavior before this Feature. An explicit `WithSerializedStorage()` `CollectionOption` MUST also be provided that selects the same Serialized engine, so the default can be stated explicitly on a registered collection.

#### REQ: per-collection-engine

Engine selection MUST be per collection: to back a collection with a non-default engine, its definition (carrying the storage-engine `CollectionOption`) MUST be registered with the database via the schema registration channel. A single `database` MUST be able to register multiple collections backed by different engines simultaneously, each routing through its own engine instance, while any unregistered collection in the same database continues to use Serialized.

### Preserving today's behavior

#### REQ: behavior-preserved

Refactoring the existing operations behind the seam MUST NOT change any observable behavior of the default (Serialized) path: the full existing `dalgo2memory` test suite MUST pass unchanged, and `dalgo2memory` MUST remain at 100% statement coverage.

## Acceptance Criteria

### AC: engine-interface-defined (verifies REQ:engine-interface)

**Given** the `dalgo2memory` package
**When** its source is inspected
**Then** an internal storage-engine interface exists declaring store-by-id (with insert/overwrite distinction), exists-by-id, load-by-id-into-record, delete-by-id, update-by-id, and row-enumeration members, and the enumeration member yields each row as a `map[string]any` field view plus a typed-materialization path with no parameter or return type that is a serialized byte representation.

### AC: operations-succeed-through-engine (verifies REQ:route-through-engine)

**Given** a `database` with one collection
**When** a `Set`, then a `Get`, a `Delete`, an `Update`, and a structured query whose filter is a single `Equal` comparison (the predicate the adapter currently supports) are executed
**Then** every operation succeeds and returns the same results as before this Feature, with each result produced via the collection's storage-engine instance.

### AC: no-byte-map-on-database (verifies REQ:route-through-engine)

**Given** the refactored `dalgo2memory` package
**When** the `database` type is inspected
**Then** no `map[string]map[string][]byte` (or other direct byte-storage) field remains on `database`; stored records live solely inside per-collection storage-engine instances.

### AC: withcollection-options-backward-compatible (verifies REQ:withcollection-options)

**Given** the existing signature `WithCollection[T any](name string, newRecord func() *T)` extended to `WithCollection[T any](name string, newRecord func() *T, opts ...CollectionOption)`, and an existing call `WithCollection[T]("users", newRecord)` that passes no option
**When** the package is compiled and that collection is used
**Then** the call compiles unchanged with `newRecord` keeping its `func() *T` type, behaves exactly as before, and an exported `CollectionOption` type is available to pass as a trailing argument.

### AC: default-is-serialized (verifies REQ:serialized-default)

**Given** a registered collection with no storage-engine option, and an otherwise identical registered collection created with `WithSerializedStorage()`
**When** the same sequence of `Set`/`Get`/equality-filtered query operations runs against each
**Then** both produce identical results, and both match the adapter's pre-Feature behavior.

### AC: unregistered-collection-is-serialized (verifies REQ:serialized-default)

**Given** a `database` and a collection that was never registered through a schema
**When** records are written to and read from it
**Then** the operations succeed using the Serialized engine, exactly as before this Feature.

### AC: mixed-engines-in-one-db (verifies REQ:per-collection-engine)

**Given** a `database` registering collection `a` with `WithSerializedStorage()` and collection `b` with a second engine option
**When** records are written to both
**Then** each collection stores and retrieves its records through its own engine instance, and operations on one collection do not affect the other.

### AC: existing-suite-green (verifies REQ:behavior-preserved)

**Given** the refactor that moves operations behind the seam
**When** the existing `dalgo2memory` test suite and coverage run
**Then** all tests pass unchanged and statement coverage remains 100%.

## Architecture & Components

- **`storageEngine` interface (new, internal).** Declares the per-collection operations listed in `REQ:engine-interface`. Row enumeration returns rows that expose a `map[string]any` view (for the existing WHERE/GROUP BY/ORDER BY/projection logic) and a `materialize(target any) error` capability (replacing today's `json.Unmarshal(row.raw, data)` calls), so no caller depends on a byte representation.
- **`database` storage field (changed).** `collections map[string]map[string][]byte` is replaced by a per-collection engine registry (e.g. `map[string]storageEngine`), created lazily on first write or eagerly from registered collection definitions. The single global `db.mu` RWMutex is retained unchanged.
- **`CollectionOption` (new, exported) + `WithSerializedStorage()` (new, exported).** `WithCollection[T]` gains `opts ...CollectionOption`; an option records which engine constructor backs the collection. The engine choice is carried on `collectionDef` and propagated into the database when the schema is applied (and, for the default, used whenever a collection has no recorded choice).
- **`session` methods (changed).** Each method resolves the target collection's engine and delegates, rather than touching a byte map. The Serialized engine encapsulates today's `json.Marshal`/`json.Unmarshal`/`checkUnknownFields` logic (its contract is specified in the Serialized-storage Feature).

## Data Flow

`WithCollection[T](name, newRecord, opts...)` records an engine choice (default: Serialized) on the collection definition → `WithSchema` propagates choices into the `database` → on each operation, the `session` looks up the collection's `storageEngine` (constructing the default Serialized engine if none was registered) and delegates → query execution iterates the engine's rows, using the `map[string]any` view for filtering/grouping/ordering/projection and `materialize` for typed `IntoRecord`/factory results.

## Error Handling & Failure Modes

- An operation on a collection with no registered engine choice does not error — it uses the Serialized default (`REQ:serialized-default`).
- The seam introduces no new error surface for the Serialized path; existing errors (not-found, duplicate-on-insert, unknown-field rejection) continue to originate from the engine and are unchanged for the default.
- Engine-specific preconditions (e.g. an engine that requires a schema) are the concern of that engine's own Feature, not the seam; the seam only routes.

## Testing Strategy

Table tests proving: the default and `WithSerializedStorage()` paths are observably identical and match pre-Feature behavior; an unregistered collection uses Serialized; two collections in one database can use different engines independently; and the existing suite passes unchanged behind the seam. A compile-time interface assertion (`var _ storageEngine = ...`) guards the Serialized engine against the interface. `dalgo2memory` MUST remain at 100% statement coverage, exercising the default-resolution branch and the per-collection routing.

## Rehearse Integration

All ACs are exercisable through pure Go — package compilation, source-level interface presence, and `dalgo2memory` operations over in-memory collections — so they map directly to table tests (see `## Testing Strategy`). Per-AC Rehearse stub files are deferred to the Plan, where each AC becomes a concrete `*_test.go` case; the rehearsal surface is the Go test suite.

## Out of Scope

- The **Serialized** engine's own behavioral contract (JSON representation, ref-breaking fidelity, `DisallowUnknownFields` rejection) — specified in the `serialized-storage` Feature; here it is only the default an unselected collection routes to.
- The **Columnar** engine and its `WithColumnarStorage(...)` option, typed slices, slot/tombstone/compaction model, and `ColumnStrategy` seam — specified in the `columnar-storage` Feature.
- Any change to the concurrency model: the single global write lock is retained; per-collection locking, two-phase locking, and rollback are out of scope (deferred per the source Idea).
- Out-of-core extension packages (e.g. `bitmap4dalgo2memory`) — they attach at the Columnar engine's `ColumnStrategy` seam, not at this engine seam.
- Persistence/durability — engines remain in-memory.

## Assumption Carryover

From the source Idea `dalgo2memory-storage-engines`:

- **Carried (Must):** the engine seam can host an alternative storage path without changing observable behavior of the existing query/Get/Set surface — validated by `AC:existing-suite-green` and `AC:default-is-serialized`.
- **Carried (Should):** engine selection composes cleanly through `WithCollection[T]` options without a config explosion — embodied by `REQ:withcollection-options` and validated by `AC:withcollection-options-backward-compatible`.
- **Deferred:** columnar concurrency correctness, the `ColumnStrategy` extension contract, tombstone/compaction, and the bitmap extension — owned by the `columnar-storage` Feature, not this one.
- **Resolved:** the default engine has a name — **Serialized** — and is selectable explicitly via `WithSerializedStorage()`.

## Open Questions

- The exact method set and signatures of the `storageEngine` interface (e.g. whether `materialize` takes `any` or a typed callback) — an implementation detail for the Plan.
- Whether engine instances are created eagerly when a schema is applied or lazily on first write to a collection — a Plan-time choice; both satisfy the ACs.

---
*This document follows the https://specscore.md/feature-specification*
