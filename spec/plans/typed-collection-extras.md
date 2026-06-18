---
format: https://specscore.md/plan-specification
status: Draft
---
# Plan: Typed Collection Extras

**Status:** Implemented
**Source Feature:** typed-collection-extras
**Date:** 2026-06-07
**Owner:** alexandertrakhimenok
**Supersedes:** —

## Summary

Implements the `typed-collection-extras` Feature: the opt-in `dal.ManyInserter[K, T]` batch-insert interface (with dal-native `Item[K, T]`) plus the `Count`/`Exists`/`First` read conveniences on `dal.Collection[K, T]`, all additive over existing `dal` interfaces and cycle-free. Depends on the `typed-collection` Feature.

## Approach

Each terminal is independent (it adds one method/interface on the existing `Collection[K, T]` and is tested against `dalgo2memory`), so the tasks are ordered for review convenience rather than hard dependency: batch first, then the three read conveniences. `Count`/`First` reuse the same `StructuredQuery` plumbing as `All` (from `typed-collection`); `Exists` is a key-only probe; `InsertMany` builds complete keys from `Item.ID`. Linear, no parallel branches.

## Tasks

### Task 1: Item[K, T] type and ManyInserter[K, T] batch insert

**Verifies:** typed-collection-extras#ac:item-type-no-record-import, typed-collection-extras#ac:insert-many-roundtrips
**Depends-On:** —
**Status:** done

Define `dal.Item[K comparable, T any]{ ID K; Value T }` (no `record` import) and the `dal.ManyInserter[K, T]` interface; implement `InsertMany(ctx, WriteSession, items ...Item[K, T]) ([]*dal.Key, error)` on the concrete `Collection[K, T]` by building `[]dal.Record` from each item's complete key (`Item.ID` + the handle's `CollectionRef`) and delegating to the session's `MultiInserter` (falling back to per-item `Inserter.Insert`), returning keys in input order.

### Task 2: Count terminal

**Verifies:** typed-collection-extras#ac:count-returns-total, typed-collection-extras#ac:count-unsupported
**Depends-On:** —
**Status:** done

Implement `Count(ctx, ReadSession) (int, error)` building a count aggregation `StructuredQuery` over the handle's `CollectionRef` and running it via the `ReadSession` query executor; surface `dal.ErrNotSupported` from incapable backends rather than a silent `0`.

### Task 3: Exists terminal

**Verifies:** typed-collection-extras#ac:exists-true-false, typed-collection-extras#ac:exists-error-passthrough
**Depends-On:** —
**Status:** done

Implement `Exists(ctx, ReadSession, id K) (bool, error)` as a key-only probe: build the key from the typed `id K` + the handle's `CollectionRef`, call the session getter, and map a not-found error to `(false, nil)`, any other error to `(false, err)`, and success to `(true, nil)`.

### Task 4: First terminal

**Verifies:** typed-collection-extras#ac:first-returns-or-empty, typed-collection-extras#ac:first-unsupported
**Depends-On:** —
**Status:** done

Implement `First(ctx, ReadSession) (T, bool, error)` building a limit-1 `StructuredQuery` over the `CollectionRef` with a `new(T)` factory; return `(value, true, nil)` for a non-empty collection, `(zero T, false, nil)` for an empty one, and surface `dal.ErrNotSupported` from incapable backends.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
