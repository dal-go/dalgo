# Plan: Pluggable per-collection storage engine seam (dalgo2memory)

**Status:** Implemented
**Source Feature:** storage-engine-seam
**Date:** 2026-06-06
**Owner:** alex
**Supersedes:** —

## Summary

Decomposes the `storage-engine-seam` Feature into four linear tasks: define the representation-neutral `storageEngine` interface, add the `CollectionOption`/`WithSerializedStorage()` selection plumbing on `WithCollection[T]`, route every operation through a per-collection engine registry (extracting today's JSON logic into a default Serialized engine), and prove per-collection selection plus full behavior preservation. All eight acceptance criteria are covered by a task; none are deferred. This is the first of three coordinated plans (`storage-engine-seam` → `serialized-storage` → `columnar-storage`).

## Approach

The seam is an interface plus a refactor, so the order is define-the-contract, add-the-selection, route-everything-through-it, then prove-it. Task 1 declares the internal `storageEngine` interface with no byte assumption. Task 2 adds the exported `CollectionOption` and `WithSerializedStorage()` and threads an engine choice onto the collection definition via `WithSchema`, keeping existing 2-arg `WithCollection[T]` calls compiling. Task 3 is the core refactor: replace `database.collections map[string]map[string][]byte` with a per-collection engine registry, move the existing `json.Marshal`/`json.Unmarshal`/`checkUnknownFields`/insert-guard/not-found logic into a default Serialized engine implementation (whose *formal contract* is owned by the `serialized-storage` plan), and make every `database`/`session` operation delegate — defaulting unregistered collections to Serialized. Task 4 proves a single database can host different engines per collection and that the whole existing suite still passes at 100% coverage. Tasks are strictly ordered: 2 needs 1's interface, 3 needs both, 4 verifies 3.

## Tasks

### Task 1: Define the storageEngine interface

**Verifies:** storage-engine-seam#ac:engine-interface-defined
**Status:** complete

Declare an internal `storageEngine` interface owning per-collection store-by-id (insert/overwrite), exists-by-id, load-by-id-into-record, delete-by-id, update-by-id (decoded field updates, not bytes), and row enumeration. Enumeration yields each row as a `map[string]any` field view plus a typed-materialization path, with no parameter or return type that is a serialized byte representation.

### Task 2: CollectionOption plumbing and WithSerializedStorage()

**Verifies:** storage-engine-seam#ac:withcollection-options-backward-compatible
**Depends-On:** 1
**Status:** complete

Introduce an exported `CollectionOption` type and `WithSerializedStorage()` option, and extend `WithCollection[T]` to `WithCollection[T](name, newRecord, opts ...CollectionOption)` so existing 2-arg calls compile unchanged with `newRecord` keeping its `func() *T` type. Record the selected engine on the collection definition so `WithSchema` carries it into the database.

### Task 3: Route all operations through a per-collection engine registry

**Verifies:** storage-engine-seam#ac:operations-succeed-through-engine, storage-engine-seam#ac:no-byte-map-on-database, storage-engine-seam#ac:default-is-serialized, storage-engine-seam#ac:unregistered-collection-is-serialized
**Depends-On:** 2
**Status:** complete

Replace `database.collections map[string]map[string][]byte` with a per-collection engine registry, extracting today's JSON marshal/unmarshal/`checkUnknownFields`/insert-guard/not-found logic into a default `serializedEngine` that implements the interface. Make every `database`/`session` operation (`Exists`, `Get`/`GetMulti`, `Set`/`SetMulti`, `Insert`/`InsertMulti`, `Delete`/`DeleteMulti`, `Update`/`UpdateRecord`/`UpdateMulti`, both query paths) delegate to the target collection's engine, resolving the Serialized default for any unregistered or option-less collection so observable behavior is unchanged.

### Task 4: Per-collection engine selection and behavior preservation

**Verifies:** storage-engine-seam#ac:mixed-engines-in-one-db, storage-engine-seam#ac:existing-suite-green
**Depends-On:** 3
**Status:** complete

Support distinct engines per collection within one `database`, each routing through its own engine instance with no cross-collection interference, and confirm the default and `WithSerializedStorage()` paths are observably identical to pre-Feature behavior. The full existing `dalgo2memory` test suite passes unchanged and statement coverage remains 100%.

## Open Questions

- Whether engine instances are constructed eagerly when the schema is applied or lazily on first write — both satisfy the ACs; settled during implementation.

---
*This document follows the https://specscore.md/plan-specification*
