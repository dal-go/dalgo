# Plan: Transaction Message

**Status:** Approved
**Source Feature:** transaction-message
**Date:** 2026-06-02
**Owner:** alex
**Supersedes:** —

## Summary

Decomposes the `transaction-message` Feature into three linear tasks: land the additive `message` surface on `dal` transaction options, perform the coupled breaking removal of the legacy `Name` surface with its call-site migration, then add the shared-reference round-trip tests across readonly and read-write transactions. All six acceptance criteria are covered; none are deferred.

## Approach

Order is forced by the compiler. Task 1 is purely additive (option, interface methods, shared-reference `NewTransactionOptions`) and unblocks everything. Task 2 is the only breaking change and must land as one unit — removing `Name()` and migrating every reference together, since the tree will not build otherwise — and it depends on Task 1 because the migration targets (`TxWithMessage`, `Message()`) must already exist. Task 3 adds the behaviour tests that exercise the shared-reference guarantee on both transaction kinds and depends on Task 1's `*txOptions` return. Each task is one focused work session.

## Tasks

### Task 1: Add the message surface to dal transaction options

**Verifies:** transaction-message#ac:option-sets-message, transaction-message#ac:message-empty-by-default, transaction-message#ac:setmessage-overwrites

Add a `message` field to `txOptions`; add `TxWithMessage(message string) TransactionOption`; add `Message() string` (reader) and a pointer-receiver `SetMessage(message string)` to both the `TransactionOptions` interface and `txOptions`; change `NewTransactionOptions` to return `*txOptions` so the value is shared by reference rather than copied. Because the concrete return type changes, update any surviving value-type assertions on the options (e.g. `opts.(txOptions)`) to `*txOptions`. Add table tests in `dal/transaction_test.go` covering the seeded option, the empty-string default, and `SetMessage` overwrite.

### Task 2: Remove TxWithName / Name() and migrate all references

**Verifies:** transaction-message#ac:name-surface-removed

Delete `TxWithName` and `TransactionOptions.Name()` (interface declaration and `txOptions` implementation). Migrate every in-tree reference so the tree builds cleanly: the `end2end` `TxWithName(...)` call sites become `TxWithMessage(...)`, and the `txOptions.Name()` readers at `end2end/end2end_test.go:125` and `:182` become `Message()` — **preserving the exact seeded strings**, because the surrounding `switch txName` blocks dispatch on those literal values, so the rename stays green only if the strings are unchanged. In `dal/transaction_test.go`, rename `TestTxWithName` → `TestTxWithMessage` and update/remove its `txOptions.Name()` assertions; the standalone `mockTx.Name()` test-double method is not part of the removed interface surface — remove it as incidental dead-code cleanup, not as a build requirement. A clean `go build ./... && go test ./...` is the completeness check. Depends on Task 1, whose `TxWithMessage`/`Message()` are the migration targets.

### Task 3: Verify shared-reference round-trip on readonly and read-write transactions

**Verifies:** transaction-message#ac:runtime-message-visible-after-worker, transaction-message#ac:message-settable-on-readonly

In `dal/transaction_test.go`, give the mock transaction an `Options()` that returns the shared `*txOptions` it was constructed with. Add tests that seed the message via `TxWithMessage("init")`, call `tx.Options().SetMessage("final")`, and assert a separately obtained `tx.Options().Message()` returns `"final"` — proving `Options()` returns the shared reference, not a copy — exercised on both a read-write and a readonly transaction, and asserting that setting a message on a readonly transaction does not error. Depends on Task 1's shared-reference `NewTransactionOptions`.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
