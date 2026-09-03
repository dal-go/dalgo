---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: Field patterns — wildcard field allow-lists on rules

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/access-policies/field-patterns?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/access-policies/field-patterns?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/access-policies/field-patterns?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/access-policies/field-patterns?op=request-change) |
**Status:** Implementing
**Date:** 2026-09-02
**Owner:** alex
**Source Ideas:** row-level-access-conditions
**Tracking:** [dal-go/dalgo#133](https://github.com/dal-go/dalgo/issues/133)

## Summary

Let an allow rule carry `fields`: an allow-list of field names and dotted
paths with `*` wildcards (`name`, `address.*`, `public_*`). On reads the
secured wrapper **projects** a query to the allowed fields and **redacts** point
reads to them; on writes it requires that every field present (`Insert`,
`Set`) or touched (`Update`) matches the list. Wildcards for collections are
the parent feature's path patterns; this extends the same habit to fields.

The canonical case: a caller may list `users` but never receive
`passwordHash`.

The product promise is: **say which fields once, and reads never return more
than that, writes never change more than that, on any adapter.**

## Problem

A rule today grants an operation on a whole record. A support tool allowed to
read `users` sees password hashes; a reporting tool allowed to query `orders`
sees card tokens; an extension allowed to update a contact can rewrite its
`ownerID`. Field exposure is decided in application code per endpoint — or not
at all. Directus lets a permission name allowed fields (`*` or a list); Firestore
rules can test the keys a write touches. DALgo has neither.

## Design Principles

- **Allow-list, wildcard, dotted.** `fields` names what is allowed; `*` matches
  one segment; a trailing `.*` matches a subtree. No deny-list in the first
  slice — the most-specific rule already narrows.
- **Reads project, writes check.** Projection is pushed into the query where the
  adapter supports column selection; point reads are redacted in the wrapper.
- **Conditions may see what callers may not.** A `where`/`check` condition may
  reference a field outside `fields`; it is evaluated by the wrapper or the
  engine, never returned.
- **Fail closed.** A write that mentions a disallowed field is denied whole; a
  query that cannot be projected is denied rather than returned in full.

## Behavior

### REQ: field-pattern-vocabulary

A rule MUST accept `fields` as a list of patterns. A pattern MUST be a dotted
path whose segments are literal names or `*`; a trailing `.*` MUST match the
whole subtree below the preceding path; a `*` inside a segment (`public_*`)
MUST match a name prefix or suffix. Patterns MUST be validated at construction.
A rule without `fields` MUST mean all fields, as today.

#### AC-1: pattern-forms

**Given** `fields: [name, address.*, public_*]`
**When** the fields `name`, `address.city`, `public_bio`, `address`, `email` are tested
**Then** the first three match and the last two do not.

### REQ: read-projection

For `Query` under a rule with `fields`, the wrapper MUST restrict the query's
`Columns` to the intersection of the caller's selection (all fields when none
is given) and the allowed patterns, before delegating; when the adapter cannot
honour a projection the wrapper MUST redact returned records. For `Get`, the
wrapper MUST redact the returned record to the allowed fields. Redaction MUST
remove disallowed fields entirely rather than blanking them, so a caller cannot
distinguish "absent" from "hidden".

#### AC-1: query-projected

**Given** a rule allowing `Query` on `/users` with `fields: [id, name]` and a recording adapter
**When** the caller queries `users` selecting all columns
**Then** the adapter observes a query selecting `id` and `name` only.

#### AC-2: get-redacted

**Given** the same rule
**When** the caller gets `/users/u1`
**Then** the returned data contains `id` and `name` and no other field.

### REQ: write-field-check

For `Insert` and `Set`, every field present in the data MUST match an allowed
pattern; for `Update`, every field path the update touches MUST match; a
violation MUST deny the whole write (and the whole batch for multi-record
writes) before delegation. Condition slots (`where`, `check`) MAY reference
disallowed fields.

#### AC-1: update-outside-fields

**Given** a rule allowing `Update` on `/contacts/*` with `fields: [name, phones.*]`
**When** the caller updates `ownerID`
**Then** the update is denied naming `ownerID`.

#### AC-2: condition-may-read-hidden-field

**Given** a rule with `fields: [name]` and `where: ownerID == $currentUser`
**When** the caller gets their own record
**Then** the read is allowed and the returned data contains `name` only.

### REQ: precedence-and-composition

`fields` MUST participate in precedence like any other rule attribute: the
winning rule's list applies. Across policies in an intersection, the effective
allowed set MUST be the intersection of every applicable rule's allowed fields.

#### AC-1: intersection-across-policies

**Given** a database policy allowing `fields: [id, name, email]` and a context policy allowing `fields: [name, phone]` on the same path
**When** the caller reads a record
**Then** only `name` is returned.

## Acceptance Criteria

### AC: pattern-forms (verifies REQ:field-pattern-vocabulary)

**Given** `fields: [name, address.*, public_*]`
**When** the fields `name`, `address.city`, `public_bio`, `address`, `email` are tested
**Then** the first three match and the last two do not.

### AC: query-projected (verifies REQ:read-projection)

**Given** a rule allowing `Query` on `/users` with `fields: [id, name]` and a recording adapter
**When** the caller queries `users` selecting all columns
**Then** the adapter observes a query selecting `id` and `name` only.

### AC: get-redacted (verifies REQ:read-projection)

**Given** the same rule
**When** the caller gets `/users/u1`
**Then** the returned data contains `id` and `name` and no other field.

### AC: update-outside-fields (verifies REQ:write-field-check)

**Given** a rule allowing `Update` on `/contacts/*` with `fields: [name, phones.*]`
**When** the caller updates `ownerID`
**Then** the update is denied naming `ownerID`.

### AC: condition-may-read-hidden-field (verifies REQ:write-field-check)

**Given** a rule with `fields: [name]` and `where: ownerID == $currentUser`
**When** the caller gets their own record
**Then** the read is allowed and the returned data contains `name` only.

### AC: intersection-across-policies (verifies REQ:precedence-and-composition)

**Given** a database policy allowing `fields: [id, name, email]` and a context policy allowing `fields: [name, phone]` on the same path
**When** the caller reads a record
**Then** only `name` is returned.

## Architecture

- `access.Rule` gains the `Fields(patterns ...string)` builder (documents:
  `fields:`); compiled rules hold a parsed matcher. `fields` is accepted on
  allow rules only and applies to every operation the rule names.
- The secured query executor rewrites `Columns` using the existing
  [column projection](../../query-column-projection/README.md) support; when an
  adapter reports it cannot project, the wrapper redacts in the reader.
- Point-read redaction removes keys from map data in place; a struct target is
  zeroed and re-populated with the allowed fields only, so hidden fields are at
  their zero value (a struct cannot express absence). Callers that need to
  tell "absent" from "zero" read into a map.
- Write checks inspect record data (maps or structs via field access) and the
  `update` operations' field paths.
- Codecs extend the versioned document model with `fields`.

No adapter is modified.

## Error Handling and Failure Modes

- Invalid pattern: constructor error; `Must…` panics.
- Write touching a disallowed field: deny whole write/batch; explanation names the field.
- Query cannot be projected and redaction is impossible for the reader kind: deny.
- Empty intersection across policies: reads return records with no fields; writes are denied.

## Testing Strategy

- Pattern matcher table tests (literal, `*` segment, prefix/suffix, subtree).
- Recording adapter tests proving projected queries and redacted reads.
- Write tests for insert/set/update/multi with allowed and disallowed fields.
- Intersection tests across database and context policies.
- Codec round trips.
- The parent feature's suite re-run with `fields` absent (no change).

## Out of Scope

- Field deny-lists (`except`), field-level *conditions* (a field visible only when …), and computed/derived fields.
- Encrypting or hashing hidden fields; redaction removes, it does not transform.
- Index-, cost- or aggregate-aware constraints.

## Open Questions

None at this time.

Resolved during implementation:

- Redaction on struct data keeps the caller's type: the struct is zeroed and
  re-populated with the allowed fields (see Architecture). A map is not
  substituted because the caller chose the target type.
- One rule carries one `fields` list for every operation it names; different
  read and write lists are two rules, so precedence stays per rule.
