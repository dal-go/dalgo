---
format: https://specscore.md/idea-specification
status: Specifying
---
# Idea: Triggers and webhooks for trigger-less stores

**Status:** Specifying
**Date:** 2026-09-02
**Owner:** alex
**Promotes To:** triggers
**Supersedes:** —
**Related Ideas:** extends:transaction-message, depends_on:row-level-access-conditions

## Problem Statement

How might we let an application react to data changes — run a handler, call a
webhook — portably across every DALgo adapter, including stores that have no
native triggers or change feeds (SQLite, files, inGitDB, the in-memory
adapter), with a delivery guarantee an application can reason about?

## Context

DALgo's hooks run **before** a write: `AddBeforeSaveHook` and
`AddBeforeDeleteHook` are process-wide validation points in the framework write
pipeline (`dal/hooks.go`, `dal/write_pipeline.go`). Nothing runs **after** a
successful write or commit, and nothing leaves the process. Stores with native
change capture (Firestore listeners, PostgreSQL `LISTEN/NOTIFY` and logical
replication) expose it in incompatible ways; SQLite, plain files, inGitDB and
`dalgo2memory` expose nothing at all.

So every consumer rebuilds the same thing above DALgo: the Sneat products each
run their own schedulers and dispatchers for reminders, renewals and settlement
timers; a Git-backed store cannot tell a mirror it changed; a reporting tool
cannot "watch this query". The founder's framing (2026-09-02): *"we can
introduce triggers and webhooks in dalgo so people can have triggers on
trigger-less DBs (like SQLite, files, etc.)"*, and the open question of where
triggers should live at all. Tracked as
[dal-go/dalgo#134](https://github.com/dal-go/dalgo/issues/134).

Two shipped and specified pieces make this cheaper than it looks. The
[transaction message](transaction-message.md) gives every change a
human-readable reason. The access-policies matcher already expresses "which
paths and operations" and, with
[row-level conditions](row-level-access-conditions.md), "which rows" — a trigger
needs exactly those two things plus an action.

## Recommended Direction

**A `triggers` package beside `dal` — not inside `dal.DB` — that turns
successful writes into change events, matches them against declarative
triggers, and delivers actions with an at-least-once guarantee built from a
transactional outbox, so trigger-less stores get triggers without native
support.**

- **Change event.** Emitted by the framework write pipeline after a successful
  write (or, inside a transaction, after commit): operation, key, optional
  pre-image, post-image, transaction id, the transaction `Message()`, timestamp
  and actor (from context variables). One event per record, batches fan out.
- **Trigger = match + when + action.** *Match* reuses the access-policies path
  pattern and operation vocabulary (`/spaces/*/ext/trackus/**`, `Insert|Update`).
  *When* is an optional `dal.Condition` over the post-image (or pre-image for
  deletes), the same AST row-level conditions use. *Action* is an in-process
  handler or a **webhook**: HTTP POST of the event, HMAC-signed, with timeout,
  exponential-backoff retries and a dead-letter state after N attempts.
- **Delivery on trigger-less stores = transactional outbox.** When the adapter
  supports transactions, the pipeline writes the event as a record into an
  outbox collection **inside the same transaction** as the data write, so
  commit persists both or neither. A dispatcher — an in-process loop or an
  explicit `Dispatch(ctx)` call a CLI can drive — reads pending outbox records,
  evaluates triggers, delivers, and marks them delivered. At-least-once; the
  outbox record id is the idempotency key the receiver de-duplicates on.
- **Capability, not assumption.** Adapters without transactions emit
  best-effort in-process events and report `Guarantee: best-effort`; adapters
  with native change capture may later implement a `ChangeSource` capability
  that feeds the same trigger model, replacing the outbox as the event source.
- **Boundaries.** Triggers decide *that* something happened and *who* is told;
  they are not a workflow engine. Chained operations, conditions between steps
  and schedules belong to the consumer (an automation core above DALgo), which
  subscribes to these events.
- **Governed like data.** The outbox is an ordinary collection: access policies
  decide who may read or truncate it, and the audit selector can include it.

## Alternatives Considered

- **Extend the existing before-hooks into after-hooks and stop there.** In-process
  only, no persistence, lost on crash, no webhooks — the exact thing a
  trigger-less store cannot offer and the reason applications rebuild
  dispatchers. After-hooks are a useful degenerate case of the design, not the
  design.
- **Rely on each store's native change feed** (Firestore listeners, PostgreSQL
  replication) and give up on the others. Leaves SQLite, files, inGitDB and
  the test adapter — the stores DALgo exists to make interchangeable — without
  triggers, and gives every consumer three integration codepaths.
- **Put the whole thing in a separate module (`dalgo-triggers`).** Keeps core
  lean, but the outbox write must happen inside the framework write pipeline to
  be atomic with the data write, and that pipeline is in `dal`. The split that
  works is: event model, outbox and matching in `dalgo`; the webhook action's
  HTTP client is standard library; queue or broker actions live in separate
  modules.
- **Polling the store for changes instead of an outbox.** Works nowhere
  generically (no change timestamps guaranteed), misses deletes, and scans
  whole collections.

## MVP Scope

Change events for `Insert`, `Set`, `Update` and `Delete` emitted by the write
pipeline; the outbox collection written in-transaction on transactional
adapters; a dispatcher with at-least-once delivery, retries, backoff and
dead-letter; two actions — in-process handler and signed webhook; trigger match
by path pattern and operation with an optional `when` condition; a
`Guarantee` capability flag; conformance on `dalgo2memory` and SQLite
(`dalgo2sql`), proven by a webhook receiver that observes exactly the committed
changes after a simulated crash between commit and delivery. Timebox: one
release after the row-level-conditions feature lands.

## Not Doing (and Why)

- A workflow or automation engine (operation chains, schedules, cron) — that is
  the consumer's layer; triggers emit and deliver, nothing more
- Exactly-once delivery — at-least-once plus an idempotency key is the honest
  contract an outbox can keep; exactly-once needs receiver cooperation anyway
- Cross-collection ordering guarantees — per-outbox order only; global ordering
  is a broker's job
- Native change-capture adapters in the MVP — the `ChangeSource` capability is
  designed for, not built, until a consumer needs lower latency than the
  outbox gives
- A UI for triggers — declarative YAML alongside access policies is the
  authoring surface; anything visual belongs to a product

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | The outbox record can be written inside the same transaction as the data write on every transactional adapter, from the framework pipeline, without adapter changes. | Prototype on `dalgo2memory` and SQLite via `dalgo2sql`; kill the process between commit and dispatch and confirm the event survives and is delivered on restart. |
| Must-be-true | Reusing the access-policies path matcher and the `dal.Condition` AST for trigger matching needs no new grammar. | Author the Sneat reminder and the inGitDB-mirror triggers in YAML with the existing vocabulary; anything they cannot express is a design gap, not a grammar request. |
| Must-be-true | Emitting events from the write pipeline does not change write latency measurably on stores without an outbox and stays within one extra write on stores with one. | Benchmark the pipeline with triggers disabled, enabled-no-match, and enabled-with-outbox on SQLite and memory. |
| Should-be-true | Capturing a pre-image for `Update`/`Delete` can share the read-before-write mechanism row-level conditions need, so the cost is paid once. | Implement pre-image capture as one pipeline step consumed by both features. |
| Should-be-true | A signed webhook with retries and a dead-letter state is enough for the first consumers; no broker is needed. | Run the Sneat automation-core prototype and an inGitDB post-commit sync on webhooks only for a month of fixture traffic. |
| Might-be-true | Adapters with native change capture will want to replace the outbox rather than complement it. | Defer; keep `ChangeSource` as an optional capability behind the same trigger model. |

## SpecScore Integration

- **New Features this would create:** a `triggers` umbrella (change events, outbox and dispatcher, trigger matching, webhook action, guarantee capability) — decomposed at specify time
- **Existing Features affected:** [transaction-message](../features/transaction-message/README.md) (message carried on the event), [access-policies](../features/access-policies/README.md) (matcher reuse; outbox governed as a collection), the framework write pipeline
- **Dependencies:** [row-level-access-conditions](row-level-access-conditions.md) for the condition/parameter node and the shared pre-image read; adapters advertise transaction support already

## Open Questions

- Where exactly should triggers live? *Decided 2026-09-03 (founder):* the change-event
  model, the transactional outbox and trigger matching live in this module (a
  `triggers` package beside `dal`), because the outbox write must be atomic with
  the data write inside the framework write pipeline; the webhook action uses the
  standard library; broker and queue actions live in separate modules.
- Package and collection naming: `triggers` vs `events` vs `cdc`; the default
  outbox path (`/_dalgo/outbox/*`?) and whether it is configurable per database.
- Should the pre-image be captured by default for `Update` and `Delete`, or only
  when a trigger or a row-level condition needs it? Recommendation: only when
  needed, decided at pipeline time from the registered triggers and policies.
- What is the minimum webhook contract (payload shape, signature header, retry
  schedule) receivers can rely on, and is it versioned like policy documents?
