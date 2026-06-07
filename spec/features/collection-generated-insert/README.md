---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Generated-ID Insert for Collection[T] (approach C)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/collection-generated-insert?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/collection-generated-insert?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/collection-generated-insert?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/collection-generated-insert?op=request-change) |
**Status:** Approved
**Date:** 2026-06-07
**Owner:** alex
**Source Ideas:** generics-convenience-layer
**Supersedes:** —
**Grade:** A

## Summary

Adds the bare Insert(ctx, ws, value, ...InsertOption) (*Key, error) terminal to dal.Collection[T] for generated ids, and makes adapters (dalgo2memory first) honor InsertOption by running id generation + collision-retry at the storage layer (approach C), with a loud-failure guard and an end2end proof.

## Problem

The `typed-collection` Feature deliberately excluded generated-id inserts because they are not pure layer sugar: dalgo's deferred, retry-aware id-generation path (`InsertOption` → `InsertWithIdGenerator`) is currently a **silent no-op in every shipped adapter** — `dalgo2memory` discards the `...InsertOption` variadic and `dalgo2fs` returns `ErrNotImplementedYet`, and no production code wires `InsertWithIdGenerator`. So a typed `Insert` that just forwarded an `InsertOption` would silently store a record under a `<nil>` id. The source Idea chose **approach C**: make adapters honor `InsertOption` at the storage layer (a coordinated, storage-specific change that also fixes the latent gap for ALL `session.Insert` callers), and add a typed `Insert` terminal on top with a loud-failure guard. dalgo's eager `WithRandomStringID`/`WithIDGenerator` KeyOptions are NOT used (they assign the id at option-apply time with a nil ctx and no collision-retry); the deferred `InsertOption` family (`WithRandomStringKey`, …) is.

## Behavior

### Typed generated-insert terminal

#### REQ: generated-insert-terminal

`dal.Collection[T]` MUST provide `Insert(ctx, s WriteSession, value T, opts ...dal.InsertOption) (*dal.Key, error)` that inserts `value` under a generated id and returns the assigned key. When `opts` is empty the layer MUST inject a default generator equivalent to `WithRandomStringKey(dal.DefaultRandomStringIDLength, 5)`; when `opts` are supplied they MUST be passed through to the underlying `WriteSession.Insert`. The record handed to the session MUST start with an incomplete key (collection + optional parent from the handle, id absent) so the generator fills it.

#### REQ: loud-failure-guard

After the underlying insert returns, `Insert` MUST verify the record's key has a non-nil id and return a descriptive error otherwise. This makes generated `Insert` fail LOUDLY on any adapter that ignores `InsertOption` (which would otherwise store a `<nil>`-id record) instead of reporting a false success.

#### REQ: generator-compile-rejection

Only the bare `Insert` MUST accept `...dal.InsertOption`; `InsertWithID`/`Get`/`Set`/`Update`/`Delete` MUST NOT. Because `InsertOption` is a distinct type accepted by no other terminal, passing a generator to any id-taking terminal MUST be a compile error — no marker interface or runtime guard is introduced.

### Adapter support (approach C)

#### REQ: memory-honors-insert-option

`dalgo2memory`'s write path MUST honor `...dal.InsertOption`: when `NewInsertOptions(opts...).IDGenerator()` is non-nil, it MUST run the generator with collision-retry (via `dal.InsertWithIdGenerator` or a storage-specific equivalent) to fill an incomplete key, persisting under the generated id. This MUST also make a raw `session.Insert(ctx, record, gen)` (outside the typed layer) generate an id, fixing the latent no-op.

#### REQ: generation-before-storage

When honoring a generator, the adapter MUST run id generation BEFORE computing the storage key, so the record is never stored under a `<nil>` id; on generator exhaustion it MUST return the generator's error (e.g. `ErrExceedsMaxNumberOfAttempts`), not a partial write.

### Proof

#### REQ: end2end-generated-insert

A `dalgo2memory`-specific test (NOT the shared, gomock-driven `TestDalgoDB`) MUST exercise generated `Insert` — both with an explicit `WithRandomStringKey` and with the zero-option default — asserting a COMPLETE (non-empty id) key is returned and the record round-trips via `Get`. A separate test MUST assert that the loud-failure guard makes `Insert` error against a stub `WriteSession` that ignores `InsertOption`.

## Architecture & Components

- **`dal.Collection[T].Insert` (terminal, in `dal`)** — builds a record with an INCOMPLETE key from the handle's `CollectionRef` (`NewRecordWithIncompleteKey`), forwards `value` + `opts` (defaulting to `WithRandomStringKey(dal.DefaultRandomStringIDLength, 5)` when empty) to `WriteSession.Insert`, then applies the loud-failure guard and returns `record.Key()`.
- **Loud-failure guard** — after the session insert returns nil error, the terminal checks `record.Key().ID` is non-nil/non-empty; if not, it returns a descriptive sentinel-style error (e.g. an exported `dal.ErrInsertOptionNotHonored` or wrapped equivalent) so an adapter that ignored `InsertOption` fails loudly rather than reporting a `<nil>`-id success.
- **`dalgo2memory` write path (approach C)** — branches on `NewInsertOptions(opts...).IDGenerator()`: when non-nil, runs `dal.InsertWithIdGenerator` supplying an `exists` predicate (wrapping the engine's existence check) and an `insert` callback (the engine store), so generation + collision-retry happen before the storage key is computed.
- **Dependencies** — the `typed-collection` Feature (the `Collection[T]` type this terminal extends), plus the existing `dal.InsertOption`/`InsertOptions`/`IDGenerator`/`InsertWithIdGenerator` machinery and `dalgo2memory`'s engine.

## Not Doing / Out of Scope

- **Other adapters** (`dalgo2fs`/`dalgo2sql`/`dalgo2firestore`/`dalgo2datastore`) honoring `InsertOption` — `dalgo2memory` only here; others follow per the Open Question (the guard keeps them safe meanwhile).
- **Eager `WithRandomStringID`/`WithIDGenerator` KeyOptions** — not used for generation in this layer (they run at option-apply time with a nil ctx and no retry).
- **Batch insert** (`ManyInserter`/`Item`) and **`Count`/`Exists`/`First`** — a separate Feature.
- **Point CRUD terminals** (`Get`/`All`/`InsertWithID`/`Set`/`Update`/`Delete`) — owned by `typed-collection`.

## Dependencies

- typed-collection

## Acceptance Criteria

### AC: generated-insert-returns-key (verifies REQ:generated-insert-terminal)

**Given** a `dal.Collection[User]` and a writable transaction `tx` against `dalgo2memory`
**When** `Insert(ctx, tx, User{Name:"Alice"})` is called with no options
**Then** it returns a `*dal.Key` whose ID is a non-empty generated string and a nil error, and the stored record round-trips via `Get`.

### AC: generated-insert-explicit-option (verifies REQ:generated-insert-terminal)

**Given** a `dal.Collection[User]` and a writable `tx`
**When** `Insert(ctx, tx, value, dal.WithRandomStringKey(20, 5))` is called
**Then** the returned key's ID is a 20-character generated string.

### AC: loud-failure-on-nonhonoring-adapter (verifies REQ:loud-failure-guard)

**Given** a stub `WriteSession` whose `Insert` ignores `...InsertOption` (leaving the key incomplete)
**When** `Collection[T].Insert(ctx, stub, value)` is called
**Then** it returns a descriptive error (it does NOT report success with a `<nil>`/empty id).

### AC: generator-not-accepted-elsewhere (verifies REQ:generator-compile-rejection)

**Given** the `Collection[T]` method set
**When** code attempts to pass a `dal.InsertOption` to `InsertWithID`, `Get`, `Set`, `Update`, or `Delete`
**Then** it does not compile (only the bare `Insert` accepts `...dal.InsertOption`).

### AC: memory-generates-via-insert-option (verifies REQ:memory-honors-insert-option)

**Given** a `dalgo2memory` session and a record built with an incomplete key
**When** `session.Insert(ctx, record, dal.WithRandomStringKey(16, 5))` is called directly (outside the typed layer)
**Then** the record is persisted under a generated non-empty id and is retrievable by that id.

### AC: generation-precedes-storage (verifies REQ:generation-before-storage)

**Given** a `dalgo2memory` session honoring a generator
**When** an insert with a generator runs
**Then** no record is ever stored under a `<nil>` id, and if the generator exhausts its attempts the call returns the generator's error with nothing persisted.

### AC: e2e-generated-insert-roundtrip (verifies REQ:end2end-generated-insert)

**Given** the `dalgo2memory`-specific generated-insert test (not the shared `TestDalgoDB`)
**When** it inserts via the typed `Insert` (default and explicit generator) and reads back
**Then** each returned key is complete and each record round-trips, and the existing shared `TestDalgoDB` suite stays green (unchanged).

## Open Questions

- **Per-backend approach-C rollout** — `dalgo2memory` is in scope here; `dalgo2fs`/`dalgo2sql`/`dalgo2firestore`/`dalgo2datastore` honor `InsertOption` later (some backends generate ids server-side, so "honor" may mean "defer to the backend"). Until an adapter implements it, the loud-failure guard ensures generated `Insert` errors there rather than silently no-opping.

---
*This document follows the https://specscore.md/feature-specification*
