# Plan: Serialized storage engine (dalgo2memory default)

**Status:** Implemented
**Source Feature:** serialized-storage
**Date:** 2026-06-06
**Owner:** alex
**Supersedes:** —

## Summary

Decomposes the `serialized-storage` Feature into four linear tasks that formalize and pin the contract of the `serializedEngine` extracted by the `storage-engine-seam` plan: the byte representation and engine role, the serialization-fidelity guarantees, the schema-aware write semantics, and the read/query/update semantics. All ten acceptance criteria are covered by a task; none are deferred. This is the second of three coordinated plans and depends on the seam plan being implemented first.

## Approach

The Serialized engine's behavior already exists in the codebase and is extracted behind the seam interface by the `storage-engine-seam` plan's Task 3; this plan gives that engine an explicit, tested contract. Task 1 names the `serializedEngine` type, adds the interface conformance assertion, and pins the byte-representation and default/schemaless role. Task 2 locks the fidelity guarantees (ref-breaking on write and across reads, rejection of non-serializable values). Task 3 covers the schema-aware write rules (unknown-field rejection on typed vs schemaless, insert-duplicate vs set). Task 4 covers reads, queries, and updates (not-found, decoded views with non-shared results, read-modify-write with re-validation). The tasks are independent contract slices but ordered representation → fidelity → writes → reads for a natural review flow.

## Tasks

### Task 1: serializedEngine type, byte representation, and default/schemaless role

**Verifies:** serialized-storage#ac:stored-as-decodable-bytes, serialized-storage#ac:serialized-is-default-and-schemaless-capable
**Status:** done
**Notes:** Builds on the seam plan's Task 3, which extracts the engine.

Formalize the `serializedEngine` implementing `storageEngine`, with a compile-time `var _ storageEngine = (*serializedEngine)(nil)` assertion, storing each record as a serialized (JSON) byte buffer keyed by id and reconstructing by decoding. Confirm it is the engine resolved for the default/`WithSerializedStorage()` selection and that it supports both schema-typed and schemaless collections.

### Task 2: Serialization-fidelity guarantees

**Verifies:** serialized-storage#ac:mutation-after-write-isolated, serialized-storage#ac:two-reads-independent, serialized-storage#ac:non-serializable-write-errors
**Depends-On:** 1
**Status:** done

Pin the fidelity contract: because records are stored as bytes and decoded afresh, mutating a caller value after a write does not affect stored data, two reads of one record are mutually independent, and a write of a non-serializable value fails with a descriptive error set on the record while storing nothing for that id.

### Task 3: Schema-aware write semantics

**Verifies:** serialized-storage#ac:unknown-field-rejected-when-typed, serialized-storage#ac:insert-duplicate-errors
**Depends-On:** 1
**Status:** done

Lock the write rules: for a schema-typed collection a write/update carrying a field undefined on the type is rejected with a descriptive error naming the collection (no check for schemaless collections), `Insert` on an existing id returns an "already exists" error without overwriting, and `Set` overwrites; both validate before storing so a rejected write leaves storage unchanged.

### Task 4: Read, query, and update semantics

**Verifies:** serialized-storage#ac:get-absent-not-found, serialized-storage#ac:query-decodes-rows, serialized-storage#ac:update-applies-and-revalidates
**Depends-On:** 1
**Status:** done

Cover reads and updates: `Get` on an absent id returns a not-found error set on the record while `Exists` reports false; query execution exposes decoded `map[string]any` views and typed materializations with no shared references between result records; and `Update` reads-modifies-writes the decoded data, re-runs the unknown-field check for typed collections, and returns not-found (storing nothing) on an absent id.

## Open Questions

- None at this time.

---
*This document follows the https://specscore.md/plan-specification*
