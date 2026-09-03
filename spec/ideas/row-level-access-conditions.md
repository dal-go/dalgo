---
format: https://specscore.md/idea-specification
status: Specifying
---
# Idea: Declarative per-principal policies with row-level conditions, path captures and field patterns

**Status:** Specifying
**Date:** 2026-09-02
**Owner:** alex
**Promotes To:** access-policies/row-level-conditions, access-policies/principal-bindings, access-policies/field-patterns
**Supersedes:** —
**Related Ideas:** extends:access-policies

## Problem Statement

How might we let people declare, in one portable document, what a user, a
group or a role may do — down to *which rows* ("records where `createdBy` is
the current user", "orders where `tenantID` is the caller's tenant") and *which
fields* — using wildcard paths and DTQL conditions the way Firestore security
rules read, so that ownership, tenant isolation and field exposure are enforced
once across every DALgo adapter, and ad-hoc reporting built on DALgo can be
granted a safe slice of a collection without a data-model change?

## Context

[Access policies](access-policies.md) are **path-scoped**: a rule allows or
denies an operation on a structural path such as `/spaces/*/ext/trackus/**`.
They cannot express a predicate over the record, cannot restrict fields, and
have no notion of *who* — a policy is attached to a database handle or a
context, never to a role or a user. The original idea reserved the room
deliberately ("field, predicate, projection, index, and cost constraints"). That
room is still empty: nothing in the `access` package evaluates a record's fields
(verified against `access/*.go` on 2026-09-02).

The consequence is that every application re-implements row and field rules in
code, per call site and per adapter, and the same class of bug recurs: an
[`AdminOnly` check missing on one endpoint](https://github.com/sneat-co/backstage/blob/main/docs/roadmaps/ecosystem-roadmap-2026-H2.md)
grants a whole collection.

Two systems prove the shape. **Firestore security rules** match paths with
wildcards and named captures (`/spaces/{spaceID}/...`), grant per operation
(`get`, `list`, `create`, `update`, `delete`), and test `resource.data` (the
existing row) and `request.resource.data` (the row after the write) against
`request.auth`. **Directus** attaches reusable policies to roles *or* users,
unions them, and filters with runtime variables (`$CURRENT_USER`); PostgreSQL
row-level security names the same two slots `USING` and `WITH CHECK`. None of
them is portable across DALgo's adapters or present in the in-memory test
adapter.

The founder's direction (2026-09-02): row-level conditions are "super useful for
DataTug and any ad-hoc reporting built on top of dalgo"; **internally use
`dal.Condition`, but users should declare policies declaratively, per
user/group/role, using DTQL**; add wildcard patterns for tables and fields;
"think Firestore rules". Tracked as
[dal-go/dalgo#133](https://github.com/dal-go/dalgo/issues/133).

## Recommended Direction

**One declarative policy document — YAML, conditions in DTQL, bound to
principals — compiled to the existing `access` model plus `dal.Condition`, and
enforced by the secured wrapper in the way each operation needs.** Three parts,
each its own child feature of access-policies, shipped in this order.

**1. Row-level conditions (`access-policies/row-level-conditions`).** An allow
rule may carry `where` (the rows it applies to — `resource.data` in Firestore
terms) and `check` (the shape a written row must have afterwards —
`request.resource.data`; defaults to `where`). Both are `dal.Condition` trees:
`Comparison` and And/Or groups, the same AST `dal.StructuredQuery` uses and DTQL
serialises — no second predicate language. Variables are a new parameter
expression resolved from the context and substituted before any adapter sees a
query: `$currentUser`, `$now`, `$principal.roles`, named variables, and
**path captures** — a wildcard segment may be named (`/spaces/{spaceID}/**`) and
referenced as `$path.spaceID`. Enforcement per operation: `Get`/`Exists`
evaluate `where` on the read record; `Query` **AND-rewrites** `where` into the
structured query so the engine filters and paging stays correct; `Insert`
evaluates `check`; `Set`/`Update` evaluate `where` on the pre-image and `check`
on the post-image inside the same transaction; `Delete` evaluates `where` on the
pre-image. Conditions on allow rules only in the first slice (a conditional deny
needs a `NOT`/`!=` the AST lacks). Read and write levels differ by declaring
two rules on one path — a user reads every customer but edits only the
customers assigned to them (founder example, 2026-09-03) — and a conditional
rule never authorises `Truncate`. Precedence, composition and explanations are
unchanged; denials never leak values.

**2. Principal bindings (`access-policies/principal-bindings`).** A document
binds rule sets to `roles`, `groups`, `users` and `everyone`. The request's
principal (`id`, `roles`, `groups`) travels on the context. The effective policy
for a request is the **union** of the rule sets its principal is bound to,
compiled as *one* policy — so most-specific-wins and deny-on-tie resolve
overlaps exactly as today — and that single principal policy then
**intersects** with database-bound policies, so a binding can never reopen what
the database denies. Additive within the principal (Directus), monotonic across
layers (DALgo).

**3. Field patterns (`access-policies/field-patterns`).** A rule may carry
`fields`: an allow-list of field names and dotted paths with `*` wildcards
(`name`, `address.*`, `public_*`). On reads the wrapper limits a query's
projection and redacts point reads to the allowed fields; on writes only allowed
fields may be present (`Insert`/`Set`) or touched (`Update`). Wildcards for
*tables* are already the path patterns; this extends the same habit to fields.

**The document, in Firestore-rules terms.** `match` = a path pattern with
captures; `allow get, list, create, update, delete` = the operation vocabulary;
`if resource.data.owner == request.auth.uid` = `where` with `$currentUser`;
`request.resource.data` = `check`; `request.auth.token.role` = the principal's
roles, available both as bindings and as `$principal.roles` in conditions. What
DALgo adds that Firestore rules lack: portability across engines, engine-side
query rewriting, and the in-memory adapter enforcing the same document in tests.

## Alternatives Considered

- **Post-filter everything in the wrapper (no query rewrite).** Simple and
  adapter-neutral, but it breaks `Limit`/`Offset` and cursor semantics (a page
  of 10 may shrink to 3), moves the whole collection over the wire, and cannot
  be verified when the query projects away the condition's fields. Rewrite is
  the primary mechanism; post-filter stays as an optional strict-mode check.
- **Map conditions to native row-level security per adapter** (PostgreSQL RLS,
  Firestore rules). Strong defence in depth, but only two adapters have it,
  the in-memory adapter has none, and it splits the policy into per-backend
  dialects — the exact non-portability access policies exist to remove. Left
  as a later push-down capability an adapter may advertise.
- **A separate predicate language for policies** (CEL, a Firestore-rules-like
  grammar, a mini-DSL). Adds a parser, an evaluator and a second thing to
  learn; the query AST already has the operators the use cases need and DTQL
  already serialises it. The Firestore-rules *concepts* are adopted; its syntax
  is not.
- **Union across all policies (Directus-style additive everywhere).** Would let
  a context policy widen a database policy, which breaks the parent feature's
  monotonic-composition guarantee. Union is confined to the bindings of one
  principal; layers still intersect.
- **Conditions on deny rules in the first slice.** Attractive for "everything
  except locked rows", but without `NOT`/`!=` in the AST a conditional deny
  cannot be rewritten into a query, so it would silently fall back to
  post-filtering. Deferred until the AST grows negation.

## MVP Scope

Part 1 in full: `where`/`check` on allow rules with `Comparison` and And/Or
groups, `$currentUser`, `$now`, `$principal.*`, named variables and path
captures; enforced for `Get`, `Exists`, `Query` (AND-rewrite), `Insert`, `Set`,
`Update`, `Delete`; YAML/JSON codec support; DTQL `param` node; conformance on
`dalgo2memory` and the `end2end` suite; denial explanations without values.
Parts 2 and 3 are specified alongside and follow in the next two releases, in
that order: bindings first because a reporting tool needs "this role may query
these rows" before it needs field masks. Timebox: one `dalgo` release per part.

## Not Doing (and Why)

- Conditional **deny** rules — need negation in the query AST first; deferred
- A Firestore-rules-syntax parser — concepts adopted, syntax not; YAML + DTQL is
  the document
- Conditions that reference other records or sub-queries (Firestore's `get()`
  in rules) — no sub-query node in the AST and no cost model; single-record
  predicates only, designed so a bounded lookup can be added later
- Adapter-native push-down (PostgreSQL RLS, Firestore rules) — a later optional
  capability; the wrapper must be complete without it
- Identity, sessions, role storage — the host puts the principal on the
  context; DALgo never looks users up
- Best-effort checking on non-transactional adapters by default — fail closed;
  an explicit opt-in exists for adapters that cannot open a transaction

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | AND-rewriting a rule condition into `dal.StructuredQuery.Where` preserves the caller's results exactly (same rows minus the forbidden ones), including `Limit`, `Offset`, `OrderBy` and cursors, on every adapter. | Extend the `end2end` conformance suite with conditional-policy cases per adapter; compare rewritten results against an unconstrained query filtered in memory. |
| Must-be-true | Pre-image reads for conditional `Set`/`Update`/`Delete` can run inside the same read-write transaction on every transactional adapter without changing adapter code. | Prove through the secured wrapper against `dalgo2memory`, `dalgo2sql` (SQLite/PostgreSQL) and `dalgo2firestore`; a concurrent writer changing ownership between read and write must be rejected by the transaction, not slip through. |
| Must-be-true | Union-within-principal, intersection-across-layers keeps the parent feature's guarantees: order independence, most-specific wins, deny on tie, and no widening of a database denial. | Re-run the whole access-policies acceptance suite with bindings and conditions added at every depth; no decision may change for unconditional requests; add widening-attempt cases that must deny. |
| Should-be-true | The DTQL condition syntax plus `param` and path captures is enough for policy documents; no separate grammar is needed. | Author the Sneat extension policies, a DataTug tenant-reporting policy and a Firestore-rules sample translated one-to-one, in YAML; round-trip through the codec and compare decisions. |
| Should-be-true | Field patterns can be enforced as projection on reads and touched-field checks on writes without adapter changes. | Prototype on `dalgo2memory` and `dalgo2sql`; a query for `*` under a `fields: [name]` rule must reach the adapter as a projection of `name`. |
| Should-be-true | `Exists` upgraded to a read for conditional rules is an acceptable cost. | Measure on `dalgo2firestore`; document the cost; keep unconditional `Exists` untouched. |
| Might-be-true | Adapters with native row-level security can accept a push-down of the same condition later without changing the policy documents. | Defer; keep the condition as a `dal.Condition` value the adapter can inspect. |

## SpecScore Integration

- **New Features this would create:** [access-policies/row-level-conditions](../features/access-policies/row-level-conditions/README.md), [access-policies/principal-bindings](../features/access-policies/principal-bindings/README.md), [access-policies/field-patterns](../features/access-policies/field-patterns/README.md)
- **Existing Features affected:** [access-policies](../features/access-policies/README.md) (rule model, secured wrapper, codecs, path patterns gain named captures), [dtql](../features/dtql/README.md) (parameter expression node), the query builder (`dal.Condition` gains a parameter expression)
- **Dependencies:** none beyond the shipped access-policies feature; shares the pre-image read mechanism and the condition/parameter machinery with [triggers-and-webhooks](triggers-and-webhooks.md)

## Open Questions

- Should a row-level denial on `Get` surface as a `DeniedError` or as
  `ErrRecordNotFound`? *Decided 2026-09-03 (founder):* `DeniedError` by default,
  with a policy option to hide denied records as not found.
- Where do role and group *memberships* come from — always the host via the
  context, or may a binding document also declare group membership? Recommendation:
  host only; DALgo stays identity-agnostic.
- Should the DTQL parameter node be specified inside the `dtql` feature as a
  change request, or inside the row-level-conditions feature? Recommendation:
  change request on `dtql`, referenced from the feature.
- Should a bounded cross-record lookup (Firestore's `get()` in rules) be
  designed into the condition vocabulary now, so the slot exists, or left
  entirely to a later idea?
