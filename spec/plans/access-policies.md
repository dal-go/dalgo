---
format: https://specscore.md/plan-specification
status: Approved
---

# Plan: Implement hierarchical access policies

**Status:** Implemented
**Source Feature:** access-policies
**Date:** 2026-07-17
**Owner:** alex
**Supersedes:** —

## Summary

Implement the `access-policies` Feature in seven bottom-up tasks: vocabulary and matching, access/audit decisions, portable documents, context composition, secured sessions, secured transactions and queries, and integration hardening. Each task is independently testable and every Feature acceptance criterion is assigned.

## Approach

Build the pure policy engine before wrappers so precedence and explanations are stable and easy to test. Then wrap leaf sessions, followed by DB transaction coordination and query-source analysis, so no adapter changes are required. Finish with memory-adapter integration. Audit scope is deliberately classification-only: it reuses the matcher but does not introduce an audit persistence subsystem.

## Tasks

### Task 1: Operation vocabulary and structural resources

**Verifies:** access-policies#ac:write-only-insert, access-policies#ac:get-without-query, access-policies#ac:truncate-is-reserved, access-policies#ac:typed-id-and-slash-safe, access-policies#ac:terminal-collection-rule
**Status:** complete

Add leaf operations and immutable convenience groups, including reserved `Truncate`. Add record, collection, collection-group, and opaque-query resources plus typed path patterns supporting literal IDs, `AnyID`, and terminal collections.

### Task 2: Hierarchical access and audit policy engine

**Verifies:** access-policies#ac:allowed-parent-denied-child, access-policies#ac:denied-parent-reopened-children, access-policies#ac:order-independent, access-policies#ac:denial-is-inspectable, access-policies#ac:unknown-operation-denied, access-policies#ac:audit-selected-mutations, access-policies#ac:audit-log-recursion-carveout, access-policies#ac:audit-does-not-authorize
**Status:** complete

Compile nested scopes into deterministic rules; implement deepest/literal specificity and deny/ignore tie-breaking; expose direct decisions, typed denial errors, and independent audit classification with explanations.

### Task 3: Versioned YAML and JSON policy documents

**Verifies:** access-policies#ac:yaml-roundtrip, access-policies#ac:json-has-identical-semantics, access-policies#ac:storage-neutral-streams, access-policies#ac:invalid-or-future-document-rejected
**Status:** complete

Define one declarative document model for access and audit policies, implement strict versioned validation, YAML/JSON bytes and stream codecs, and a codec extension boundary that does not couple loading to files or another storage system.

### Task 4: Database and context policy composition

**Verifies:** access-policies#ac:context-cannot-widen-global, access-policies#ac:bound-policy-survives-context-replacement, access-policies#ac:required-context-policy
**Status:** complete

Add context attachment, database-bound policies, bound-context capture, monotonic intersection, and the require-context option.

### Task 5: Complete secured read/write sessions

**Verifies:** access-policies#ac:every-leaf-method-enforced, access-policies#ac:batch-preflight-is-all-or-nothing, access-policies#ac:generated-id-under-allowed-collection, access-policies#ac:exact-id-does-not-match-incomplete-key
**Status:** complete

Wrap every current read and write leaf method, preflight whole batches, preserve return values, and authorize incomplete-key inserts at their target collection without requiring read permission.

### Task 6: Secured transactions and structured queries

**Verifies:** access-policies#ac:readwrite-transaction-is-secured, access-policies#ac:context-transaction-is-secured, access-policies#ac:query-only-does-not-grant-get, access-policies#ac:every-join-source-authorized, access-policies#ac:collection-group-is-explicit, access-policies#ac:opaque-query-is-explicit
**Status:** complete

Capture effective policies at transaction start, pass secured transactions and contexts, preserve options/retries, and authorize every structured query source while keeping collection-group and opaque-query grants explicit.

### Task 7: Memory-adapter integration and compatibility hardening

**Verifies:** access-policies#ac:unwrapped-suite-remains-green, access-policies#ac:memory-adapter-roundtrip
**Status:** complete

Exercise allowed and forbidden point, batch, generated-insert, query, and transaction flows through `dalgo2memory`; run the full DALgo suite and fix wrapper compatibility without changing unwrapped behavior.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
