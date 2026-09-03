---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Triggers and webhooks: change events, transactional outbox, dispatcher

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/triggers?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/triggers?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/triggers?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/triggers?op=request-change) |
**Status:** Draft
**Date:** 2026-09-03
**Owner:** alex
**Source Ideas:** triggers-and-webhooks
**Tracking:** [dal-go/dalgo#134](https://github.com/dal-go/dalgo/issues/134)

## Summary

A `triggers` package beside `dal` turns successful writes into **change
events**, matches them against declarative **triggers** (path pattern +
operations + optional row condition), and delivers **actions** — an in-process
handler or an HMAC-signed **webhook** — with an **at-least-once** guarantee
built from a **transactional outbox**: on adapters with transactions the event
is written to an outbox collection inside the same transaction as the data
write, and a dispatcher drains it. Stores with no native triggers or change
feeds — SQLite, files, inGitDB, the in-memory adapter — get triggers without
native support. Adapters without transactions emit best-effort events and say
so through a capability.

Decided by the founder (2026-09-03): the change-event model, the outbox and
trigger matching live in this module; broker and queue actions live in
separate modules.

The product promise is: **react to any change, on any adapter, with a
guarantee you can name.**

## Problem

DALgo's hooks run before a write (validation) and only in-process. Nothing runs
after a successful write or commit, and nothing leaves the process. Stores with
native change capture expose it in incompatible ways; SQLite, files, inGitDB
and `dalgo2memory` expose nothing. Every consumer therefore rebuilds its own
scheduler and dispatcher, a Git-backed store cannot tell a mirror it changed,
and "call this webhook when a record under `/spaces/*/ext/trackus/**` changes"
is impossible portably.

## Design Principles

- **Atomic with the write.** The outbox record is written in the same
  transaction as the data; commit persists both or neither.
- **At-least-once, honestly.** Delivery may repeat; every delivery carries an
  idempotency key the receiver de-duplicates on. Exactly-once is not promised.
- **Reuse the vocabulary.** Matching uses the access-policies path pattern and
  operation set; `when` uses the `dal.Condition` AST row-level conditions use.
- **Capability, not assumption.** A consumer can ask which guarantee a database
  gives before relying on it.
- **Not a workflow engine.** Triggers say *that* something happened and *who*
  is told. Chains, schedules and inter-step logic belong above DALgo.
- **Governed like data.** The outbox is an ordinary collection under access
  policies and audit selection.

## Behavior

### REQ: change-event

The package MUST define a `Change` carrying: the leaf operation (`Insert`,
`Set`, `Update` or `Delete`), the record key, the post-image (nil for
`Delete`), an optional pre-image, the transaction id, the transaction
`Message()` when one was set, the event time, and the actor (the
`$currentUser` variable of the writing context when present). A multi-record
write MUST produce one `Change` per record. Inside a read-write transaction
events MUST become observable only after the transaction commits; a rolled-back
transaction MUST produce no events.

#### AC-1: one-event-per-record

**Given** a registered trigger on `/notes/**` for every operation
**When** a transaction inserts two notes and updates a third, then commits
**Then** exactly three `Change` events are delivered, one per record, each carrying that record's operation and post-image.

#### AC-2: rollback-emits-nothing

**Given** the same trigger
**When** a transaction writes a note and returns an error
**Then** no event is delivered and the outbox holds no record for it.

### REQ: trigger-definition

A `Trigger` MUST consist of a name, a match (an access-policies path pattern
plus a set of leaf operations), an optional `when` condition over the
post-image (over the pre-image for `Delete`) in the `dal.Condition` AST with
parameters resolved from the writing context, and one action. Registration
MUST validate the pattern, the operations and the condition (through the
shared condition validator) and MUST reject a duplicate name. A trigger whose
`when` is false for an event MUST NOT fire.

#### AC-1: when-filters

**Given** a trigger on `/orders/*` for `Update` with `when: status == 'paid'`
**When** an order is updated to `status: paid` and another to `status: open`
**Then** the trigger fires once, for the paid order.

#### AC-2: invalid-trigger-rejected

**Given** a trigger whose `when` condition compares a constant to a constant
**When** it is registered
**Then** registration fails naming the condition error.

### REQ: transactional-outbox

On an adapter that supports read-write transactions, the framework write
pipeline MUST write one outbox record per matched event into the outbox
collection **inside the same transaction** as the data write. The outbox
collection path MUST be configurable per database and MUST default to
`/_dalgo/outbox`. An outbox record MUST carry the `Change`, the trigger name,
a pending/delivered/dead state, the attempt count, the last error and the
next-attempt time. A write MUST fail if the outbox write fails.

#### AC-1: outbox-survives-crash

**Given** a trigger with a webhook action and a receiver that is down
**When** a transaction commits a matching write and the process stops before dispatching
**Then** after restart the outbox still holds the pending record and dispatching delivers it.

### REQ: dispatcher

`Dispatch(ctx)` MUST process pending outbox records whose next-attempt time
has passed, in insertion order per outbox collection: evaluate the trigger's
action, mark the record delivered on success, and on failure increment the
attempt count and set the next-attempt time by exponential backoff with a
configurable base and cap. After a configurable maximum number of attempts the
record MUST be marked dead with its last error and MUST NOT be retried
automatically. Every delivery MUST include the outbox record id as its
idempotency key. A dispatcher loop MAY be started in-process; `Dispatch` MUST
also be callable explicitly so a CLI or a scheduler can drive it.

#### AC-1: retry-then-deliver

**Given** a webhook receiver that fails twice then succeeds
**When** the dispatcher runs three times past the backoff intervals
**Then** the record is delivered on the third attempt and the receiver saw the same idempotency key each time.

#### AC-2: dead-letter

**Given** a receiver that always fails and a maximum of three attempts
**When** the dispatcher runs four times
**Then** the record is dead after the third attempt with the last error recorded and the fourth run does not contact the receiver.

### REQ: webhook-action

A webhook action MUST POST the event as JSON to its URL with headers
`Content-Type: application/json`, `X-Dalgo-Delivery` (the idempotency key),
`X-Dalgo-Attempt`, `X-Dalgo-Trigger`, and `X-Dalgo-Signature` — a hex
HMAC-SHA256 of the body under a shared secret — within a configurable timeout.
A 2xx response MUST count as delivered; 5xx, 408, 429 and transport errors
MUST be retried; any other 4xx MUST dead-letter the record immediately. The
secret MUST NOT appear in the payload, in logs or in errors.

#### AC-1: signed-delivery

**Given** a webhook action with secret `s`
**When** an event is delivered
**Then** the receiver can verify `X-Dalgo-Signature` against the body with `s`, and the body contains the change and the idempotency key.

#### AC-2: non-retryable-status

**Given** a receiver answering 404
**When** the dispatcher delivers
**Then** the record is dead after one attempt.

### REQ: in-process-action

A handler action MUST be a function receiving the context and the `Change`
and returning an error; a nil return MUST count as delivered and an error MUST
follow the dispatcher's retry rules. On a best-effort database the handler
MUST be invoked after the write returns, in the writing goroutine or a
dedicated one, and its error MUST be reported through the guarantee's error
callback rather than failing the write.

#### AC-1: handler-retried

**Given** a handler that errors once then succeeds
**When** the dispatcher runs twice
**Then** the handler is invoked twice with the same `Change` and the record ends delivered.

### REQ: guarantee-capability

The package MUST expose the delivery guarantee a database gives:
`AtLeastOnce` when the adapter supports transactions and the outbox is
enabled, `BestEffort` otherwise, and `None` when triggers are not enabled. A
consumer MUST be able to read it before registering triggers, and a best-effort
database MUST emit events in-process after each write without persisting them.

#### AC-1: guarantee-reported

**Given** the memory adapter with triggers enabled and a hypothetical adapter without transactions
**When** the guarantee is read for each
**Then** the first reports at-least-once and the second best-effort.

### REQ: governed-outbox

The outbox MUST be an ordinary collection: access policies MUST be able to
allow or deny reading, inserting and truncating it, and audit selection MUST be
able to include or exclude it. The dispatcher MUST run with an explicitly
supplied context policy rather than bypassing enforcement.

#### AC-1: outbox-under-policy

**Given** a secured database whose policy denies `Query` on `/_dalgo/outbox`
**When** a caller other than the dispatcher queries the outbox
**Then** the query is denied, while the dispatcher's context policy allows it.

## Acceptance Criteria

### AC: one-event-per-record (verifies REQ:change-event)

**Given** a registered trigger on `/notes/**` for every operation
**When** a transaction inserts two notes and updates a third, then commits
**Then** exactly three `Change` events are delivered, one per record, each carrying that record's operation and post-image.

### AC: rollback-emits-nothing (verifies REQ:change-event)

**Given** the same trigger
**When** a transaction writes a note and returns an error
**Then** no event is delivered and the outbox holds no record for it.

### AC: when-filters (verifies REQ:trigger-definition)

**Given** a trigger on `/orders/*` for `Update` with `when: status == 'paid'`
**When** an order is updated to `status: paid` and another to `status: open`
**Then** the trigger fires once, for the paid order.

### AC: invalid-trigger-rejected (verifies REQ:trigger-definition)

**Given** a trigger whose `when` condition compares a constant to a constant
**When** it is registered
**Then** registration fails naming the condition error.

### AC: outbox-survives-crash (verifies REQ:transactional-outbox)

**Given** a trigger with a webhook action and a receiver that is down
**When** a transaction commits a matching write and the process stops before dispatching
**Then** after restart the outbox still holds the pending record and dispatching delivers it.

### AC: retry-then-deliver (verifies REQ:dispatcher)

**Given** a webhook receiver that fails twice then succeeds
**When** the dispatcher runs three times past the backoff intervals
**Then** the record is delivered on the third attempt and the receiver saw the same idempotency key each time.

### AC: dead-letter (verifies REQ:dispatcher)

**Given** a receiver that always fails and a maximum of three attempts
**When** the dispatcher runs four times
**Then** the record is dead after the third attempt with the last error recorded and the fourth run does not contact the receiver.

### AC: signed-delivery (verifies REQ:webhook-action)

**Given** a webhook action with secret `s`
**When** an event is delivered
**Then** the receiver can verify `X-Dalgo-Signature` against the body with `s`, and the body contains the change and the idempotency key.

### AC: non-retryable-status (verifies REQ:webhook-action)

**Given** a receiver answering 404
**When** the dispatcher delivers
**Then** the record is dead after one attempt.

### AC: handler-retried (verifies REQ:in-process-action)

**Given** a handler that errors once then succeeds
**When** the dispatcher runs twice
**Then** the handler is invoked twice with the same `Change` and the record ends delivered.

### AC: guarantee-reported (verifies REQ:guarantee-capability)

**Given** the memory adapter with triggers enabled and a hypothetical adapter without transactions
**When** the guarantee is read for each
**Then** the first reports at-least-once and the second best-effort.

### AC: outbox-under-policy (verifies REQ:governed-outbox)

**Given** a secured database whose policy denies `Query` on `/_dalgo/outbox`
**When** a caller other than the dispatcher queries the outbox
**Then** the query is denied, while the dispatcher's context policy allows it.

## Architecture

- **`triggers` package** (sibling of `dal`, like `access`): `Change`,
  `Trigger`, `Registry`, `Outbox` (the record shape and its collection path),
  `Dispatcher` (with backoff and attempt policy), `WebhookAction`,
  `HandlerAction`, `Guarantee`.
- **Pipeline seam in `dal`**: the framework write pipeline gains an
  after-write step that, when a registry is attached to the database, matches
  the write against triggers and, on transactional adapters, writes the outbox
  records through the same transaction; on best-effort adapters it invokes the
  registry after the adapter returns. The seam is the only change to `dal`;
  adapters are not modified.
- **Pre-image capture** reuses the read-before-write step introduced by
  row-level conditions and is performed only when a matched trigger asks for
  the pre-image or the operation is a `Delete`.
- **Condition evaluation** reuses `condeval`; path matching reuses the
  `access` path pattern and operation vocabulary.
- **Webhook signing** uses the standard library (`crypto/hmac`, `net/http`);
  broker and queue actions live in separate modules that implement the action
  interface.

## Error Handling and Failure Modes

- Outbox write fails inside the transaction: the transaction fails; no
  partial event.
- Dispatcher cannot read the outbox (denied, unavailable): `Dispatch` returns
  the error; nothing is marked.
- Action fails: attempt recorded, backoff scheduled; dead after the maximum.
- Non-retryable webhook status: dead immediately with the status recorded.
- Condition evaluation error on an event: the trigger does not fire and the
  error is reported through the registry's error callback; the write is not
  affected.
- Best-effort handler error: reported through the error callback; the write
  succeeds.

## Testing Strategy

- Unit tests for matching, `when` evaluation, backoff arithmetic, state
  transitions and signature computation.
- `dalgo2memory` integration: one event per record, rollback emits nothing,
  outbox written in-transaction, retry and dead-letter sequences with a fake
  clock, crash simulation (dispatch after a fresh registry over the same
  database).
- `httptest` receivers for webhook delivery, signature verification, retryable
  and non-retryable statuses, timeouts.
- end2end conformance cases so adapters prove at-least-once (SQLite) and
  best-effort behaviour.
- 100% statement coverage.

## Out of Scope

- Chained operations, schedules, cron or any workflow semantics.
- Exactly-once delivery.
- Cross-collection or global ordering guarantees.
- Native change-capture adapters (a `ChangeSource` capability is designed for,
  not built).
- Broker and queue actions (separate modules).
- A UI for triggers.

## Open Questions

- Package and collection naming: `triggers` and `/_dalgo/outbox` are the
  working defaults; confirm before implementation.
- Payload versioning for webhooks: an explicit `version` field in the body, or
  the `apiVersion` convention policy documents use?
- Should a trigger be able to ask for the pre-image on `Update` explicitly, or
  is it captured whenever any trigger on the path declares `when` over
  pre-image fields?
