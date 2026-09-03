---
format: https://specscore.md/plan-specification
status: Implemented
---
# Plan: Implement row-level access conditions (read-side MVP)

**Status:** Implemented
**Source Feature:** access-policies/row-level-conditions
**Date:** 2026-09-03
**Owner:** alex
**Supersedes:** —

## Summary

Implement the read side of `access-policies/row-level-conditions`: a parameter
expression in `dal`, a shared record-condition evaluator, `where` conditions on
allow rules with runtime variables, enforcement for `Get`, `Exists` and `Query`
(AND-rewrite), portable documents in DTQL condition syntax with a `param` node,
and the DTQL `param` node itself. Conditions are evaluated against record values
only; no lookups into other collections. The write side (`check`, pre-image and
post-image checks) and path captures are deliberately left to a second plan.

## Approach

Bottom-up, each task independently testable at 100% statement coverage (the
CI gate). Pure pieces first (`dal.Param`, the evaluator), then the policy engine
(rules, precedence-aware residual conditions, variables), then the secured
wrappers, then documents and DTQL, then integration against `dalgo2memory`.

Precedence keeps the parent feature's order: matching rules are walked in
order; conditional allows encountered before the first unconditional rule
combine with OR; an unconditional allow makes the residual moot; an unconditional
deny or default deny leaves the OR as the residual. Residuals from several
policies intersect with AND. The residual is what the wrapper enforces: after
the read for point operations, by query rewrite for `Query`.

Founder decisions applied: a denied conditional `Get` returns `DeniedError`
(the hide option is a follow-up); `dal.Condition` inside, DTQL outside;
read and write levels differ by declaring two rules on one path.

## Tasks

### Task 1: Parameter expression in dal

**Id:** task-1
**Verifies:** access-policies/row-level-conditions#ac:comparison-and-groups
**Depends-On:** —
**Status:** complete

Add `dal.Param` (string form `$<name>`, validated name) as a `FieldRef`/`Constant`
sibling, accepted by `WhereField`, so queries and policy conditions share one
parameter node.

### Task 2: Shared record-condition evaluator

**Id:** task-2
**Verifies:** access-policies/row-level-conditions#ac:comparison-and-groups
**Depends-On:** 1
**Status:** complete

New `condeval` package: `Match` over JSON-normalised record data (comparisons,
`In` incl. array-field overlap, And/Or, dotted field paths, missing field is
false), `Validate` (structural rules, referenced fields and params),
`Substitute` (params to constants/arrays; unresolved is an error), `ToMap`.

### Task 3: Conditional allow rules, variables and residual decisions

**Id:** task-3
**Verifies:** access-policies/row-level-conditions#ac:conditional-deny-rejected, access-policies/row-level-conditions#ac:write-group-excludes-truncate, access-policies/row-level-conditions#ac:missing-variable-denies, access-policies/row-level-conditions#ac:unconditional-suite-unchanged, access-policies/row-level-conditions#ac:no-leak
**Depends-On:** 2
**Status:** complete

`Rule.Where(cond)`; compile validation (allow only, path rules only, explicit
`Truncate` rejected, `write`/`readwrite` drop it); `WithVariables`,
`WithCurrentUser`, built-in `$now`; `Decide` resolves parameters from the context
and returns a residual condition; explanations name rule, slot and condition text
without values.

### Task 4: Read-side enforcement in the secured wrappers

**Id:** task-4
**Verifies:** access-policies/row-level-conditions#ac:get-other-users-record, access-policies/row-level-conditions#ac:tenant-slice, access-policies/row-level-conditions#ac:no-leak
**Depends-On:** 3
**Status:** complete

`Get`/`GetMulti` evaluate the residual after the read and zero the data on
denial; `Exists` upgrades to a read; `Query` is wrapped with the residual ANDed
into `Where()`; residuals from several policies conjoin; queries with joins under
a conditional rule are denied in this slice.

### Task 5: Portable documents and the DTQL param node

**Id:** task-5
**Verifies:** access-policies/row-level-conditions#ac:yaml-roundtrip
**Depends-On:** 3
**Status:** complete

`where` on `DocumentRule` in DTQL condition syntax with `param`; encode and
decode both directions in YAML and JSON; DTQL gains the `param` expression and
its generated schema is refreshed.

### Task 6: Integration against dalgo2memory and coverage gate

**Id:** task-6
**Verifies:** access-policies/row-level-conditions#ac:tenant-slice
**Depends-On:** 4, 5
**Status:** complete

Secured `dalgo2memory` round trips: tenant slice with limit and order, read all
customers, get another user's record denied, exists upgraded; whole module at
100% statement coverage; existing suites unchanged.

## Deferred AC Coverage

- access-policies/row-level-conditions#ac:cannot-take-ownership — write-side
  enforcement (pre-image `where`, post-image `check`) is the next plan; in this
  slice a conditional rule on a write operation denies fail-closed with an
  explanation, so no write is ever granted by an unenforced condition.
- access-policies/row-level-conditions#ac:read-all-edit-assigned — the read
  half (query returns every customer, get succeeds) is exercised by task 6; the
  edit half needs the write side and is deferred with it.
- access-policies/row-level-conditions#ac:capture-in-condition — path captures
  (`{name}` segments bound to `$path.<name>`) are the next plan.

## Open Questions

- Path captures (`{name}` segments to `$path.<name>`) and the write side
  (`check`, pre-image reads) are the next plan; the hide-as-not-found option
  and strict post-verification of query results ride with it.

---
*This document follows the https://specscore.md/plan-specification*
