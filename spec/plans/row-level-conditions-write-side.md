---
format: https://specscore.md/plan-specification
status: Implemented
---
# Plan: Row-level access conditions — write side

**Status:** Implemented
**Source Feature:** access-policies/row-level-conditions
**Date:** 2026-09-03
**Owner:** alex
**Supersedes:** —

## Summary

Second slice of `access-policies/row-level-conditions`, after the read-side
MVP ([`row-level-conditions-mvp`](row-level-conditions-mvp.md)): enforce row
conditions on writes. A rule gains `check` (the post-image condition, defaulting
to `where`); the secured write session reads the pre-image inside the caller's
transaction, computes the post-image (new data for Insert and Set, pre-image
plus updates for Update), and admits or refuses each write before anything
reaches the adapter. This lifts the read-side slice's fail-closed refusal of
conditional writes, so "read all customers, edit only the ones assigned to me"
works end to end.

## Approach

Rule selection follows the same precedence walk as reads: matching rules in
order, conditional allows first, then the first unconditional rule. For a write
the *first alternative whose `where` holds on the pre-image* decides the row and
its `check` must hold on the post-image; a new row (Insert, or Set of a missing
row) is admitted by the first alternative whose `check` it satisfies; when no
alternative applies, the terminal unconditional allow admits the write, subject
to its own `check` if it declares one. Deletes need only `where`. Every policy
in the intersection is evaluated; any refusal wins. Batches are evaluated in
full before any write is delegated.

The pre-image is read into a private map through the transaction's own read
session; a write session that cannot read refuses conditional writes. The
post-image for Update is computed with the shared `condeval.ApplyUpdates`, the
same field-path semantics the in-memory adapter applies.

## Tasks

### Task 1: Shared update applier

**Id:** task-1
**Verifies:** access-policies/row-level-conditions#ac:comparison-and-groups
**Depends-On:** —
**Status:** complete

`condeval.ApplyUpdates` and `condeval.CloneMap`: field-name and field-path
sets, `update.DeleteField`, intermediate maps created, non-map intermediates and
non-serialisable values refused.

### Task 2: Check on rules and write residuals in decisions

**Id:** task-2
**Verifies:** access-policies/row-level-conditions#ac:check-defaults-to-where, access-policies/row-level-conditions#ac:conditional-deny-rejected
**Depends-On:** 1
**Status:** complete

`Rule.Check(cond)`; compile validation shared with `where`; `Decision.Writes`
carries per-resource `WriteResidual` (ordered alternatives plus terminal allow),
resolved once per request; unresolved parameters deny.

### Task 3: Write enforcement in the secured write session

**Id:** task-3
**Verifies:** access-policies/row-level-conditions#ac:cannot-take-ownership, access-policies/row-level-conditions#ac:update-post-image, access-policies/row-level-conditions#ac:batch-all-or-nothing, access-policies/row-level-conditions#ac:read-all-edit-assigned
**Depends-On:** 2
**Status:** complete

Pre-image read inside the transaction, post-image computation, alternative
selection per operation (Insert, Set, Update, UpdateRecord, UpdateMulti,
Delete, multi variants), batches refused whole, explanations without values.

### Task 4: Portable documents

**Id:** task-4
**Verifies:** access-policies/row-level-conditions#ac:yaml-roundtrip
**Depends-On:** 2
**Status:** complete

`check` on `DocumentRule`, YAML and JSON, both directions.

### Task 5: Integration and conformance

**Id:** task-5
**Verifies:** access-policies/row-level-conditions#ac:read-all-edit-assigned
**Depends-On:** 3, 4
**Status:** complete

The customers example end to end on `dalgo2memory` (read all, edit only
assigned, cannot take ownership, cannot reassign away); end2end write
sub-tests for adapters; 100% statement coverage.

## Deferred AC Coverage

- access-policies/row-level-conditions#ac:capture-in-condition — path captures
  are the next plan.
- access-policies/row-level-conditions#ac:unknown-capture-rejected — path captures,
  verified by [`row-level-conditions-path-captures`](row-level-conditions-path-captures.md).
- access-policies/row-level-conditions#ac:comparison-and-groups — read-side criterion
  already verified by [`row-level-conditions-mvp`](row-level-conditions-mvp.md);
  behaviour unchanged here, suites re-run green.
- access-policies/row-level-conditions#ac:conditional-deny-rejected — read-side criterion
  already verified by [`row-level-conditions-mvp`](row-level-conditions-mvp.md);
  behaviour unchanged here, suites re-run green.
- access-policies/row-level-conditions#ac:write-group-excludes-truncate — read-side criterion
  already verified by [`row-level-conditions-mvp`](row-level-conditions-mvp.md);
  behaviour unchanged here, suites re-run green.
- access-policies/row-level-conditions#ac:missing-variable-denies — read-side criterion
  already verified by [`row-level-conditions-mvp`](row-level-conditions-mvp.md);
  behaviour unchanged here, suites re-run green.
- access-policies/row-level-conditions#ac:get-other-users-record — read-side criterion
  already verified by [`row-level-conditions-mvp`](row-level-conditions-mvp.md);
  behaviour unchanged here, suites re-run green.
- access-policies/row-level-conditions#ac:tenant-slice — read-side criterion
  already verified by [`row-level-conditions-mvp`](row-level-conditions-mvp.md);
  behaviour unchanged here, suites re-run green.
- access-policies/row-level-conditions#ac:unconditional-suite-unchanged — read-side criterion
  already verified by [`row-level-conditions-mvp`](row-level-conditions-mvp.md);
  behaviour unchanged here, suites re-run green.
- access-policies/row-level-conditions#ac:yaml-roundtrip — read-side criterion
  already verified by [`row-level-conditions-mvp`](row-level-conditions-mvp.md);
  behaviour unchanged here, suites re-run green.
- access-policies/row-level-conditions#ac:no-leak — read-side criterion
  already verified by [`row-level-conditions-mvp`](row-level-conditions-mvp.md);
  behaviour unchanged here, suites re-run green.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
