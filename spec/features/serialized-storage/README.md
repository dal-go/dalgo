---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Serialized storage engine (dalgo2memory default)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/serialized-storage?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/serialized-storage?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/serialized-storage?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/serialized-storage?op=request-change) |
**Status:** Stable
**Date:** 2026-06-06
**Owner:** alex
**Source Ideas:** dalgo2memory-storage-engines
**Supersedes:** —
**Grade:** A

## Summary

Formalizes `dalgo2memory`'s existing storage behavior as the **Serialized** engine: each record is kept as serialized JSON bytes per collection, and reads decode those bytes. This engine is the seam's default and the implementation behind `WithSerializedStorage()`. Its defining property is full serialization fidelity — stored data shares no references with caller values, non-serializable values are rejected, and unknown fields are rejected for schema-typed collections.

## Problem

The behavior `dalgo2memory` already has — store each record as JSON bytes, decode on read — is currently spread across `session.save`/`Get`/`UpdateRecord` and the query paths with no name and no stated contract. Once the storage-engine seam exists, that behavior must become a concrete engine with an explicit, testable contract so that (a) it can be selected/defaulted through the seam, (b) other engines (Columnar) have a reference contract to match for the parts that must stay identical, and (c) the serialization-fidelity guarantees that make `dalgo2memory` a faithful test double are pinned down rather than incidental.

## Behavior

### Representation and fidelity

#### REQ: json-byte-representation

The Serialized engine MUST store each record as a serialized byte buffer keyed by record id within its collection, and reconstruct records by decoding those bytes. The serialization format for this MVP is JSON (`encoding/json`); the engine name is format-agnostic so the serializer may change later without renaming.

#### REQ: ref-breaking-fidelity

Because records are stored as bytes and decoded afresh on every read, a stored record MUST share no references with the value supplied to a write or returned by a prior read. Mutating a caller's struct after writing it, or mutating a value obtained from one read, MUST NOT affect stored data or any subsequent read.

#### REQ: reject-non-serializable

A write whose data cannot be serialized (e.g. it contains a value the serializer cannot encode) MUST fail with a descriptive error, set that error on the record, and MUST NOT store partial or corrupt data for that id.

#### REQ: unknown-field-rejection

For a collection registered with a record type, the Serialized engine MUST reject a write or update whose serialized data contains fields not defined on that type, returning a descriptive error that names the collection. A schemaless collection (no registered type) MUST perform no unknown-field check.

### Write semantics

#### REQ: insert-vs-set

`Set` MUST create-or-overwrite the record at its id. `Insert` MUST fail with a descriptive "already exists" error when a record with that id is already present, and MUST NOT overwrite it. Both MUST serialize and (for schema-typed collections) run the unknown-field check before storing, so a rejected write leaves storage unchanged.

### Read and query

#### REQ: get-decodes-or-not-found

`Get`/`GetMulti` on a present id MUST decode the stored bytes into the caller-provided record's data. `Get` on an absent id MUST return a not-found error and set it on the record. `Exists` MUST report presence/absence by id without decoding.

#### REQ: query-row-views

For query execution the Serialized engine MUST expose each stored row both as a decoded `map[string]any` field view (consumed by WHERE/GROUP BY/ORDER BY/projection) and as a typed materialization into the query's `IntoRecord`/factory target, in both cases by decoding the stored bytes. Distinct decoded rows MUST NOT share references.

### Update

#### REQ: update-read-modify-write

`Update`/`UpdateRecord` MUST read the stored record, apply the given field updates to its decoded data, re-run the unknown-field check for schema-typed collections, and store the re-serialized result. `Update` on an absent id MUST return a not-found error and store nothing.

### Role within the seam

#### REQ: default-and-schemaless

The Serialized engine MUST be the concrete engine the seam resolves to for the default (unselected) case and for an explicit `WithSerializedStorage()`, and MUST support both schema-registered and schemaless collections. The engine is always fully faithful: fidelity is inherent to the byte representation and is never disabled here — any fidelity opt-out belongs to the Columnar engine, not this one.

## Acceptance Criteria

### AC: stored-as-decodable-bytes (verifies REQ:json-byte-representation)

**Given** a Serialized-backed collection
**When** a record is `Set` and then `Get`
**Then** the retrieved data equals the written data, having been reconstructed by decoding the stored serialized bytes (the stored form is a byte buffer, not the original value).

### AC: mutation-after-write-isolated (verifies REQ:ref-breaking-fidelity)

**Given** a record written via `Set` whose data includes a nested struct or slice
**When** the caller mutates that nested field after the `Set` returns, then the record is `Get` into a fresh target
**Then** the retrieved data reflects the value as it was at write time, unaffected by the post-write mutation.

### AC: two-reads-independent (verifies REQ:ref-breaking-fidelity)

**Given** one stored record
**When** it is read into two separate targets and one target's nested field is mutated
**Then** the other target is unchanged.

### AC: non-serializable-write-errors (verifies REQ:reject-non-serializable)

**Given** a Serialized-backed collection
**When** a record whose data contains a non-serializable value is written
**Then** the write returns a descriptive error, the error is set on the record, and no entry for that id exists afterward.

### AC: unknown-field-rejected-when-typed (verifies REQ:unknown-field-rejection)

**Given** a collection registered with record type `T`
**When** a write carries a field not defined on `T`
**Then** it is rejected with a descriptive error naming the collection; the same data written to a schemaless collection succeeds.

### AC: insert-duplicate-errors (verifies REQ:insert-vs-set)

**Given** a record already present at an id
**When** `Insert` is called for that id
**Then** it returns an "already exists" error and the existing record is unchanged, whereas `Set` for that id overwrites successfully.

### AC: get-absent-not-found (verifies REQ:get-decodes-or-not-found)

**Given** a Serialized-backed collection with no record at id `x`
**When** `Get` is called for `x`
**Then** it returns a not-found error set on the record, while `Exists` for `x` reports `false`.

### AC: query-decodes-rows (verifies REQ:query-row-views)

**Given** several stored records
**When** a structured query with an equality filter runs and also materializes into a typed `IntoRecord` target
**Then** the engine yields decoded `map[string]any` rows for filtering and decoded typed records for results, and two result records do not share references.

### AC: update-applies-and-revalidates (verifies REQ:update-read-modify-write)

**Given** a stored record in a schema-typed collection
**When** `Update` applies a field change defined on the type, and separately an update introducing an undefined field is attempted
**Then** the defined-field update is read-modified-written and persisted, the undefined-field update is rejected with a descriptive error, and `Update` on an absent id returns not-found and stores nothing.

### AC: serialized-is-default-and-schemaless-capable (verifies REQ:default-and-schemaless)

**Given** a database with an unregistered collection and another collection selected via `WithSerializedStorage()`
**When** records are written to and read from both
**Then** both operate through the Serialized engine and succeed, demonstrating support for schemaless and schema-typed collections respectively.

## Architecture & Components

- **`serializedEngine` (new, internal).** Implements the `storageEngine` interface from the seam Feature. Holds the per-collection `map[id][]byte`. Encapsulates today's `json.Marshal` (write), `json.Unmarshal` (read/materialize), `checkUnknownFields` (schema-typed validation), the insert-duplicate guard, and the not-found error — i.e. the logic currently inline in `session.save`/`Get`/`UpdateRecord` and the query loops moves into this engine unchanged in behavior.
- **Decoded views for queries.** The engine's row enumeration unmarshals each stored buffer into a `map[string]any` for the existing filter/group/order/projection code, and offers a materialize-into-target that unmarshals into the query's `IntoRecord`/factory value — replacing the current direct `json.Unmarshal(row.raw, …)` calls with engine-owned ones.
- **Schema awareness.** When the collection has a registered record factory, writes/updates run the unknown-field check; without one, they don't — preserving today's schemaless behavior.

## Data Flow

Write: caller data → `json.Marshal` → (schema-typed: `checkUnknownFields`) → store bytes at id (Insert first checks absence). Read: bytes at id → `json.Unmarshal` into caller target (or not-found). Query: for each id, bytes → `json.Unmarshal` → `map[string]any` view (+ typed materialize on demand). Update: bytes → decode → apply field updates → re-validate (schema-typed) → re-`json.Marshal` → store.

## Error Handling & Failure Modes

- Non-serializable write → marshal error, set on record, nothing stored (`REQ:reject-non-serializable`).
- Unknown field on a schema-typed collection → descriptive error naming the collection; nothing stored (`REQ:unknown-field-rejection`).
- `Insert` on an existing id → "already exists" error; existing record untouched (`REQ:insert-vs-set`).
- `Get`/`Update` on an absent id → not-found error; for `Get` it is set on the record; `Update` stores nothing (`REQ:get-decodes-or-not-found`, `REQ:update-read-modify-write`).

## Testing Strategy

Table tests covering: byte round-trip equality; post-write mutation isolation and two-read independence (fidelity); non-serializable rejection with no residual entry; unknown-field rejection on typed vs acceptance on schemaless; insert-duplicate vs set-overwrite; get/exists on absent id; equality-query decoding with typed materialization and non-shared results; update read-modify-write with re-validation and absent-id not-found. These largely formalize existing `dalgo2memory` tests against the named engine. `dalgo2memory` MUST remain at 100% statement coverage, and a compile-time assertion `var _ storageEngine = (*serializedEngine)(nil)` guards the interface conformance.

## Rehearse Integration

All ACs are exercisable through pure Go — `dalgo2memory` operations over in-memory collections, including a deliberately non-serializable value and a schema-typed-vs-schemaless pair — so they map directly to table tests (see `## Testing Strategy`). Per-AC Rehearse stub files are deferred to the Plan, where each AC becomes a concrete `*_test.go` case.

## Out of Scope

- The storage-engine interface, selection plumbing, and default routing — owned by the `storage-engine-seam` Feature; here the engine only implements that interface and states its behavioral contract.
- The Columnar engine, typed slices, slot/tombstone/compaction, `ColumnStrategy`, and any fidelity opt-out — owned by the `columnar-storage` Feature. The Serialized engine is always faithful.
- Any change to the concurrency model (single global write lock retained) or to query-predicate support (still single `Equal`); this Feature does not broaden either.
- Persistence/durability — storage remains in-memory bytes.

## Assumption Carryover

From the source Idea `dalgo2memory-storage-engines`:

- **Carried (Must):** fidelity (ref-breaking + reject-non-serializable + unknown-field rejection) is preserved by default — embodied by `REQ:ref-breaking-fidelity`, `REQ:reject-non-serializable`, `REQ:unknown-field-rejection` and their ACs.
- **Carried (Should):** the default engine has the name **Serialized** and is explicitly selectable — embodied by `REQ:default-and-schemaless`.
- **Resolved:** "disable fidelity per-schema/per-collection" does not apply to the Serialized engine (fidelity is inherent to its byte representation); the opt-out is a Columnar concern, recorded here as out of scope.
- **Deferred:** columnar storage, concurrency model, and the bitmap extension — other Features.

## Open Questions

- Whether the engine should expose the serialized bytes (or a content hash) for future diffing/debugging, or keep them fully internal — not needed for the MVP contract.

---
*This document follows the https://specscore.md/feature-specification*
