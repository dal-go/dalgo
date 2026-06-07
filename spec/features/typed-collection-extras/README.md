---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Collection[T] batch insert and Count/Exists/First terminals

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/typed-collection-extras?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/typed-collection-extras?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/typed-collection-extras?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/typed-collection-extras?op=request-change) |
**Status:** Approved
**Date:** 2026-06-07
**Owner:** alex
**Source Ideas:** generics-convenience-layer
**Supersedes:** —
**Grade:** A

## Summary

Adds the opt-in ManyInserter[T] batch-insert interface (with dal-native Item[T]) and the Count/Exists/First read conveniences to dal.Collection[T], all additive over existing dal interfaces and cycle-free.

## Problem

`typed-collection` ships the point-CRUD core but defers the remaining ergonomic terminals the source Idea identified: batch insert and the `Count`/`Exists`/`First` read conveniences (borrow-doc #2). These are mechanical follow-ons on the same `Collection[T]` handle, but each has a distinct contract worth pinning, and batch insert needs a `dal`-native id+value pair so the layer stays cycle-free (no `record` import). This Feature adds them additively, mirroring dalgo's existing single/multi split.

## Behavior

### Batch insert

#### REQ: item-type

`dal` MUST define `Item[T any]` as a struct `{ ID any; Value T }` — a dal-native id+value pair (the `ID` follows the same `id any` = plain value | `WithID`/`WithFields` `KeyOption` convention as the point terminals). It MUST NOT reference the `record` package, so the batch API adds no `dal` → `record` import.

#### REQ: many-inserter

`dal` MUST define `ManyInserter[T any]` as a separate interface `{ InsertMany(ctx, s WriteSession, items ...Item[T]) (keys []*dal.Key, err error) }`, mirroring dalgo's `Inserter`/`MultiInserter` split. The concrete `Collection[T]` value MUST satisfy it. `InsertMany` MUST insert each item at its known id (delegating to the session's `MultiInserter` where available, else a sequential fallback) and return the keys in input order. Generated ids are out of scope here (that is the `collection-generated-insert` Feature).

### Read conveniences

#### REQ: count

`Collection[T]` MUST provide `Count(ctx, s ReadSession) (int, error)` returning the number of records in the collection. Where the backend cannot execute the underlying count query it MUST surface `dal.ErrNotSupported` rather than a silent `0`.

#### REQ: exists

`Collection[T]` MUST provide `Exists(ctx, s ReadSession, id any) (bool, error)` reporting whether a record exists at `id`. A not-found result MUST map to `(false, nil)` — not an error — while any other failure is returned as `(false, err)`.

#### REQ: first

`Collection[T]` MUST provide `First(ctx, s ReadSession) (value T, found bool, err error)` returning the first record in the collection (an underlying limit-1 query). An empty collection MUST yield `(zero T, false, nil)`; an incapable backend MUST surface `dal.ErrNotSupported`.

## Architecture & Components

- **`dal.Item[T]` + `dal.ManyInserter[T]`** — new dal-native types; `Collection[T]`'s concrete impl implements `ManyInserter[T]` by building `[]dal.Record` (each with a complete key from `Item.ID` + the handle's `CollectionRef`) and calling the session's `MultiInserter.InsertMulti`, falling back to per-item `Inserter.Insert` when multi is unavailable.
- **`Count`/`First`** — build a `StructuredQuery` over the handle's `CollectionRef` (count aggregation; limit-1 select with a per-row `new(T)` factory) and run it via the `ReadSession`'s `QueryExecutor`, inheriting `ErrNotSupported` portability — like `All` in `typed-collection`.
- **`Exists`** — a key-only existence probe: build the key from `id` + the handle's `CollectionRef` and call the session getter, mapping a not-found error to `(false, nil)`, any other error to `(false, err)`, and success to `(true, nil)`.
- **Dependencies** — `typed-collection` (the `Collection[T]` type and its `CollectionRef`/`id any` conventions), plus existing `dal` `MultiInserter`/`QueryExecutor`.

## Not Doing / Out of Scope

- **Generated-id batch insert** — `Item.ID` carries a known id; generated ids are the `collection-generated-insert` Feature.
- **Filtered `Count`/`First`** (`Where`/predicate) — whole-collection only here; query filtering is a separate Idea/Feature.
- **`Last`/`ScanAndCount`/pagination** — not in scope; pagination is borrow #5 (separate).
- **Point CRUD terminals** — owned by `typed-collection`.

## Dependencies

- typed-collection

## Acceptance Criteria

### AC: item-type-no-record-import (verifies REQ:item-type)

**Given** the new `dal.Item[T]` type
**When** the `dal` package is built and its import graph inspected
**Then** `Item[T]` is `{ID any; Value T}` and `dal` still does not import the `record` package.

### AC: insert-many-roundtrips (verifies REQ:many-inserter)

**Given** a `Collection[User]` and a writable `tx`
**When** `InsertMany(ctx, tx, dal.Item[User]{ID:"u1",Value:a}, dal.Item[User]{ID:"u2",Value:b})` is called
**Then** records exist at `"u1"` and `"u2"`, and the returned keys are `["u1","u2"]` in input order.

### AC: count-returns-total (verifies REQ:count)

**Given** three records stored in the collection
**When** `Count(ctx, db)` is called
**Then** it returns `3` and a nil error.

### AC: count-unsupported (verifies REQ:count)

**Given** a `ReadSession` whose query executor reports the count query is unsupported
**When** `Count(ctx, s)` is called
**Then** it returns `dal.ErrNotSupported` (not `0, nil`).

### AC: exists-true-false (verifies REQ:exists)

**Given** a record stored at `"u1"` and none at `"missing"`
**When** `Exists(ctx, db, "u1")` and `Exists(ctx, db, "missing")` are called
**Then** the first returns `(true, nil)` and the second returns `(false, nil)`.

### AC: first-returns-or-empty (verifies REQ:first)

**Given** a non-empty collection, then an empty one
**When** `First(ctx, db)` is called on each
**Then** the non-empty case returns `(value, true, nil)` and the empty case returns `(zero T, false, nil)`.

### AC: exists-error-passthrough (verifies REQ:exists)

**Given** a `ReadSession` whose lookup fails with a non-not-found error
**When** `Exists(ctx, s, "u1")` is called
**Then** it returns `(false, err)` with that underlying error (it does NOT swallow it as `(false, nil)`).

### AC: first-unsupported (verifies REQ:first)

**Given** a `ReadSession` whose query executor reports the limit-1 query is unsupported
**When** `First(ctx, s)` is called
**Then** it returns `dal.ErrNotSupported` (not `(zero T, false, nil)`).

## Open Questions

- **Ordering of `First`** — with no `OrderBy`, "first" is backend-defined; a later filtering/ordering Feature can add an explicit order. Documented as backend-defined for now.

---
*This document follows the https://specscore.md/feature-specification*
