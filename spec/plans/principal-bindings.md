---
format: https://specscore.md/plan-specification
status: Implemented
---
# Plan: Implement principal bindings

**Status:** Implemented
**Source Feature:** access-policies/principal-bindings
**Date:** 2026-09-03
**Owner:** alex
**Supersedes:** —

## Summary

Implement `access-policies/principal-bindings`: a principal on the context, a
`PrincipalPolicySet` that unions the rule sets bound to the principal's ID,
roles and groups (plus `everyone`) into one compiled policy, layer intersection
through the existing guard, binding attribution in explanations, and portable
documents with `ruleSets` and `bindings`.

## Approach

Bindings are a `Policy` implementation, not a change to the evaluator: the
union of a principal's rule sets is compiled as an ordinary `AccessPolicy`
(rule names prefixed per set so they stay unique and attributable), cached per
binding combination, and evaluated by the existing precedence walk. The
secured wrapper sees one more policy in its intersection, so no widening is
possible. `$currentUser`, `$principal.roles` and `$principal.groups` default
from the principal unless explicit variables override them.

## Tasks

### Task 1: Principal on the context and principal-derived variables

**Id:** task-1
**Verifies:** access-policies/principal-bindings#ac:principal-drives-current-user
**Depends-On:** —
**Status:** complete

`Principal{ID, Roles, Groups}`, `WithPrincipal`, `PrincipalFrom`; the variable
resolver defaults `$currentUser`, `$principal.roles` and `$principal.groups`
from the principal, explicit variables winning.

### Task 2: Rule sets, bindings and the effective policy

**Id:** task-2
**Verifies:** access-policies/principal-bindings#ac:shared-rule-set, access-policies/principal-bindings#ac:roles-union, access-policies/principal-bindings#ac:binding-in-explanation
**Depends-On:** 1
**Status:** complete

`NewPrincipalPolicySet` validates names and references; `Decide` unions bound
rule sets (sorted, de-duplicated), compiles and caches the union, delegates,
and appends `via role:…, group:…, user:…, everyone` to explanations.

### Task 3: Layer intersection and denials

**Id:** task-3
**Verifies:** access-policies/principal-bindings#ac:binding-cannot-widen
**Depends-On:** 2
**Status:** complete

The set is one more `Policy` in the guard's intersection; an unbound principal
(or an absent one with no `everyone`) is denied with an explanation naming it.

### Task 4: Portable documents

**Id:** task-4
**Verifies:** access-policies/principal-bindings#ac:shared-rule-set
**Depends-On:** 2
**Status:** complete

`ruleSets` and `bindings` on the document; `DecodePrincipalPolicySet`,
`EncodePrincipalPolicySet`, YAML/JSON helpers, and `DecodePolicy` routing by
shape; an `AccessPolicy` decode of a bindings document is refused.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
