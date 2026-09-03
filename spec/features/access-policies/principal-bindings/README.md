---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: Principal bindings — policies per user, group and role

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/access-policies/principal-bindings?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/access-policies/principal-bindings?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/access-policies/principal-bindings?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/access-policies/principal-bindings?op=request-change) |
**Status:** Implementing
**Date:** 2026-09-02
**Owner:** alex
**Source Ideas:** row-level-access-conditions
**Tracking:** [dal-go/dalgo#133](https://github.com/dal-go/dalgo/issues/133)

## Summary

Let a policy document bind **rule sets to principals** — `roles`, `groups`,
`users`, and `everyone` — and let the request carry its principal (`id`,
`roles`, `groups`) on the context. The secured wrapper computes the request's
**effective principal policy** as the union of every rule set the principal is
bound to, compiled as one policy so the parent feature's precedence resolves
overlaps, and then intersects that single policy with database-bound policies
exactly as today. Additive within a principal, monotonic across layers.

The product promise is: **declare once what a role, a group or a user may do,
in the same document as the paths and rows, and DALgo derives the caller's
authority from who they are.**

## Problem

Access policies attach to a database handle or to a context; nothing relates
them to *who is calling*. A host that wants "members may read, admins may
write, the owner may delete their own" must build the policy per request in
code, choosing rule sets by role by hand — which is the same per-call-site
logic that access policies were meant to replace. Directus attaches reusable
policies to roles or users and unions them; Firestore rules read the caller's
claims. DALgo has no principal at all.

## Design Principles

- **Identity-agnostic.** DALgo never looks a user up; the host puts a principal
  on the context. Roles and groups are opaque strings.
- **Additive within the principal, monotonic across layers.** Bindings union;
  layers intersect. A binding can never widen a database-level denial.
- **One policy, one precedence.** The union is compiled as a single policy so
  most-specific-wins and deny-on-tie resolve overlaps between bindings without
  a new rule.
- **Same document, same codec.** Bindings live in the versioned YAML/JSON policy
  model beside paths, conditions and fields.

## Behavior

### REQ: principal-on-context

The package MUST provide `access.WithPrincipal(ctx, Principal)` where
`Principal` carries an `ID`, a set of `Roles` and a set of `Groups`, all opaque
strings, and `access.PrincipalFrom(ctx)`. The principal's `ID` MUST be what
`$currentUser` resolves to; `$principal.roles` and `$principal.groups` MUST be
available as array-valued variables in conditions.

#### AC-1: principal-drives-current-user

**Given** a context carrying principal `u1` with role `member`
**When** a rule conditioned on `ownerID == $currentUser` is evaluated
**Then** the condition compares against `u1`.

### REQ: bindings-document

A policy document MUST accept a `bindings` section mapping `roles`, `groups`
and `users` to named rule sets, plus an `everyone` rule set that applies to any
principal, including an absent one. Rule sets MUST use the same rule shape as
the parent feature (scopes, operations, effect) including `where`/`check` and
`fields` when those features are present. A binding MAY reference a rule set
by name so several roles share one.

#### AC-1: shared-rule-set

**Given** a document whose `editor` and `admin` roles both reference rule set `content-write`
**When** principals with either role write under the rule set's paths
**Then** both are allowed by the same rule id.

### REQ: effective-policy-union

For a request, the wrapper MUST collect every rule set bound to the principal's
`ID`, each of its `Roles`, each of its `Groups`, and `everyone`, and MUST
compile them into **one** policy for evaluation. Within that policy the parent
feature's precedence MUST apply unchanged: greatest depth, then literal
specificity, then deny on tie, declaration order irrelevant. A rule from one
binding MAY therefore be overridden by a more specific rule from another
binding, and an equally specific deny from any binding MUST win.

#### AC-1: roles-union

**Given** role `reader` allowing `Get` under `/docs/**` and role `writer` allowing `Insert` there
**When** a principal with both roles gets and inserts under `/docs/`
**Then** both operations are allowed.

#### AC-2: specific-deny-across-bindings

**Given** role `member` allowing `ReadWrite` under `/spaces/*/**` and group `suspended` denying `Write` under `/spaces/*/**`
**When** a suspended member writes under a space
**Then** the write is denied (equal specificity, deny wins).

### REQ: layer-intersection

The effective principal policy MUST be treated as one more policy in the parent
feature's intersection with database-bound and context-bound policies. A binding
MUST NOT reopen a resource a database policy denies. When no binding applies and
no `everyone` rule set exists, the principal policy MUST deny everything
(default deny).

#### AC-1: binding-cannot-widen

**Given** a database policy denying `Delete` under `/audit/**` and an `admin` role binding allowing `ReadWrite` there
**When** an admin deletes under `/audit/`
**Then** the delete is denied.

#### AC-2: unbound-principal-denied

**Given** a document with no `everyone` rule set and a principal with no bound role, group or user entry
**When** any operation is evaluated
**Then** it is denied with an explanation stating that no binding applies.

### REQ: explanations-name-binding

A decision produced through bindings MUST name the binding kind and value
(`role:admin`, `group:staff`, `user:u1`, `everyone`) alongside the policy and
rule ids.

#### AC-1: binding-in-explanation

**Given** a denial produced by a rule bound through group `suspended`
**When** the `DeniedError` is formatted
**Then** the text contains `group:suspended` and the rule id.

## Acceptance Criteria

### AC: principal-drives-current-user (verifies REQ:principal-on-context)

**Given** a context carrying principal `u1` with role `member`
**When** a rule conditioned on `ownerID == $currentUser` is evaluated
**Then** the condition compares against `u1`.

### AC: shared-rule-set (verifies REQ:bindings-document)

**Given** a document whose `editor` and `admin` roles both reference rule set `content-write`
**When** principals with either role write under the rule set's paths
**Then** both are allowed by the same rule id.

### AC: roles-union (verifies REQ:effective-policy-union)

**Given** role `reader` allowing `Get` under `/docs/**` and role `writer` allowing `Insert` there
**When** a principal with both roles gets and inserts under `/docs/`
**Then** both operations are allowed.

### AC: binding-cannot-widen (verifies REQ:layer-intersection)

**Given** a database policy denying `Delete` under `/audit/**` and an `admin` role binding allowing `ReadWrite` there
**When** an admin deletes under `/audit/`
**Then** the delete is denied.

### AC: binding-in-explanation (verifies REQ:explanations-name-binding)

**Given** a denial produced by a rule bound through group `suspended`
**When** the `DeniedError` is formatted
**Then** the text contains `group:suspended` and the rule id.

## Architecture

- `access.Principal` and the context helpers live in the `access` package.
- The document model gains `ruleSets` (named) and `bindings` (`roles`, `groups`,
  `users`, `everyone` → rule-set names). Codecs extend the versioned model.
- A `PrincipalPolicySet` compiles the document once; per request it selects the
  bound rule sets and compiles their union into an `AccessPolicy`, cached by
  the sorted tuple of binding keys so repeated requests by the same role
  combination do not recompile.
- The secured wrapper treats the derived policy as one more entry in its
  intersection; no change to the evaluator.

No adapter is modified.

## Error Handling and Failure Modes

- Binding references an unknown rule set: document error at load.
- No binding applies and no `everyone`: deny with an explanation naming the principal id (never its roles' contents beyond names).
- Principal absent from context and document has an `everyone` rule set: only `everyone` applies.
- Conflicting equally specific rules across bindings: deny wins, as in the parent.

## Testing Strategy

- Document round-trip fixtures with rule sets and bindings in YAML and JSON.
- Union/precedence table tests across two, three and many bindings, reversed declaration order.
- Intersection tests proving no widening against database and context policies.
- Cache tests: identical binding tuples reuse one compiled policy; different tuples do not.
- The parent feature's full suite re-run with a bindings document producing the same effective policy.

## Out of Scope

- Storing or resolving memberships; the host supplies the principal.
- Role hierarchies or inheritance between roles (a role is a flat name).
- Per-principal audit selection (audit policies remain path-only).
- Time-bound or conditional bindings ("admin until Friday").

## Open Questions

- Should role and group names accept wildcard patterns in bindings (`role:space-*`)? Recommendation: not in the first slice; exact names only.
- Should `everyone` be able to grant, or only deny, when a principal is absent? Recommendation: it may grant; anonymous read of public paths is a common need.
