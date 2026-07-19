---
format: https://specscore.md/idea-specification
status: Implemented
---

# Idea: Portable hierarchical access policies

**Status:** Implemented
**Date:** 2026-07-17
**Owner:** alex
**Promotes To:** access-policies
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might DALgo let applications grant least-privilege data capabilities once and enforce them consistently across every adapter, transaction, batch, and query?

## Context

DALgo gives the same `dal.DB` and transaction API to code using Firestore,
SQL, memory, files, or Git-backed storage, but today that handle carries the
full authority of its adapter. In an extension host such as Sneat, a module
that should write only beneath `/ext/<id>/...` or
`/spaces/<space-id>/ext/<id>/...` can accidentally mutate a root collection.
The convention is neither enforced in production nor reproduced reliably by
the in-memory test adapter.

The same gap affects tenant-scoped services, ingestion endpoints, analytics,
and support tools. A logger may need insert-only access; a support tool may be
allowed to get a known record but forbidden to enumerate the collection; an
analytics tool such as DataTug may need query-only access today and constrained
fields, predicates, projections, or indexes later.

## Recommended Direction

Add hierarchical, adapter-independent policies over structural DAL paths and explicit operations. Compose database and context policies by intersection, keep read/write/query permissions independent, reuse the matcher for audit selection, and serialize declarative policies as versioned YAML or JSON through storage-neutral codecs.

Within one policy, the most-specific path wins, enabling both common shapes:

```text
Allow /spaces/*/**
  Deny /spaces/*/private/**

Deny /spaces/*/**
  Allow /spaces/*/ext/trackus/**
```

Separate policies intersect, so a context capability can narrow but never
reopen a database-level denial. Operations remain explicit: `Get`, `Exists`,
`Query`, `Insert`, `Set`, `Update`, `Delete`, and reserved `Truncate`. Thus an
append-only log can allow only `Insert`, while a known-key secret store can
allow `Get` and deny `Query`.

The same hierarchy can classify audit events with independent verbs:

```text
Audit mutations under /users/** and /transactions/**
Ignore mutations under /auditLog/**
```

YAML is the canonical authored format, JSON represents the same versioned data
model, and codecs operate on streams or bytes rather than filesystem paths so
policies can live in object storage, a database, Git, embedded assets, or files.

## Alternatives Considered

- **Application checks and save hooks.** Too easy to omit, do not cover reads or
  every batch/query path, and vary between adapters.
- **Database-native security only.** Strong as defense in depth but not portable
  across DALgo adapters and absent from in-memory tests.
- **Deny always wins at every ancestor.** Simple but prevents useful
  `Deny(path).Allow(subpaths...)` policies. Most-specific-wins inside one policy
  plus intersection between policies preserves both hierarchy and hard bounds.
- **HCL as the primary document format.** Attractive for a configuration
  language, but expressions, variables, functions, and evaluation contexts add
  semantics the policy model does not need. A codec extension can add HCL when
  a concrete consumer requires it.

## MVP Scope

A secured DALgo wrapper; Get, Exists, Query, Insert, Set, Update, Delete, and reserved Truncate operations; hierarchical allow/deny; context capture; batch and transaction enforcement; explicit collection-group and opaque-query handling; audit/ignore classification; YAML and JSON codecs.

## Not Doing (and Why)

- Database-native IAM or hostile plug-in sandboxing — the boundary covers operations routed through secured DALgo handles.
- Audit persistence and delivery guarantees — the MVP classifies events but leaves sinks to applications.
- Field-level and WHERE/index-aware query constraints — the operation model preserves room for follow-up features.

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | Every DAL session and transaction operation can be intercepted without adapter changes. | Wrap the public DAL interfaces and prove delegation/denial with recording fakes and `dalgo2memory`. |
| Must-be-true | Hierarchical precedence is deterministic and declaration-order independent. | Evaluate reversed rule tables, wildcard ties, and parent/child overrides. |
| Must-be-true | Context restrictions cannot be discarded inside a transaction. | Capture policies at transaction start and verify operations using `context.Background()` remain restricted. |
| Should-be-true | One declarative document model round-trips through YAML and JSON. | Decode equivalent fixtures and compare decisions and explanations. |
| Might-be-true | Query policies will grow field, predicate, projection, index, and cost constraints. | Preserve `Query` as a distinct operation and the structured query on the authorization request. |


## SpecScore Integration

- **New Features this would create:** [access-policies](../features/access-policies/README.md)
- **Existing Features affected:** none
- **Dependencies:** none

## Open Questions

None at this time.
