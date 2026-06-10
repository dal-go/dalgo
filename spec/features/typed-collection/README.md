---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Typed Collection[K, T] convenience layer (point CRUD)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/typed-collection?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/typed-collection?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/typed-collection?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/typed-collection?op=request-change) |
**Status:** Approved
**Date:** 2026-06-07
**Owner:** alex
**Source Ideas:** generics-convenience-layer
**Supersedes:** —
**Grade:** A

## Summary

A session-less generic dal.Collection[K, T] handle in package dal with point-CRUD terminals keyed by a typed id K. Reads expose GetData/GetRecord plus the typed id accessors GetRecordWithID/GetRecordWithDataAndID and the dal.GetRecordWithIDIntoData factory function (with a deprecated record.GetWithID forwarder); writes expose InsertWithID/InsertRecord/SetByID/SetRecord/UpdateByID/UpdateByKey/DeleteByID/DeleteByKey, with deprecated Get/Set/Update/Delete aliases. Key options (composite keys, parent) are configured on the constructor via WithKeyOptions. Built additively over the existing dal session interfaces. Generated Insert and batch are separate Features.

## Problem

dalgo's record/key model is explicit and portable but verbose: reading or writing one typed record means constructing a `Key`, wrapping data with `NewRecordWithData`, calling a session method, then type-asserting the data back out. Every competitor (GORM generics, upper/db, Bun, ent) has converged on a typed, context-first convenience shape that returns `(T, error)` directly. This Feature adds that shape to dalgo for **point CRUD** — without reflection, codegen, or any change to the existing `dal` interfaces. Two findings from the source Idea constrain the API: Go forbids type parameters on methods (so a typed-with-id accessor that returns `record.WithID[K]` must be a free function in package `record`, keeping `dal` free of any `record` import), and dalgo's `DB` satisfies `ReadSession` but not `WriteSession` (writes go through a transaction), which the design turns into compile-time read/write safety by passing the session per call. The id type is a type parameter `K` so ids are statically typed rather than `any`. Generated-id `Insert` and batch are deliberately **separate Features** (the former needs adapter-side `InsertOption` support); this Feature is the additive, `dal`-only point-CRUD core.

## Behavior

### Handle construction

#### REQ: session-less-handle

`dal` MUST provide a session-less generic handle — the interface `dal.Collection[K comparable, T any]` — created two ways: `CollectionOf[K comparable, T CollectionNamer](opts ...CollectionOption)` resolves the collection name from `T`'s `CollectionName() string` method (declared on a **value** receiver), and `CollectionAt[K comparable, T any](name string, opts ...CollectionOption)` takes an explicit name. Both return `dal.Collection[K, T]`. The handle MUST carry only path identity — a composed `dal.CollectionRef` (name plus an optional parent) — the configured key options, and the phantom types `K, T`, and MUST hold no session or connection, so it is a reusable value that can be declared once (e.g. a package-level `var`).

### Identifier argument

#### REQ: id-argument

Every point terminal that takes an id MUST take a strongly typed `id K` argument (a scalar value such as `string` or `int`), matching the core's `Key.ID`. KeyOptions MUST NOT be passed in the id slot; instead, collection-level key configuration (e.g. `WithFields` for a composite id, or a parent key) is supplied to the constructor via `dal.WithKeyOptions(...dal.KeyOption)` and applied when each key is built. Composite / per-record keys are addressed through the `*ByKey` terminals (build a `*dal.Key` with `NewKeyWithFields`). Id-generating options are NOT accepted here (see Not Doing — generated `Insert` is a separate Feature).

### Read terminals

#### REQ: typed-get

`Collection[K, T]` MUST provide `GetData(ctx, s ReadSession, id K) (T, error)` that returns the decoded value of type `T`. On not-found it MUST return the zero value of `T` together with the not-found error returned by the underlying session `Get` CALL — it MUST NOT rely on `record.Error()` (which is nil for not-found by design) or `record.Exists()` (which can panic if the record was never fetched). A deprecated `Get(ctx, s ReadSession, id K) (T, error)` alias MUST remain, delegating to `GetData`.

#### REQ: typed-get-record

`Collection[K, T]` MUST provide `GetRecord(ctx, s ReadSession, id K) (dal.Record, error)` that returns the underlying `dal.Record` (its `Data()` is a `*T`). It is the read primitive the other read accessors delegate to.

`Collection[K, T]` MUST additionally provide typed id-bearing accessors built over `GetRecord`:
- `GetRecordWithID(ctx, s ReadSession, id K) (dal.RecordWithID[K], error)` — id + key + record (no typed data).
- `GetRecordWithDataAndID(ctx, s ReadSession, id K) (dal.RecordWithDataAndID[K, *T], error)` — adds the decoded `*T` (the same pointer the Record holds).

For models whose data type is an interface created by a factory (so `new(T)` cannot allocate it), `dal` MUST provide a free function `GetRecordWithIDIntoData[K comparable, D any](ctx, s ReadSession, key *dal.Key, id K, data D) (dal.RecordWithDataAndID[K, D], error)` that decodes INTO the caller-supplied `data` value. It is a free function (not a method) because Go forbids type parameters on methods, so the decoupled data type `D` cannot be a `Collection[K, T]` method.

For backward compatibility package `record` MUST keep `GetWithID[K comparable, T any](...) (record.WithID[K], error)` as a deprecated thin forwarder to `Collection.GetRecordWithID` (`record.WithID` is an alias for `dal.RecordWithID`).

#### REQ: typed-all

`Collection[K, T]` MUST provide `All(ctx, s ReadSession) ([]T, error)` returning every record in the collection, each decoded into a freshly allocated value so that no two results alias the same backing struct. Where the backend cannot execute the query, `All` MUST surface `dal.ErrNotSupported` rather than silently returning an empty slice.

### Write terminals

#### REQ: insert-with-id

`Collection[K, T]` MUST provide `InsertWithID(ctx, s WriteSession, id K, value T) (*dal.Key, error)` that inserts a new record at a KNOWN id and returns the record's key. It MUST delegate to the record primitive `InsertRecord(ctx, s WriteSession, r dal.Record, opts ...dal.InsertOption) error`. (Generated ids are out of scope for this Feature.)

#### REQ: typed-set

`Collection[K, T]` MUST provide `SetByID(ctx, s WriteSession, id K, value T) error` that stores (upserts) `value` at `id`, plus the record primitive `SetRecord(ctx, s WriteSession, r dal.Record) error` it delegates to. A deprecated `Set(ctx, s WriteSession, id K, value T) error` alias MUST remain, delegating to `SetByID`.

#### REQ: typed-update

`Collection[K, T]` MUST provide `UpdateByID(ctx, s WriteSession, id K, updates []update.Update, preconditions ...dal.Precondition) error` and `UpdateByKey(ctx, s WriteSession, k *dal.Key, updates []update.Update, preconditions ...dal.Precondition) error`, mirroring the existing `dal.Updater.Update` shape (updates slice + variadic preconditions); `UpdateByID` delegates to `UpdateByKey`. A deprecated `Update(...)` alias MUST remain, delegating to `UpdateByID`.

#### REQ: typed-delete

`Collection[K, T]` MUST provide `DeleteByID(ctx, s WriteSession, id K) error` and `DeleteByKey(ctx, s WriteSession, k *dal.Key) error` that delete the record at `id` / `k`; `DeleteByID` delegates to `DeleteByKey`. A deprecated `Delete(ctx, s WriteSession, id K) error` alias MUST remain, delegating to `DeleteByID`.

### Read/write session safety

#### REQ: session-type-safety

Read terminals MUST take a `dal.ReadSession` and write terminals a `dal.WriteSession`. Because `dal.DB` satisfies `ReadSession` but not `WriteSession`, invoking a write terminal with a plain `DB` MUST be a compile error, while invoking the same terminal with a transaction handle (which satisfies `WriteSession`/`ReadwriteSession`) MUST compile. No separate read-only vs read-write handle types are introduced.

### Nesting

#### REQ: subcollection-nesting

`Collection[K, T]` MUST provide `In(parent *dal.Key) dal.Collection[K, T]` that returns a handle scoped under `parent` by composing the existing `CollectionRef` parent chain (one level of nesting is in scope), carrying over the configured key options. If `parent` is an incomplete key (no id), a terminal on the resulting handle MUST return a descriptive error rather than panicking.

### Additivity

#### REQ: additive-no-core-changes

The layer MUST live in package `dal`, implemented over the existing session interfaces and `Key`/`CollectionRef` (reused by composition, never generified). It MUST add no reflection of its own — value decoding remains the adapter's responsibility, exactly as today — and MUST NOT introduce a `dal` → `record` import (so no import cycle); the typed-with-id accessor lives in package `record` instead.

## Acceptance Criteria

### AC: construct-by-convention (verifies REQ:session-less-handle)

**Given** a type `User` with a value-receiver method `CollectionName() string` returning `"users"`
**When** `dal.CollectionOf[string, User]()` is called
**Then** it returns a `dal.Collection[string, User]` whose collection path is `"users"` and which holds no session, and the same value can be reused across multiple calls.

### AC: construct-by-explicit-name (verifies REQ:session-less-handle)

**Given** a type `T` that does not implement `CollectionNamer`
**When** `dal.CollectionAt[string, T]("things")` is called
**Then** it returns a `dal.Collection[string, T]` whose collection path is `"things"`.

### AC: key-options-on-constructor (verifies REQ:id-argument)

**Given** a `Collection[string, User]` constructed with `dal.WithKeyOptions(dal.WithParentKey(parent))`
**When** a terminal builds a key from a typed id `"u1"`
**Then** the configured key options are applied to the resulting key (e.g. the parent is set); and a failing key option is surfaced as an error from the terminal's id resolution.

### AC: typed-get-roundtrip (verifies REQ:typed-get)

**Given** a `User{Name:"Alice"}` previously stored at id `"u1"`
**When** `GetData(ctx, db, "u1")` is called
**Then** it returns the `User` value with `Name=="Alice"` and a nil error; the deprecated `Get` alias returns the same.

### AC: get-not-found (verifies REQ:typed-get)

**Given** no record stored at id `"missing"`
**When** `GetData(ctx, db, "missing")` is called
**Then** it returns the zero `User` value and a not-found error obtained from the session `Get` call (not from `record.Error()`/`record.Exists()`).

### AC: get-record-and-with-id (verifies REQ:typed-get-record)

**Given** a `User{Name:"Alice"}` stored at id `"u1"`
**When** `GetRecord`, `GetRecordWithID`, `GetRecordWithDataAndID`, and `dal.GetRecordWithIDIntoData` (and the deprecated `record.GetWithID` forwarder) are called for `"u1"`
**Then** `GetRecord` returns a `dal.Record` whose `Data()` is a `*User` with `Name=="Alice"`; `GetRecordWithID` returns a `dal.RecordWithID[string]` carrying id/key/record; `GetRecordWithDataAndID` additionally exposes `Data` as the `*User` the Record holds; `GetRecordWithIDIntoData` decodes into a caller-supplied value (including an interface value); and on not-found each returns the session's not-found error.

### AC: typed-all-distinct (verifies REQ:typed-all)

**Given** two distinct `User` records stored in the collection
**When** `All(ctx, db)` is called
**Then** it returns a slice of two `User` values that do not alias one another (mutating one element does not change the other).

### AC: all-unsupported-surfaces-error (verifies REQ:typed-all)

**Given** a `ReadSession` whose query executor reports the query is unsupported
**When** `All(ctx, s)` is called
**Then** it returns `dal.ErrNotSupported` rather than an empty slice with a nil error.

### AC: insert-with-id-returns-key (verifies REQ:insert-with-id)

**Given** a `Collection[string, User]` and a writable transaction `tx`
**When** `InsertWithID(ctx, tx, "u1", User{Name:"Alice"})` is called
**Then** a record exists at id `"u1"` and the returned `*dal.Key` has ID `"u1"` and a nil error; a direct `InsertRecord` with a caller-built record stores it the same way.

### AC: set-upserts (verifies REQ:typed-set)

**Given** a `Collection[string, User]` and a writable transaction `tx`
**When** `SetByID(ctx, tx, "u1", User{Name:"Bob"})` (or the deprecated `Set`) is called, whether or not a record already exists at `"u1"`
**Then** the stored record at `"u1"` equals `User{Name:"Bob"}`; a direct `SetRecord` upserts the same way.

### AC: update-applies-fields (verifies REQ:typed-update)

**Given** a `User{Name:"Alice"}` stored at `"u1"`
**When** `UpdateByID(ctx, tx, "u1", []update.Update{update.ByFieldName("name","Bob")})` is called (or `UpdateByKey` with the explicit key, or the deprecated `Update`)
**Then** the stored record at `"u1"` has `Name=="Bob"`.

### AC: delete-removes (verifies REQ:typed-delete)

**Given** a `User` stored at `"u1"`
**When** `DeleteByID(ctx, tx, "u1")` is called (or `DeleteByKey` with the explicit key, or the deprecated `Delete`)
**Then** a subsequent `GetData(ctx, db, "u1")` returns the not-found error.

### AC: write-needs-write-session (verifies REQ:session-type-safety)

**Given** a plain `dal.DB` (which satisfies `ReadSession` but not `WriteSession`)
**When** code attempts to call a write terminal such as `SetByID(ctx, db, "u1", value)`
**Then** it does not compile (verified by the method signatures and a non-compiling example), whereas the same call inside `RunReadwriteTransaction` (whose handle satisfies `WriteSession`) compiles and commits.

### AC: nested-get (verifies REQ:subcollection-nesting)

**Given** a complete parent key for `users/u1` and a `Contact` stored at `users/u1/contacts/c1`
**When** `dal.CollectionOf[string, Contact]().In(parentKey).GetData(ctx, db, "c1")` is called
**Then** it resolves the path `users/u1/contacts` and returns the stored `Contact`.

### AC: nested-incomplete-parent-errors (verifies REQ:subcollection-nesting)

**Given** an incomplete parent key (collection set, id absent)
**When** a terminal is called on the handle returned by `.In(incompleteParent)`
**Then** it returns a descriptive error and does not panic.

### AC: additive-suite-stays-green (verifies REQ:additive-no-core-changes)

**Given** the new `Collection[K, T]` layer added to package `dal`
**When** the full pre-existing `dal` + `dalgo2memory` + `end2end` test suites are run and the import graph is inspected
**Then** every pre-existing test passes unchanged and `dal` still does not import the `record` package.

## Architecture & Components

- **`dal.Collection[K, T]` (interface)** — the typed contract: `GetData`/`Get`(deprecated)/`GetRecord`/`GetRecordWithID`/`GetRecordWithDataAndID`/`All`/`InsertWithID`/`InsertRecord`/`SetByID`/`Set`(deprecated)/`SetRecord`/`UpdateByID`/`Update`(deprecated)/`UpdateByKey`/`DeleteByID`/`Delete`(deprecated)/`DeleteByKey`/`In`. Lives in `dal` (the contracts package), alongside `ReadSession`/`Getter`/etc.
- **`dal.GetRecordWithIDIntoData[K, D]` (free function)** — decodes into a caller-supplied `data` value, so `D` may be an interface (factory pattern); a free function because Go forbids method type parameters.
- **unexported impl (in `dal`)** — a small value composing a `dal.CollectionRef` (name + optional parent), the configured `[]KeyOption`, and the phantom `K, T`. `idToKey(id K)` builds a `Key` from the handle's `CollectionRef`, sets `Key.ID = id`, applies the collection's key options via `setKeyOptions`, then guards the parent chain. Each terminal wraps `new(T)`/`&value` with `NewRecordWithData`, calls the matching session method, and returns the typed value / key / error. `*ByID` terminals delegate to the corresponding `*ByKey`/`*Record` primitive.
- **`CollectionOption` + `WithKeyOptions`** — a constructor option carrying collection-level `dal.KeyOption`s.
- **`CollectionNamer` (interface)** — `CollectionName() string`; a constraint on `CollectionOf`.
- **Constructors** — `CollectionOf[K comparable, T CollectionNamer](opts ...CollectionOption)` and `CollectionAt[K comparable, T any](name string, opts ...CollectionOption)`, both returning `dal.Collection[K, T]`.
- **`record.GetWithID[K, T]` (deprecated free function)** — a thin forwarder to `Collection.GetRecordWithID`, kept for backward compatibility (`record.WithID` aliases `dal.RecordWithID`).
- **Dependencies** — existing `dal` session interfaces (`ReadSession`/`WriteSession`), `Key`/`CollectionRef`, and the `update` package. No new external dependency; no `record` import in `dal`.

Data flow (GetData): `id K` → `Key` (handle `CollectionRef` + id + key options) → `GetRecord` → `NewRecordWithData(key, new(T))` → `ReadSession.Get` → return `*new(T)`, mapping the call's not-found error to `(zero T, err)`.

Error handling: not-found is taken from the session call's returned error; a failing key option or an incomplete `.In`/`*ByKey` parent yields a descriptive error; an unsupported `All` surfaces `dal.ErrNotSupported`.

## Not Doing / Out of Scope

- **Generated-id `Insert`** (`Insert(ctx, ws, value, ...InsertOption)`) — needs adapter-side `InsertOption` support (approach C); it is the separate `collection-generated-insert` Feature.
- **Batch** (`ManyInserter[K, T]`/`Item[K, T]`) and **`Count`/`Exists`/`First`** — the separate `typed-collection-extras` Feature.
- **Query filtering** (`Where`/typed query terminal, pagination) — borrow #4/#5, a separate Idea/Feature; `All` reads the whole collection here.
- **Multi-level nesting** (3+ collection segments) — one level of `.In` is in scope.
- **Composite / struct ids in the typed `id K` slot** — `K` is a scalar id type; composite/multi-field keys go through `WithKeyOptions(WithFields(...))` or the `*ByKey` terminals, not a struct `K`.
- **Adapter changes** — none; this Feature is `dal`-only and additive.

## Rehearse Integration

No Rehearse markdown stubs. Every AC has a concrete Go test surface (pure-function construction, in-memory data round-trips, a non-compiling example for the read/write-safety AC, and the existing `dal`/`dalgo2memory`/`end2end` suites for additivity). Those Go tests are authored during implementation and are the canonical verification for this internal library API; markdown stubs would duplicate them without adding signal. (Override available if the team later wants Rehearse scenarios.)

## Open Questions

- **`CollectionName()` receiver convention** — pinned to **value** receiver (so `CollectionOf[string, User]()` works without `*User`); to be documented in the package doc.
- **`All` ↔ future `Where`/`First` seam** — `All` should build its `StructuredQuery` through an internal helper that a later filtering terminal can reuse, so adding `Where`/`First` does not require a redesign.

---
*This document follows the https://specscore.md/feature-specification*
