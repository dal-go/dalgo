---
format: https://specscore.md/plan-specification
status: Draft
---
# Plan: Typed Collection

**Status:** Approved
**Source Feature:** typed-collection
**Date:** 2026-06-07
**Owner:** alexandertrakhimenok
**Supersedes:** —

## Summary

Implements the `typed-collection` Feature: a session-less generic `dal.Collection[T]` handle in package `dal` with point-CRUD terminals (`Get`/`All`/`InsertWithID`/`Set`/`Update`/`Delete`) and one-level `.In` nesting, additive over the existing `dal` session interfaces and `CollectionRef`. Verified with unit tests against the in-memory adapter (`dalgo2memory`) and by keeping the existing suites green.

## Approach

Build bottom-up in package `dal`: first the interface, constructors, and session-less handle (which also establishes the read/write session typing), then the shared `id any` → `*Key` resolution, then read terminals, then write terminals, then nesting, then a final additivity gate. Each step is unit-tested against `dalgo2memory`. The order respects inferable dependencies — every terminal relies on the interface (Task 1) and id resolution (Task 2), nesting exercises `Get` (Task 3), and the additivity gate runs last once all terminals exist. Linear, no parallel branches.

## Tasks

### Task 1: Collection[T] interface, CollectionNamer, constructors

**Verifies:** typed-collection#ac:construct-by-convention, typed-collection#ac:construct-by-explicit-name, typed-collection#ac:write-needs-write-session
**Depends-On:** —
**Status:** done

Define `dal.Collection[T]` (read methods take `ReadSession`, write methods take `WriteSession`), the `CollectionNamer` interface, the unexported impl composing a `dal.CollectionRef`, and the `CollectionOf[T CollectionNamer]()` / `CollectionAt[T any](name)` constructors. Add a negative-compile example proving a write terminal rejects a plain `DB`.

### Task 2: id-argument resolution

**Verifies:** typed-collection#ac:id-plain-or-keyoption
**Depends-On:** 1
**Status:** done

Internal helper turning the `id any` argument into a `*dal.Key` from the handle's `CollectionRef`: a plain value sets `Key.ID`; a `dal.KeyOption` (`WithID`/`WithFields`) is applied via `NewKeyWithOptions`. Shared by every terminal.

### Task 3: Get terminal with not-found mapping

**Verifies:** typed-collection#ac:typed-get-roundtrip, typed-collection#ac:get-not-found
**Depends-On:** 2
**Status:** done

Implement `Get(ctx, ReadSession, id) (T, error)` by wrapping `new(T)` in a record and calling the session getter, returning the decoded value. Map the session call's not-found error to `(zero T, err)` without consulting `record.Error()`/`record.Exists()`.

### Task 4: All terminal

**Verifies:** typed-collection#ac:typed-all-distinct, typed-collection#ac:all-unsupported-surfaces-error
**Depends-On:** 2
**Status:** done

Implement `All(ctx, ReadSession) ([]T, error)` building a `StructuredQuery` over the handle's `CollectionRef` with a per-row `new(T)` factory so results never alias; surface `dal.ErrNotSupported` from backends that cannot run the query.

### Task 5: InsertWithID and Set terminals

**Verifies:** typed-collection#ac:insert-with-id-returns-key, typed-collection#ac:set-upserts
**Depends-On:** 2
**Status:** done

Implement `InsertWithID(ctx, WriteSession, id, value) (*dal.Key, error)` and `Set(ctx, WriteSession, id, value) error`, delegating to the session `Inserter`/`Setter` with a record built from the resolved key; `InsertWithID` returns that key.

### Task 6: Update and Delete terminals

**Verifies:** typed-collection#ac:update-applies-fields, typed-collection#ac:delete-removes
**Depends-On:** 2
**Status:** done

Implement `Update(ctx, WriteSession, id, updates []update.Update, preconditions ...dal.Precondition) error` and `Delete(ctx, WriteSession, id) error`, delegating to the session `Updater`/`Deleter` with the resolved key.

### Task 7: .In subcollection nesting

**Verifies:** typed-collection#ac:nested-get, typed-collection#ac:nested-incomplete-parent-errors
**Depends-On:** 3
**Status:** done

Implement `In(parent *dal.Key) dal.Collection[T]` composing the `CollectionRef` parent chain (one level). A terminal on a handle scoped under an incomplete parent key returns a descriptive error rather than panicking.

### Task 8: Additivity verification

**Verifies:** typed-collection#ac:additive-suite-stays-green
**Depends-On:** 7
**Status:** pending

Confirm the layer is additive: run the full pre-existing `dal` + `dalgo2memory` + `end2end` suites and keep them green with no existing call-site changes, and add a test/assertion that `dal` still does not import the `record` package.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
