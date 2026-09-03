---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: Row-level access conditions with runtime variables

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/access-policies/row-level-conditions?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/access-policies/row-level-conditions?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/access-policies/row-level-conditions?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/access-policies/row-level-conditions?op=request-change) |
**Status:** Implementing
**Date:** 2026-09-02
**Owner:** alex
**Source Ideas:** row-level-access-conditions
**Tracking:** [dal-go/dalgo#133](https://github.com/dal-go/dalgo/issues/133)

## Summary

Extend [hierarchical access policies](../README.md) so that an **allow rule may
carry a row condition**. The condition is a `dal.Condition` — the same
expression tree `dal.StructuredQuery` uses and DTQL serialises — over the
record's fields, with **runtime variables** (`$currentUser`, `$now`, `$principal.roles`,
named variables, and **path captures** such as `$path.spaceID` bound from a
named wildcard segment) resolved from the operation's `context.Context`. Two slots follow the
PostgreSQL row-level-security shape: `where` names the rows the rule applies to;
`check` names the shape a written row must have afterwards and defaults to
`where`. Because a condition attaches to a rule and a rule names its
operations, **reads and writes on the same path carry different conditions by
declaring two rules** — read every customer, edit only the customers assigned
to you — with no new precedence rule.

The secured wrapper enforces the condition in the way each operation needs:
evaluated on the record after a point read; **AND-rewritten into the structured
query** before execution so the engine filters; evaluated on the pre-image and
post-image of writes inside the same transaction. Policy documents carry
conditions in DTQL's condition syntax plus a `param` node.

The product promise is: **say which rows once, and every adapter — including the
in-memory one — enforces it, engine-side where the engine can.**

## Problem

Access policies are path-scoped. A rule can grant `Update` under
`/spaces/*/ext/trackus/**` but cannot say "only records the caller created", and
a reporting tool can be granted `Query` on `orders` only for the whole
collection, never for one tenant's rows. Every application re-implements these
predicates in code, per endpoint and per adapter, and the failure mode is
familiar: one endpoint forgets the check and a whole collection is exposed.

The parent feature reserved this space explicitly ("Query constraints over
`WHERE` … preserve room for these follow-ups") and kept `Query` a distinct
operation with the structured query on the request for that reason. Nothing
evaluates a record's fields today.

## Design Principles

- **One expression vocabulary.** Conditions are `dal.Condition` values. No second
  predicate language, no parser; DTQL already serialises the tree.
- **Engine-side when possible, wrapper-side always.** `Query` is rewritten so the
  engine filters and paging stays correct; point reads and writes are checked by
  the wrapper because there is nothing to rewrite.
- **Fail closed.** An unresolved variable, a post-image the wrapper cannot
  compute, a non-transactional adapter for a conditional write, a condition on a
  resource kind it cannot apply to — all deny.
- **Additive.** Unconditional rules, precedence, composition, documents and
  explanations behave exactly as the parent feature specifies; adding a
  condition can only narrow.
- **Explainable without disclosure.** A denial names the rule, the slot and the
  condition text with parameter names; it never includes resolved values or
  record contents.
- **Adapter-independent.** Adapters never see a variable: the wrapper resolves
  parameters to constants before delegating. No adapter changes are required.

## Behavior

### REQ: condition-vocabulary

A rule condition MUST be a `dal.Condition` composed of `dal.Comparison` nodes
and And/Or `dal.GroupCondition` nodes. A comparison's left operand MUST be a
`dal.FieldRef` naming a field of the record under evaluation; its right operand
MUST be a `dal.Constant`, a `dal.Array` (for `In`) or a **parameter
expression**. The operators MUST be exactly those the query model defines
(`==`, `In`, `>`, `>=`, `<`, `<=`). The package MUST introduce a parameter
expression type in `dal` (a `FieldRef`/`Constant` sibling) whose string form is
`$<name>`; it MUST be usable in any `dal.StructuredQuery` as well as in policy
conditions.

#### AC-1: comparison-and-groups

**Given** a rule whose condition is `And(createdBy == $currentUser, status In [draft, review])`
**When** the policy is constructed
**Then** construction succeeds and the compiled rule reports both field names it references.

#### AC-2: unsupported-node-rejected

**Given** a rule whose condition's left operand is a constant rather than a field reference
**When** the policy is constructed
**Then** construction fails with an error naming the rule and the offending node.

### REQ: rule-condition-slots

A rule MUST accept two optional conditions: `where` and `check`. `check` MUST
default to `where` when absent. In this feature, conditions MUST be accepted on
**allow** rules only; constructing a deny rule with a condition MUST fail. A
condition MUST NOT be accepted on a collection-group resource rule or on an
opaque-query rule. A conditional rule MUST NOT authorise `Truncate`: when its
operations name `Truncate` explicitly, construction MUST fail; when they name
the `write` or `readwrite` group, `Truncate` MUST be excluded from the compiled
rule. Two rules on the same path with disjoint operations MUST both apply, so
an unconditional `read` allow and a conditional `update`/`delete` allow on the
same path express "read all, edit own" without any precedence interaction.

#### AC-1: check-defaults-to-where

**Given** an allow rule with only `where: ownerID == $currentUser`
**When** an `Insert` is evaluated with data whose `ownerID` differs from the current user
**Then** the insert is denied with the explanation naming the `check` slot.

#### AC-2: conditional-deny-rejected

**Given** a deny rule carrying a `where` condition
**When** the policy is constructed
**Then** construction fails and the error states that conditional deny rules are not supported.

#### AC-3: read-all-edit-assigned

**Given** on `/customers/*` an unconditional rule allowing `Read` and a rule allowing `Update` and `Delete` `where assignedTo == $currentUser`, and a context with current user `u1`
**When** `u1` queries customers, gets a customer assigned to `u2`, updates a customer assigned to `u1`, and updates one assigned to `u2`
**Then** the query returns every customer, the get succeeds, the first update succeeds, and the second update is denied naming the conditional rule.

#### AC-4: write-group-excludes-truncate

**Given** a conditional rule allowing the `write` group `where ownerID == $currentUser` on `/notes/*`
**When** a truncate request for `notes` is evaluated
**Then** it is denied, and the compiled rule reports `Insert`, `Set`, `Update` and `Delete` only.

### REQ: variable-resolution

The package MUST resolve parameter expressions from the operation context
through a variables carrier attached with `access.WithVariables(ctx, …)` and a
convenience `access.WithCurrentUser(ctx, id)`. `$currentUser` MUST resolve from
that convenience (or from the principal's `ID` when a principal is present);
`$now` MUST resolve to the evaluation time; `$principal.roles` and
`$principal.groups` MUST resolve to the principal's role and group sets as
arrays usable on the right of `In`. All parameters of
a request MUST be resolved once, before any enforcement step, so one request
sees one consistent set of values. An unresolved parameter MUST deny the request
with an explanation naming the parameter and MUST NOT fall back to a default.

#### AC-1: missing-variable-denies

**Given** a rule conditioned on `tenantID == $tenant` and a context carrying no `tenant` variable
**When** a query on the matching collection is executed
**Then** the query is denied before the adapter is called and the explanation names `$tenant`.

#### AC-2: consistent-snapshot

**Given** a batch write of three records under a rule conditioned on `$now`
**When** the batch is evaluated
**Then** every record is evaluated against the same `$now` value.

### REQ: path-captures

A path pattern MUST accept a **named wildcard segment** written `{name}` in
place of a single-ID wildcard, matching exactly what `*` matches. When a rule
under such a pattern is evaluated for a resource, the matched segment value
MUST be bound to the variable `$path.<name>` for that rule's conditions. A
`$path.` variable referenced by a condition whose pattern does not capture that
name MUST fail construction. Captures MUST be available to `where` and `check`
for every operation, including `Query`, where the capture binds from the
query's collection path.

#### AC-1: capture-in-condition

**Given** a rule on `/spaces/{spaceID}/ext/trackus/**` conditioned on `spaceID == $path.spaceID`
**When** a record under `/spaces/s1/ext/trackus/items/i1` whose `spaceID` is `s1` is read, and one whose `spaceID` is `s2`
**Then** the first read is allowed and the second is denied.

#### AC-2: unknown-capture-rejected

**Given** a rule on `/spaces/*/ext/trackus/**` conditioned on `$path.spaceID`
**When** the policy is constructed
**Then** construction fails naming the missing capture.

### REQ: point-read-enforcement

For `Get`, the wrapper MUST delegate the read and then evaluate `where` against
the returned record; when the condition is false the wrapper MUST deny and MUST
NOT return the record's data. For `Exists`, when the winning rule is conditional
the wrapper MUST perform a read instead of the adapter's existence check so the
condition can be evaluated; an unconditional rule MUST keep using the adapter's
existence check. A denied conditional read MUST surface as a `DeniedError`
unless the policy option to hide denied records is set, in which case it MUST
surface as `dal.ErrRecordNotFound`.

#### AC-1: get-other-users-record

**Given** an allow rule `Get where ownerID == $currentUser` and a record owned by someone else
**When** the caller gets that record
**Then** the call is denied and the returned record carries no data.

#### AC-2: exists-upgraded-to-read

**Given** the same rule and a recording adapter
**When** the caller checks existence of a record
**Then** the adapter observes a get, not an exists, and the result reflects the condition.

### REQ: query-rewrite

For a `Query` whose base source is authorised by a conditional allow rule, the
wrapper MUST resolve the parameters and add the `where` condition to the
structured query's `Where` as a conjunct (`And`) before delegating, preserving
the caller's own conditions, `Columns`, `OrderBy`, `Limit`, `Offset` and cursor.
When several policies each contribute a conditional allow for the same source,
all their conditions MUST be conjoined. Joined sources MUST be treated the same
way, each with its own rules. A conditional rule MUST NOT authorise a
collection-group query or an opaque query; those keep the parent feature's
explicit-rule requirement. The wrapper MAY, when a strict option is set,
re-evaluate `where` on returned records whose projection includes every
referenced field, and MUST deny the result set if any record fails.

#### AC-1: tenant-slice

**Given** an allow rule `Query where tenantID == $tenant` on `/orders` and a context with `tenant = t1`
**When** the caller queries `orders` with `Limit 10` ordered by `createdAt`
**Then** the adapter executes a query whose `Where` conjoins `tenantID == "t1"` with the caller's conditions, and the ten returned rows all belong to `t1`.

#### AC-2: conditions-conjoined-across-policies

**Given** a database policy conditioning `Query` on `tenantID == $tenant` and a context policy conditioning it on `status == active`
**When** the caller queries the collection
**Then** the delegated query's `Where` contains both conjuncts.

#### AC-3: no-variables-reach-adapter

**Given** any conditional rule
**When** the wrapper delegates a query
**Then** the delegated query contains no parameter expressions, only constants.

### REQ: write-enforcement

Conditional writes MUST be enforced as follows. `Insert`: evaluate `check` on
the new data. `Set`: read the pre-image inside the transaction; if it exists,
evaluate `where` on it; evaluate `check` on the new data. `Update`: read the
pre-image, evaluate `where` on it, compute the post-image by applying the update
and evaluate `check` on it; if the wrapper cannot compute the post-image for the
update's operations it MUST deny. `Delete`: read the pre-image and evaluate
`where` on it. Multi-record writes MUST be evaluated in full before any record
is written and denied whole if any record fails. On an adapter without
transactions, a conditional write MUST be denied unless the policy sets an
explicit best-effort option, in which case the pre-image read precedes the
write without isolation and the decision explanation states so.

#### AC-1: cannot-take-ownership

**Given** an allow rule `Set where ownerID == $currentUser` and an existing record owned by another user
**When** the caller sets that key with data whose `ownerID` is the caller
**Then** the write is denied because `where` fails on the pre-image.

#### AC-2: update-post-image

**Given** an allow rule `Update where ownerID == $currentUser check ownerID == $currentUser`
**When** the caller updates their own record by setting `ownerID` to someone else
**Then** the update is denied because `check` fails on the computed post-image.

#### AC-3: batch-all-or-nothing

**Given** a conditional `Delete` rule and a multi-delete where one key's pre-image fails `where`
**When** the batch is executed
**Then** nothing is deleted.

### REQ: precedence-with-conditions

The parent feature's precedence — greatest depth, then literal specificity,
then deny on tie, declaration order irrelevant — MUST be unchanged. A
conditional rule whose condition evaluates false MUST be treated as not
matching for that record. An unconditional deny at the same specificity as a
conditional allow MUST still win. Composition across policies MUST remain
intersection: a conditional allow in one policy cannot reopen a denial in
another.

#### AC-1: unconditional-suite-unchanged

**Given** the parent feature's complete acceptance suite
**When** it is re-run with conditional rules added at every depth of every policy
**Then** every unconditional request receives the same decision as before.

#### AC-2: conditional-allow-cannot-reopen

**Given** a database policy denying `Update` under `/system/**` and a context policy allowing `Update where ownerID == $currentUser` there
**When** the owner updates a record under `/system/`
**Then** the update is denied.

### REQ: portable-condition-documents

YAML and JSON policy documents MUST accept `where` and `check` on a rule, each
holding a condition in the **DTQL condition syntax** (`op`/`left`/`right`
comparisons, `and`/`or` groups, `field`/`value`/`values` expressions) extended
with a `param: <name>` expression. Documents without conditions MUST remain
valid unchanged. DTQL MUST gain the same `param` expression so a saved query and
a policy condition are written identically; that DTQL change is specified as a
change request on the [`dtql`](../../dtql/README.md) feature and referenced here.

```yaml
scopes:
  - path: /customers/*
    rules:
      - id: read-all
        effect: allow
        operations: [read]
      - id: edit-assigned
        effect: allow
        operations: [update, delete]
        where:
          op: "=="
          left: { field: assignedTo }
          right: { param: currentUser }
```

#### AC-1: yaml-roundtrip

**Given** a YAML policy with a rule carrying `where: { op: "==", left: { field: ownerID }, right: { param: currentUser } }`
**When** it is decoded, evaluated against fixtures, encoded and decoded again
**Then** every decision is identical across both decodings.

### REQ: explanations-without-values

A denial caused by a condition MUST carry a decision whose explanation names the
policy, the rule, the slot (`where` or `check`) and the condition in its string
form with parameter names. It MUST NOT include resolved parameter values or any
field value of the record.

#### AC-1: no-leak

**Given** a denied conditional `Get` on a record with a secret field
**When** the `DeniedError` is formatted
**Then** the text contains the rule id and `$currentUser`, and contains neither the resolved user id nor any record value.

## Acceptance Criteria

### AC: comparison-and-groups (verifies REQ:condition-vocabulary)

**Given** a rule whose condition is `And(createdBy == $currentUser, status In [draft, review])`
**When** the policy is constructed
**Then** construction succeeds and the compiled rule reports both field names it references.

### AC: conditional-deny-rejected (verifies REQ:rule-condition-slots)

**Given** a deny rule carrying a `where` condition
**When** the policy is constructed
**Then** construction fails and the error states that conditional deny rules are not supported.

### AC: read-all-edit-assigned (verifies REQ:rule-condition-slots)

**Given** on `/customers/*` an unconditional rule allowing `Read` and a rule allowing `Update` and `Delete` `where assignedTo == $currentUser`, and a context with current user `u1`
**When** `u1` queries customers, gets a customer assigned to `u2`, updates a customer assigned to `u1`, and updates one assigned to `u2`
**Then** the query returns every customer, the get succeeds, the first update succeeds, and the second update is denied naming the conditional rule.

### AC: write-group-excludes-truncate (verifies REQ:rule-condition-slots)

**Given** a conditional rule allowing the `write` group `where ownerID == $currentUser` on `/notes/*`
**When** a truncate request for `notes` is evaluated
**Then** it is denied, and the compiled rule reports `Insert`, `Set`, `Update` and `Delete` only.

### AC: missing-variable-denies (verifies REQ:variable-resolution)

**Given** a rule conditioned on `tenantID == $tenant` and a context carrying no `tenant` variable
**When** a query on the matching collection is executed
**Then** the query is denied before the adapter is called and the explanation names `$tenant`.

### AC: capture-in-condition (verifies REQ:path-captures)

**Given** a rule on `/spaces/{spaceID}/ext/trackus/**` conditioned on `spaceID == $path.spaceID`
**When** a record under `/spaces/s1/ext/trackus/items/i1` whose `spaceID` is `s1` is read, and one whose `spaceID` is `s2`
**Then** the first read is allowed and the second is denied.

### AC: get-other-users-record (verifies REQ:point-read-enforcement)

**Given** an allow rule `Get where ownerID == $currentUser` and a record owned by someone else
**When** the caller gets that record
**Then** the call is denied and the returned record carries no data.

### AC: tenant-slice (verifies REQ:query-rewrite)

**Given** an allow rule `Query where tenantID == $tenant` on `/orders` and a context with `tenant = t1`
**When** the caller queries `orders` with `Limit 10` ordered by `createdAt`
**Then** the adapter executes a query whose `Where` conjoins `tenantID == "t1"` with the caller's conditions, and the ten returned rows all belong to `t1`.

### AC: cannot-take-ownership (verifies REQ:write-enforcement)

**Given** an allow rule `Set where ownerID == $currentUser` and an existing record owned by another user
**When** the caller sets that key with data whose `ownerID` is the caller
**Then** the write is denied because `where` fails on the pre-image.

### AC: unconditional-suite-unchanged (verifies REQ:precedence-with-conditions)

**Given** the parent feature's complete acceptance suite
**When** it is re-run with conditional rules added at every depth of every policy
**Then** every unconditional request receives the same decision as before.

### AC: yaml-roundtrip (verifies REQ:portable-condition-documents)

**Given** a YAML policy with a rule carrying a `where` condition using `param: currentUser`
**When** it is decoded, evaluated against fixtures, encoded and decoded again
**Then** every decision is identical across both decodings.

### AC: no-leak (verifies REQ:explanations-without-values)

**Given** a denied conditional `Get` on a record with a secret field
**When** the `DeniedError` is formatted
**Then** the text contains the rule id and `$currentUser`, and contains neither the resolved user id nor any record value.

## Architecture

- **`dal`** gains a parameter expression type (string form `$<name>`), usable in
  `StructuredQuery` conditions. Adapters are not required to understand it: the
  secured wrapper substitutes constants before delegating, and an adapter that
  receives a parameter unexpectedly returns `dal.ErrNotSupported`.
- **`access`** extends `Rule` with `Where` and `Check` (`dal.Condition`), the
  compiled rule with the referenced field set, and the evaluator with a
  record-condition step. Variables travel on `context.Context`
  (`WithVariables`, `WithCurrentUser`); resolution produces a substituted
  condition per request.
- **Condition evaluation over record data** reuses the in-memory condition
  evaluator that `dalgo2memory` already implements to execute structured
  queries; it is extracted into a shared package so the adapter and the policy
  wrapper share one implementation. Struct data is evaluated through the same
  field access the query model uses; map data directly.
- **Path patterns** gain named captures (`{name}`), compiled to the same
  matcher as `*`; the match records the bound values for the rule's evaluation.
- **Query rewrite** operates on `dal.StructuredQuery` only; text or otherwise
  opaque queries keep the parent feature's explicit-rule rule.
- **Pre-image capture** for conditional writes is one step in the secured
  write path, executed inside the caller's transaction (or one the wrapper
  opens); it is designed to be shared with the pre-image the triggers idea
  needs.
- **Codecs** extend the versioned document model with `where`/`check` using
  DTQL's condition mapping plus `param`.

No adapter is modified.

## Error Handling and Failure Modes

- Unresolved parameter: deny before delegation; explanation names the parameter.
- Post-image not computable for an update: deny; explanation names the update operation.
- Conditional write on a non-transactional adapter without the best-effort option: deny.
- Condition references a field absent from the record: the comparison is false; the request is denied by that rule (fail closed), never treated as a match.
- Condition on a deny rule, on a rule naming `Truncate` explicitly, on a collection-group or opaque-query rule: constructor error; `Must…` panics. A conditional `write`/`readwrite` group silently drops `Truncate`.
- Strict re-evaluation after a query finds a violating record: deny the whole result; explanation names the rule.
- Denied conditional `Get`/`Exists`: `DeniedError` (default) or `ErrRecordNotFound` (hide option), never data.

## Testing Strategy

- Table tests for condition construction, referenced-field extraction, slot defaulting and constructor rejections.
- Evaluator tests over struct and map records, missing fields, `In` arrays, and every operator.
- Recording fake sessions proving: `Exists` upgrade to a read, delegated queries contain constants only, pre-image reads occur inside the transaction, batches deny whole.
- Query-rewrite tests preserving caller `Where`, `Columns`, `OrderBy`, `Limit`, `Offset` and cursor, on single and joined sources, across multiple policies.
- The parent feature's full acceptance suite re-run with conditional rules injected (no decision changes for unconditional requests).
- `end2end` conformance cases for conditional policies on `dalgo2memory`, `dalgo2sql` (SQLite and PostgreSQL) and `dalgo2firestore`.
- Codec round-trip fixtures in YAML and JSON, including invalid `param` shapes.
- Explanation-text tests asserting the absence of resolved values and record contents.

## Out of Scope

- Conditional **deny** rules (need negation in the query AST).
- Field-level constraints, projections or redaction.
- Conditions referencing other records, sub-queries or aggregates.
- Push-down to adapter-native row-level security (PostgreSQL RLS, Firestore rules); left as a later optional capability.
- Any change to adapters' query execution.
- Persisting variables, sessions or identity; the host supplies them on the context.

## Open Questions

- Default behaviour of a denied conditional point read: *Decided 2026-09-03 (founder):* `DeniedError` by default (consistent and explainable); the policy option to hide denied records as `ErrRecordNotFound` remains available for enumeration-sensitive collections.
- Should the parameter expression live in `dal` (usable by any query) or in `access` only? Recommended: `dal`, because DTQL and DataTug need it independently.
- Should the `dtql` change (the `param` node) be filed as a change request on the `dtql` feature before this feature leaves Draft?
- Which named variables, beyond `currentUser` and `now`, deserve a built-in convenience?
