---
format: https://specscore.md/plan-specification
status: Implemented
---
# Plan: Row-level access conditions — path captures

**Status:** Implemented
**Source Feature:** access-policies/row-level-conditions
**Date:** 2026-09-03
**Owner:** alex
**Supersedes:** —

## Summary

Third slice of `access-policies/row-level-conditions`: a path pattern may name a
wildcard ID segment — `access.Capture("spaceID")` in Go, `{spaceID}` in a
document path — and the matched value is bound to `$path.spaceID` for the
conditions of rules under that pattern. This is the shape most Sneat extension
policies need: `/spaces/{spaceID}/ext/trackus/**` with
`spaceID == $path.spaceID`, so a record filed under one Space cannot claim
another.

## Approach

Captures are ordinary wildcard segments with a name. A rule's conditions may
reference `$path.<name>` only for captures its (joined) path declares; anything
else fails compilation. When a rule matches a resource, the capture values are
merged into the request's variable resolver for that rule (captures win over
same-named variables), so substitution, query rewrite, point-read checks and
write residuals need no new machinery.

## Tasks

### Task 1: Captures in path patterns

**Id:** task-1
**Verifies:** access-policies/row-level-conditions#ac:unknown-capture-rejected
**Depends-On:** —
**Status:** complete

`Capture(name)` for `Path`/`Scope`/`Under`; validated, unique names; `{name}` in
`String()` and in portable document paths, both directions; compile-time
rejection of `$path.<name>` without a matching capture.

### Task 2: Binding captures in decisions

**Id:** task-2
**Verifies:** access-policies/row-level-conditions#ac:capture-in-condition
**Depends-On:** 1
**Status:** complete

Matched capture values feed the resolver for the matched rule's `where` and
`check`, for record and collection resources, reads and writes; verified on
`dalgo2memory` with a Space-scoped policy.

## Deferred AC Coverage

- access-policies/row-level-conditions#ac:comparison-and-groups — unchanged; verified by [`row-level-conditions-mvp`](row-level-conditions-mvp.md).
- access-policies/row-level-conditions#ac:conditional-deny-rejected — unchanged; verified by [`row-level-conditions-mvp`](row-level-conditions-mvp.md).
- access-policies/row-level-conditions#ac:write-group-excludes-truncate — unchanged; verified by [`row-level-conditions-mvp`](row-level-conditions-mvp.md).
- access-policies/row-level-conditions#ac:missing-variable-denies — unchanged; verified by [`row-level-conditions-mvp`](row-level-conditions-mvp.md).
- access-policies/row-level-conditions#ac:get-other-users-record — unchanged; verified by [`row-level-conditions-mvp`](row-level-conditions-mvp.md).
- access-policies/row-level-conditions#ac:tenant-slice — unchanged; verified by [`row-level-conditions-mvp`](row-level-conditions-mvp.md).
- access-policies/row-level-conditions#ac:unconditional-suite-unchanged — unchanged; verified by [`row-level-conditions-mvp`](row-level-conditions-mvp.md).
- access-policies/row-level-conditions#ac:yaml-roundtrip — unchanged; verified by [`row-level-conditions-mvp`](row-level-conditions-mvp.md).
- access-policies/row-level-conditions#ac:no-leak — unchanged; verified by [`row-level-conditions-mvp`](row-level-conditions-mvp.md).
- access-policies/row-level-conditions#ac:check-defaults-to-where — unchanged; verified by [`row-level-conditions-write-side`](row-level-conditions-write-side.md).
- access-policies/row-level-conditions#ac:cannot-take-ownership — unchanged; verified by [`row-level-conditions-write-side`](row-level-conditions-write-side.md).
- access-policies/row-level-conditions#ac:update-post-image — unchanged; verified by [`row-level-conditions-write-side`](row-level-conditions-write-side.md).
- access-policies/row-level-conditions#ac:batch-all-or-nothing — unchanged; verified by [`row-level-conditions-write-side`](row-level-conditions-write-side.md).
- access-policies/row-level-conditions#ac:read-all-edit-assigned — unchanged; verified by [`row-level-conditions-write-side`](row-level-conditions-write-side.md).

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
