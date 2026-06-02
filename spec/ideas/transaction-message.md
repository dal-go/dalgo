# Idea: Transaction Message

**Status:** Approved
**Date:** 2026-06-02
**Owner:** alex
**Promotes To:** —
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we let a DALgo transaction carry a human-readable message — set when the transaction starts and/or refined while it runs — so backends can surface it (dalgo2ingitdb as a git commit message, and any backend in its logs)?

## Context

dalgo2ingitdb (ingitdb-cli) maps a DALgo read-write transaction onto a git commit but has no portable way to obtain a human-authored message for that commit. More broadly, any backend benefits from a human-readable annotation on a transaction for logs and observability — e.g. surfacing which module is reading or updating a table — so the need is not write-only. DALgo today exposes only `TxWithName` / `Options().Name()`, documented as a short identifier "useful for mocking transaction in tests": it conflates a test/debug label with a user-facing message and the word "name" implies brevity. dalgo is v0.44.1 (pre-1.0). The only readers of `TxWithName` are ~22 end2end test helpers inside dalgo itself, and nothing reads `Options().Name()`; `dalgo2ingitdb` uses neither — so replacing `name` with `message` is contained.

## Recommended Direction

Treat the message as a single human-readable annotation carried by the transaction's **options**, readable and writable through the existing `Options()` surface, and available on both readonly and read-write transactions.

Concretely: (1) add `TxWithMessage(string) TransactionOption` to seed the message when a transaction starts; (2) add `Message() string` (reader) and `SetMessage(string)` (writer) to the `TransactionOptions` interface — reading is always `tx.Options().Message()`, writing is either the init option or `tx.Options().SetMessage(...)` during execution, against one backing field, so there is no second accessor and no fallback; (3) **remove** `TxWithName` and `Name()` outright — "message" subsumes "name" and doesn't imply brevity. The message is **set/replace only**: the caller composes the full string (including any multi-line body) itself.

Why this lives on `Options` rather than on a new transaction method or optional interface: `TransactionOptions` has a single implementer (`txOptions` in `dal/`), so adding two methods is non-breaking for every adapter — they pass `txOptions` through. It needs no new optional interface and no type assertion at the call site. And because `Options()` is on the base `Transaction` interface, the message is uniformly available to **both readonly and read-write** transactions — which is wanted, since readonly callers use it for logging and observability (e.g. attributing a read to a module).

One implementation requirement follows: a transaction must expose its options by **shared reference** (not a value copy) so a runtime `SetMessage` persists and the backend reads the final value at commit or log time. `dalgo2ingitdb` currently returns its options by value (`return r.opts`); that becomes a reference.

## Alternatives Considered

- **Put `SetMessage`/`GetMessage` on `ReadwriteTransaction`, or on a new optional `dal.TransactionWithMessage` interface.** Rejected — the first breaks every adapter (`dalgo2fs`, `dalgo2memory`, `mocks`, `dalgo2ingitdb`) by adding a required method; the second forces a `tx.(dal.TransactionWithMessage)` type assertion at every call site. Both also scope the message to writes, but readonly transactions want it too for logging.
- **Keep `Name` and add a separate `Message` alongside it.** Rejected — two fields for the same concept. A short label and a message are the same string used for two purposes; the owner's call is that `message` subsumes `name`.
- **Keep `TxWithName`/`Name()` as deprecated aliases for one release.** Rejected — pre-1.0 with only in-tree callers; a clean hard break is simpler, and the one external consumer (`dalgo2ingitdb`) is updated as part of adopting the feature anyway.
- **Append/accumulate semantics (`AddMessage`).** Rejected for MVP — the caller composes the full string; append can be added later without breaking set/replace.

## MVP Scope

In dal-go/dalgo only: add the `TxWithMessage` option; add `Message()` and `SetMessage(string)` to `TransactionOptions`, backed by a renamed `txOptions.message` field exposed by shared reference so `SetMessage` sticks; remove `TxWithName` and `Name()`; migrate the ~22 end2end call sites and replace `TestTxWithName` with `TestTxWithMessage`. Ensure in-tree adapters (`dalgo2fs`, `dalgo2memory`) and `mocks` return options by shared reference so a runtime `SetMessage` is observable via `Options().Message()`. Unit tests: the option seeds the message; `Message()` reads it back; `SetMessage` overwrites and is visible via `Options().Message()` on **both** a readonly and a read-write transaction. Timebox: a single focused dalgo change — wiring the message into a real git commit (`dalgo2ingitdb`) and into backend logs are downstream follow-ups, not in this MVP.

## Not Doing (and Why)

- AddMessage / append accumulation — caller composes the full string; revisit only if multi-write composition becomes a real ask
- Wiring the message into a real git commit in dalgo2ingitdb, or into any backend's logs — downstream consumer work; this Idea only lands the portable surface
- A separate runtime-only interface or type-assertion pattern — unnecessary once the message lives on the single-implementer Options interface
- Per-write or per-record message attribution — transaction-level message only
- Structured message metadata (author, trailers, etc.) — single string; the caller encodes any structure it needs

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | A single transaction-level string serves both consumers: a git commit message (dalgo2ingitdb) and a log/observability annotation (any backend, including readonly). | Trace the ingitdb commit path and a representative log path; confirm one string fits both without structured fields. |
| Must-be-true | A transaction can expose its options by shared reference so a runtime `SetMessage` is visible at commit/log time. | Confirm dalgo2ingitdb and in-tree adapters can return options by pointer; verify `Options().Message()` reflects a prior `SetMessage`. |
| Should-be-true | "name" and "message" are the same concept; no consumer needs both. | grep confirms zero readers of `Options().Name()` outside its definition; reconfirm before deleting. |
| Should-be-true | Exposing `SetMessage` on readonly transactions is useful (logging), not merely harmless. | Confirmed by the owner: readonly callers annotate reads (e.g. module attribution) for logs. |
| Might-be-true | Append / multi-line composition (`AddMessage`) will eventually be wanted. | Defer until a consumer needs to build a message across multiple writes. |

## SpecScore Integration

- **New Features this would create:** `spec/features/transaction-message/` — the `TxWithMessage` option, `TransactionOptions.Message()` / `SetMessage()`, removal of `TxWithName`/`Name()`, the shared-reference `Options()` requirement, and godoc covering the read/write surface, mutability, and the readonly-logging use.
- **Existing Features affected:** none — no existing dalgo Feature references transaction `Name`. Sibling Ideas (`concurrency-capability`, `dalgo-schema-modification`) are orthogonal.
- **Dependencies:** none upstream — additive methods on `Options` plus a contained removal. Downstream consumers (informational; lifecycle tooling manages `Promotes To`): `ingitdb-cli/pkg/dalgo2ingitdb` (git commit message) and any backend wanting transaction annotations in its logs.

## Open Questions

None at this time. Prior questions are resolved with the owner: hard removal of `TxWithName`/`Name()` (no aliases); the message lives on `TransactionOptions` as `Message()` / `SetMessage()`, available to both readonly and read-write transactions.

---
*This document follows the https://specscore.md/idea-specification*
