---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Transaction Message

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/transaction-message?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/transaction-message?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/transaction-message?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/transaction-message?op=request-change) |

**Status:** Approved
**Date:** 2026-06-02
**Owner:** alex
**Source Idea:** [`transaction-message`](../../ideas/transaction-message.md)
**Supersedes:** —

## Summary

Carry a single human-readable message on a DALgo transaction. It can be seeded when the transaction starts (via a functional option) and/or set while the transaction runs (via the transaction's options), and is read back through the existing `Options()` surface. Backends consume it for their own purposes — `dalgo2ingitdb` as a git commit message, any backend as a log/observability annotation — on both readonly and read-write transactions.

## Problem

`dalgo2ingitdb` maps a read-write transaction onto a git commit but has no portable way to obtain a human-authored message for that commit. More broadly, any backend benefits from a human-readable annotation on a transaction for logs (e.g. which module is reading or updating a table), so the need is not write-only. DALgo today offers only `TxWithName` / `Options().Name()`, documented as a short identifier "useful for mocking transaction in tests": it conflates a test/debug label with a user-facing message, and "name" implies brevity. This Feature replaces `name` with a `message` surface that reads naturally for both commit messages and log annotations.

## Behavior

### Seeding the message at start

#### REQ: tx-with-message-option

The `dal` package MUST provide a functional option `TxWithMessage(message string) TransactionOption` that sets the transaction message on the options used to start a transaction. Passing it to `RunReadonlyTransaction`, `RunReadwriteTransaction`, or `NewTransactionOptions` MUST make that message readable via `Message()`.

### Reading and writing the message

#### REQ: options-message-reader

`TransactionOptions` MUST expose `Message() string`, returning the current message, or the empty string when no message has been set.

#### REQ: options-message-setter

`TransactionOptions` MUST expose `SetMessage(message string)`, which replaces the current message. After it is called, `Message()` MUST return the value last written, regardless of whether the prior value came from `TxWithMessage` at start or an earlier `SetMessage`. Semantics are set/replace — the caller composes the full string itself; there is no append.

#### REQ: message-survives-options-roundtrip

The `TransactionOptions` value produced by `NewTransactionOptions` MUST be backed by shared (reference) state: after `SetMessage(x)` on it, `Message()` MUST return `x`, and any holder of that same value — e.g. a transaction whose `Options()` returns it by reference rather than copying — MUST observe `x`. This is what lets a backend read the final message via `tx.Options().Message()` after the worker returns. Wiring each concrete adapter's `Options()` to return this shared value is downstream adoption (see Architecture & Components), not part of this Feature.

### Availability across transaction kinds

#### REQ: message-on-readonly-and-readwrite

The message surface (`Message()` and `SetMessage()`) MUST be available on both readonly and read-write transactions, since `Options()` is defined on the base `Transaction` interface. Setting a message on a readonly transaction MUST be permitted (it is meaningful for logging/observability) and MUST NOT error.

### Removal of the legacy name surface

#### REQ: remove-tx-name

The `dal` package MUST remove `TxWithName` and `TransactionOptions.Name()`. Because `Name()` is declared on the `TransactionOptions` interface, the compiler enforces that **every** in-tree reference is migrated to the message surface or removed; a clean build is the completeness check. Known reference sites, non-exhaustively: the `end2end` `TxWithName(...)` call sites and the `txOptions.Name()` readers in `end2end/end2end_test.go`; and `TestTxWithName`, `mockTx.Name()`, and the `txOptions.Name()` assertions in `dal/transaction_test.go`. This is a deliberate breaking change against dalgo v0.44.1 (pre-1.0). No adapter code requires changes for the removal, because no adapter reads `Name()`.

## Acceptance Criteria

### AC: option-sets-message (verifies REQ: tx-with-message-option)

**Given** options built with `dal.NewTransactionOptions(dal.TxWithMessage("commit subject"))`
**When** `Message()` is called on the resulting `TransactionOptions`
**Then** it returns `"commit subject"`.

### AC: message-empty-by-default (verifies REQ: options-message-reader)

**Given** options built with `dal.NewTransactionOptions()` and no message option
**When** `Message()` is called
**Then** it returns `""`.

### AC: setmessage-overwrites (verifies REQ: options-message-setter)

**Given** options seeded with `dal.TxWithMessage("first")`
**When** `SetMessage("second")` is called on those options
**Then** `Message()` returns `"second"`.

### AC: runtime-message-visible-after-worker (verifies REQ: message-survives-options-roundtrip)

**Given** a transaction whose `Options()` returns by reference the shared `TransactionOptions` it was created with, seeded via `dal.TxWithMessage("init")`
**When** `tx.Options().SetMessage("final")` is called and the worker returns
**Then** a separately obtained `tx.Options().Message()` returns `"final"`, proving `Options()` returns the shared reference rather than a copy.

### AC: message-settable-on-readonly (verifies REQ: message-on-readonly-and-readwrite)

**Given** a readonly transaction
**When** `tx.Options().SetMessage("read by module X")` is called
**Then** the call does not error and `tx.Options().Message()` returns `"read by module X"`.

### AC: name-surface-removed (verifies REQ: remove-tx-name)

**Given** a Go program importing `github.com/dal-go/dalgo/dal`
**When** it references `dal.TxWithName` or calls `Name()` on a `dal.TransactionOptions`
**Then** the program fails to compile because neither symbol exists.

## Architecture & Components

**In scope — the `dal` package only:**

- **`dal.TransactionOption` / `TxWithMessage`** — a new functional option alongside the existing `TxWithReadonly`, `TxWithIsolationLevel`, etc. Sets `txOptions.message`.
- **`dal.TransactionOptions` interface** — gains `Message() string` and `SetMessage(string)`; loses `Name()`. `txOptions` (the single implementer in `dal/`) backs both. For `SetMessage` to persist, `txOptions`'s mutating method uses a pointer receiver and `NewTransactionOptions` returns `*txOptions`, so the value is shared by reference rather than copied.

**Downstream adoption — informational, NOT part of this Feature:**

- For a backend to actually surface the message, its `Transaction.Options()` must return the shared options reference. Today `dalgo2memory.session.Options()` and `dalgo2fs.transaction.Options()` both return `nil`, and external `dalgo2ingitdb` returns `r.opts` by value — each adopts on its own schedule. Removing `Name()` does not force changes in any of them (none read `Name()`); they continue to compile against the new interface. No new interface and no type assertion are introduced at any call site.

## Error Handling & Failure Modes

There are no error paths: `Message()`, `SetMessage()`, and `TxWithMessage` cannot fail. An empty message is the valid "unset" state. Setting a message on a readonly transaction is allowed and simply has no commit effect on backends that only persist messages on write.

## Rehearse Integration

Every AC has a pure-Go surface (functional option, interface methods, compile check) and is verified by standard Go tests, not separate Rehearse scenario files:

- `option-sets-message`, `message-empty-by-default`, `setmessage-overwrites` → table tests in `dal/transaction_test.go` (replacing `TestTxWithName` with `TestTxWithMessage`).
- `runtime-message-visible-after-worker`, `message-settable-on-readonly` → in-package tests in `dal/transaction_test.go` using a mock transaction whose `Options()` returns the shared `*txOptions`, exercising the round-trip on both readonly and read-write kinds.
- `name-surface-removed` → enforced by the compiler across the tree (the migration of `end2end` call sites and `dal/transaction_test.go`); a negative compile test is not added.

Separate `_tests/*.md` Rehearse stubs are intentionally skipped as redundant with these Go tests. The user may request stubs to override this.

## Not Doing / Out of Scope

Carried from the source Idea:

- **Append / `AddMessage` accumulation** — set/replace only; the caller composes the full string. Can be added later without breaking set/replace.
- **Wiring the message into a real git commit (`dalgo2ingitdb`) or into any backend's logs** — downstream consumer work; this Feature only lands the portable surface.
- **A separate runtime-only interface or type-assertion pattern** — unnecessary once the message lives on the single-implementer `Options` interface.
- **Per-write or per-record message attribution** — transaction-level message only.
- **Structured message metadata (author, trailers, etc.)** — single string; the caller encodes any structure it needs.
- **Deprecated aliases for `TxWithName` / `Name()`** — removed outright.
- **Adapter `Options()` adoption** — wiring `dalgo2memory`, `dalgo2fs`, `mocks`, or `dalgo2ingitdb` to carry and return the shared message is downstream adoption, not part of this Feature (see Architecture & Components).

## Assumption Carryover

From the source Idea `transaction-message`:

- **Validated by this Feature's ACs:** a single transaction-level string serves both consumers; a transaction can expose its options by shared reference so a runtime `SetMessage` is visible after the worker returns.
- **Confirmed with the owner:** `name` and `message` are the same concept (no consumer needs both); exposing `SetMessage` on readonly transactions is useful (logging), not merely harmless.
- **Deferred (Might-be-true):** append / multi-line composition (`AddMessage`) — revisited only when a consumer needs to build a message across multiple writes.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
