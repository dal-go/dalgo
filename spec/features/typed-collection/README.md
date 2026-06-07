---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Typed Collection[T] convenience layer (point CRUD)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/typed-collection?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/typed-collection?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/typed-collection?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/typed-collection?op=request-change) |
**Status:** Approved
**Date:** 2026-06-07
**Owner:** alex
**Source Ideas:** generics-convenience-layer
**Supersedes:** —
**Grade:** A

## Summary

A session-less generic dal.Collection[T] handle in package dal with point-CRUD terminals (Get/All/InsertWithID/Set/Update/Delete) and .In nesting, built additively over the existing dal session interfaces. Generated Insert and batch are separate Features.

## Problem

dalgo's record/key model is explicit and portable but verbose: reading or writing one typed record means constructing a `Key`, wrapping data with `NewRecordWithData`, calling a session method, then type-asserting the data back out. Every competitor (GORM generics, upper/db, Bun, ent) has converged on a typed, context-first convenience shape that returns `(T, error)` directly. This Feature adds that shape to dalgo for **point CRUD** — without reflection, codegen, or any change to the existing `dal` interfaces. Two findings from the source Idea constrain the API: Go forbids type parameters on methods (so the type parameter must enter via a free function or a generic type), and dalgo's `DB` satisfies `ReadSession` but not `WriteSession` (writes go through a transaction), which the design turns into compile-time read/write safety by passing the session per call. Generated-id `Insert` and batch are deliberately **separate Features** (the former needs adapter-side `InsertOption` support); this Feature is the additive, `dal`-only point-CRUD core.

## Behavior

### Handle construction

#### REQ: session-less-handle

`dal` MUST provide a session-less generic handle — the interface `dal.Collection[T]` — created two ways: `CollectionOf[T CollectionNamer]()` resolves the collection name from `T`'s `CollectionName() string` method (declared on a **value** receiver), and `CollectionAt[T any](name string)` takes an explicit name. Both return `dal.Collection[T]`. The handle MUST carry only path identity — a composed `dal.CollectionRef` (name plus an optional parent) and the phantom type `T` — and MUST hold no session or connection, so it is a reusable value that can be declared once (e.g. a package-level `var`).

### Identifier argument

#### REQ: id-argument

Every point terminal MUST take an `id any` argument that is EITHER a plain id value (matching the core's `Key.ID any`) OR an eager `dal.KeyOption` that sets a concrete id (`WithID` for a single id, `WithFields` for a composite id). No new id type is introduced. Id-generating options are NOT accepted here (see Not Doing — generated `Insert` is a separate Feature).

### Read terminals

#### REQ: typed-get

`Collection[T]` MUST provide `Get(ctx, s ReadSession, id any) (T, error)` that returns the decoded value of type `T`. On not-found it MUST return the zero value of `T` together with the not-found error returned by the underlying session `Get` CALL — it MUST NOT rely on `record.Error()` (which is nil for not-found by design) or `record.Exists()` (which can panic if the record was never fetched).

#### REQ: typed-all

`Collection[T]` MUST provide `All(ctx, s ReadSession) ([]T, error)` returning every record in the collection, each decoded into a freshly allocated value so that no two results alias the same backing struct. Where the backend cannot execute the query, `All` MUST surface `dal.ErrNotSupported` rather than silently returning an empty slice.

### Write terminals

#### REQ: insert-with-id

`Collection[T]` MUST provide `InsertWithID(ctx, s WriteSession, id any, value T) (*dal.Key, error)` that inserts a new record at a KNOWN id and returns the record's key. (Generated ids are out of scope for this Feature.)

#### REQ: typed-set

`Collection[T]` MUST provide `Set(ctx, s WriteSession, id any, value T) error` that stores (upserts) `value` at `id`.

#### REQ: typed-update

`Collection[T]` MUST provide `Update(ctx, s WriteSession, id any, updates []update.Update, preconditions ...dal.Precondition) error` that applies field-level updates to the record at `id`, mirroring the existing `dal.Updater.Update` shape (updates slice + variadic preconditions).

#### REQ: typed-delete

`Collection[T]` MUST provide `Delete(ctx, s WriteSession, id any) error` that deletes the record at `id`.

### Read/write session safety

#### REQ: session-type-safety

Read terminals MUST take a `dal.ReadSession` and write terminals a `dal.WriteSession`. Because `dal.DB` satisfies `ReadSession` but not `WriteSession`, invoking a write terminal with a plain `DB` MUST be a compile error, while invoking the same terminal with a transaction handle (which satisfies `WriteSession`/`ReadwriteSession`) MUST compile. No separate read-only vs read-write handle types are introduced.

### Nesting

#### REQ: subcollection-nesting

`Collection[T]` MUST provide `In(parent *dal.Key) dal.Collection[T]` that returns a handle scoped under `parent` by composing the existing `CollectionRef` parent chain (one level of nesting is in scope). If `parent` is an incomplete key (no id), a terminal on the resulting handle MUST return a descriptive error rather than panicking.

### Additivity

#### REQ: additive-no-core-changes

The layer MUST live in package `dal`, implemented over the existing session interfaces and `Key`/`CollectionRef` (reused by composition, never generified). It MUST add no reflection of its own — value decoding remains the adapter's responsibility, exactly as today — MUST change no existing call site, and MUST NOT introduce a `dal` → `record` import (so no import cycle).

## Acceptance Criteria

### AC: construct-by-convention (verifies REQ:session-less-handle)

**Given** a type `User` with a value-receiver method `CollectionName() string` returning `"users"`
**When** `dal.CollectionOf[User]()` is called
**Then** it returns a `dal.Collection[User]` whose collection path is `"users"` and which holds no session, and the same value can be reused across multiple calls.

### AC: construct-by-explicit-name (verifies REQ:session-less-handle)

**Given** a type `T` that does not implement `CollectionNamer`
**When** `dal.CollectionAt[T]("things")` is called
**Then** it returns a `dal.Collection[T]` whose collection path is `"things"`.

### AC: id-plain-or-keyoption (verifies REQ:id-argument)

**Given** a `Collection[User]` and a record stored at id `"u1"`
**When** `Get` is called once with the plain value `"u1"` and once with `dal.WithID("u1")`
**Then** both address the same record; and a record with a composite key is addressable by passing `dal.WithFields(...)` as the id.

### AC: typed-get-roundtrip (verifies REQ:typed-get)

**Given** a `User{Name:"Alice"}` previously stored at id `"u1"`
**When** `Get(ctx, db, "u1")` is called
**Then** it returns the `User` value with `Name=="Alice"` and a nil error.

### AC: get-not-found (verifies REQ:typed-get)

**Given** no record stored at id `"missing"`
**When** `Get(ctx, db, "missing")` is called
**Then** it returns the zero `User` value and a not-found error obtained from the session `Get` call (not from `record.Error()`/`record.Exists()`).

### AC: typed-all-distinct (verifies REQ:typed-all)

**Given** two distinct `User` records stored in the collection
**When** `All(ctx, db)` is called
**Then** it returns a slice of two `User` values that do not alias one another (mutating one element does not change the other).

### AC: all-unsupported-surfaces-error (verifies REQ:typed-all)

**Given** a `ReadSession` whose query executor reports the query is unsupported
**When** `All(ctx, s)` is called
**Then** it returns `dal.ErrNotSupported` rather than an empty slice with a nil error.

### AC: insert-with-id-returns-key (verifies REQ:insert-with-id)

**Given** a `Collection[User]` and a writable transaction `tx`
**When** `InsertWithID(ctx, tx, "u1", User{Name:"Alice"})` is called
**Then** a record exists at id `"u1"` and the returned `*dal.Key` has ID `"u1"` and a nil error.

### AC: set-upserts (verifies REQ:typed-set)

**Given** a `Collection[User]` and a writable transaction `tx`
**When** `Set(ctx, tx, "u1", User{Name:"Bob"})` is called, whether or not a record already exists at `"u1"`
**Then** the stored record at `"u1"` equals `User{Name:"Bob"}`.

### AC: update-applies-fields (verifies REQ:typed-update)

**Given** a `User{Name:"Alice"}` stored at `"u1"`
**When** `Update(ctx, tx, "u1", []update.Update{update.ByFieldName("name","Bob")})` is called
**Then** the stored record at `"u1"` has `Name=="Bob"`.

### AC: delete-removes (verifies REQ:typed-delete)

**Given** a `User` stored at `"u1"`
**When** `Delete(ctx, tx, "u1")` is called
**Then** a subsequent `Get(ctx, db, "u1")` returns the not-found error.

### AC: write-needs-write-session (verifies REQ:session-type-safety)

**Given** a plain `dal.DB` (which satisfies `ReadSession` but not `WriteSession`)
**When** code attempts to call a write terminal such as `Set(ctx, db, "u1", value)`
**Then** it does not compile (verified by the method signatures and a non-compiling example), whereas the same call inside `RunReadwriteTransaction` (whose handle satisfies `WriteSession`) compiles and commits.

### AC: nested-get (verifies REQ:subcollection-nesting)

**Given** a complete parent key for `users/u1` and a `Contact` stored at `users/u1/contacts/c1`
**When** `dal.CollectionOf[Contact]().In(parentKey).Get(ctx, db, "c1")` is called
**Then** it resolves the path `users/u1/contacts` and returns the stored `Contact`.

### AC: nested-incomplete-parent-errors (verifies REQ:subcollection-nesting)

**Given** an incomplete parent key (collection set, id absent)
**When** a terminal is called on the handle returned by `.In(incompleteParent)`
**Then** it returns a descriptive error and does not panic.

### AC: additive-suite-stays-green (verifies REQ:additive-no-core-changes)

**Given** the new `Collection[T]` layer added to package `dal`
**When** the full pre-existing `dal` + `dalgo2memory` + `end2end` test suites are run and the import graph is inspected
**Then** every pre-existing test passes unchanged and `dal` still does not import the `record` package.

## Architecture & Components

- **`dal.Collection[T]` (interface)** — the typed contract: `Get`/`All`/`InsertWithID`/`Set`/`Update`/`Delete`/`In`. Lives in `dal` (the contracts package), alongside `ReadSession`/`Getter`/etc.
- **unexported impl (in `dal`)** — a small value composing a `dal.CollectionRef` (name + optional parent) and the phantom `T`. Each terminal builds a `Key` from the handle's `CollectionRef` + the `id` argument (via `NewKeyWithOptions` when the id is a `KeyOption`, else by setting `Key.ID` directly), wraps `new(T)` with `NewRecordWithData`, calls the matching session method, and returns the typed value / key / error.
- **`CollectionNamer` (interface)** — `CollectionName() string`; a constraint on `CollectionOf`.
- **Constructors** — `CollectionOf[T CollectionNamer]()` and `CollectionAt[T any](name string)`, both returning `dal.Collection[T]`.
- **Dependencies** — existing `dal` session interfaces (`ReadSession`/`WriteSession`), `Key`/`CollectionRef`, and the `update` package. No new external dependency; no `record` import.

Data flow (Get): `id` → `Key` (handle `CollectionRef` + id) → `NewRecordWithData(key, new(T))` → `ReadSession.Get` → return `*new(T)`, mapping the call's not-found error to `(zero T, err)`.

Error handling: not-found is taken from the session call's returned error; an incomplete `.In` parent yields a descriptive error; an unsupported `All` surfaces `dal.ErrNotSupported`.

## Not Doing / Out of Scope

- **Generated-id `Insert`** (`Insert(ctx, ws, value, ...InsertOption)`) — needs adapter-side `InsertOption` support (approach C); it is the separate `collection-generated-insert` Feature.
- **Batch** (`ManyInserter[T]`/`Item[T]`) and **`Count`/`Exists`/`First`** — a later Feature.
- **Query filtering** (`Where`/typed query terminal, pagination) — borrow #4/#5, a separate Idea/Feature; `All` reads the whole collection here.
- **Multi-level nesting** (3+ collection segments) — one level of `.In` is in scope.
- **Compile-time typed ids** (`Collection[T, ID comparable]`) — `id any` matches the core's `Key.ID any`.
- **Adapter changes** — none; this Feature is `dal`-only and additive.

## Rehearse Integration

No Rehearse markdown stubs. Every AC has a concrete Go test surface (pure-function construction, in-memory data round-trips, a non-compiling example for the read/write-safety AC, and the existing `dal`/`dalgo2memory`/`end2end` suites for additivity). Those Go tests are authored during implementation and are the canonical verification for this internal library API; markdown stubs would duplicate them without adding signal. (Override available if the team later wants Rehearse scenarios.)

## Open Questions

- **`CollectionName()` receiver convention** — pinned to **value** receiver (so `CollectionOf[User]()` works without `*User`); to be documented in the package doc.
- **`All` ↔ future `Where`/`First` seam** — `All` should build its `StructuredQuery` through an internal helper that a later filtering terminal can reuse, so adding `Where`/`First` does not require a redesign.

---
*This document follows the https://specscore.md/feature-specification*
