---
format: https://specscore.md/plan-specification
status: Draft
---
# Plan: Collection Generated Insert

**Status:** Approved
**Source Feature:** collection-generated-insert
**Date:** 2026-06-07
**Owner:** alexandertrakhimenok
**Supersedes:** —

## Summary

Implements the `collection-generated-insert` Feature (approach C): adds the bare `dal.Collection[T].Insert(ctx, ws, value, ...InsertOption) (*Key, error)` terminal for generated ids, makes `dalgo2memory` honor `InsertOption` by running id generation + collision-retry at the storage layer, guards against silent no-ops, and proves it end-to-end. Depends on the `typed-collection` Feature (the `Collection[T]` type).

## Approach

Adapter first, terminal second: wire `dalgo2memory` to honor `InsertOption` so generated `Insert` has a working backend, then add the typed terminal (with its default generator and compile-time generator rejection), then the loud-failure guard, then the end-to-end proof. The terminal's "returns a generated key" behavior depends on a honoring adapter, so the adapter task precedes the terminal task. Linear, no parallel branches.

## Tasks

### Task 1: dalgo2memory honors InsertOption (approach C)

**Verifies:** collection-generated-insert#ac:memory-generates-via-insert-option, collection-generated-insert#ac:generation-precedes-storage
**Depends-On:** —
**Status:** done

Extend `dalgo2memory`'s write path so that when `NewInsertOptions(opts...).IDGenerator()` is non-nil it runs `dal.InsertWithIdGenerator` (supplying an `exists` predicate over the engine and an `insert` callback), generating the id and retrying on collision BEFORE computing the storage key. On generator exhaustion return the generator's error with nothing persisted. This also makes raw `session.Insert(ctx, record, gen)` work, fixing the latent no-op.

### Task 2: bare Insert terminal + compile-time generator rejection

**Verifies:** collection-generated-insert#ac:generated-insert-returns-key, collection-generated-insert#ac:generated-insert-explicit-option, collection-generated-insert#ac:generator-not-accepted-elsewhere
**Depends-On:** 1
**Status:** done

Add `Insert(ctx, WriteSession, value T, opts ...dal.InsertOption) (*dal.Key, error)` to `dal.Collection[T]`: build a record with an incomplete key from the handle's `CollectionRef`, inject the default `WithRandomStringKey(dal.DefaultRandomStringIDLength, 5)` when `opts` is empty, forward to `WriteSession.Insert`, and return the assigned key. Confirm (signature + negative-compile example) that only the bare `Insert` accepts `InsertOption`, so generators cannot reach `InsertWithID`/`Get`/`Set`/`Update`/`Delete`.

### Task 3: loud-failure guard

**Verifies:** collection-generated-insert#ac:loud-failure-on-nonhonoring-adapter
**Depends-On:** 2
**Status:** done

After the underlying insert returns nil error, have `Insert` assert the record's key id is non-nil/non-empty and otherwise return a descriptive error (e.g. an exported sentinel), so generated `Insert` fails loudly on an adapter that ignores `InsertOption` instead of reporting a `<nil>`-id success. Test with a stub `WriteSession` that drops the option.

### Task 4: end-to-end proof

**Verifies:** collection-generated-insert#ac:e2e-generated-insert-roundtrip
**Depends-On:** 3
**Status:** pending

Add a `dalgo2memory`-specific test (NOT the shared gomock-driven `TestDalgoDB`) exercising generated `Insert` with both the zero-option default and an explicit `WithRandomStringKey`, asserting a complete assigned key is returned and the record round-trips via `Get`; confirm the existing shared `TestDalgoDB` suite stays green.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
