# Idea: DALgo Concurrency Capability Interface

**Status:** Approved
**Date:** 2026-05-12
**Owner:** alex
**Promotes To:** —
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we let DALgo callers know whether a backend supports concurrent connections, so they can size their workers and stream pools without engine-specific knowledge?

## Context

Consumers of DALgo today must hard-code per-engine knowledge to decide whether to run with concurrent connections — SQLite serializes writers, PostgreSQL is happy with many, dalgo2ingitdb's safety under concurrent writes is unproven. The trigger is the datatug-cli 'cross-engine db copy' Idea (datatug-cli/spec/ideas/cross-engine-db-copy.md): its --parallel-streams flag defaults to runtime.NumCPU()-1 but must cap to 1 when either the source or target backend doesn't support concurrent connections. Today the consumer has no portable way to ask. This Idea adds that small surface so consumers don't have to maintain their own engine-by-engine concurrency table.

## Recommended Direction

Add a single optional capability interface to DALgo: 'dal.ConcurrencyAware' with one method 'SupportsConcurrentConnections() bool'. Drivers opt in by implementing it on their Database (or Connection) type. Callers do a type assertion: 'if a, ok := db.(dal.ConcurrencyAware); ok && a.SupportsConcurrentConnections() { … }'. Drivers that don't implement the interface are treated as 'unknown', which conservative callers should treat as 'no'. Reference implementations in MVP: dalgo2sql returns false for SQLite, true for PostgreSQL; dalgo2ingitdb returns false until stress-tested. The interface lives in 'dal/' (not a sub-package) because it's a single method that's broadly applicable, not a feature family — analogous to io.Closer being in 'io' rather than 'io/closer'.

## Alternatives Considered

- **General `Capabilities() map[string]bool` (or struct).** Rejected — there is one capability today. A map invites adding capabilities ad-hoc with string keys (no compile-time check) and over-generalizes ahead of need. If a second capability appears, add a second interface (`dal.TransactionAware`, etc.); Go-style capability interfaces compose without a registry.
- **Static lookup table in the consumer.** ("I know SQLite doesn't, I know Postgres does.") Lost because every consumer would maintain its own table — defeating the abstraction. The consumer-side table is exactly what this Idea removes.
- **Read capability from a connection URL scheme.** ("`sqlite://...` → false, `postgres://...` → true.") Rejected — the URL scheme is a transport concern, not a capability one. SQLite over WAL with a single connection has different semantics than SQLite in WAL-shared mode; the URL doesn't tell you which.
- **Make `ConcurrencyAware` a required method on `dal.Database`.** Rejected — touches every existing driver. Capability interfaces should be opt-in; non-implementers default to "unknown."

## MVP Scope

Land 'dal.ConcurrencyAware' (one method, one line in the interface) in dal-go/dalgo. Implement on dalgo2sql/sqlite (false), dalgo2sql/postgres (true), and dalgo2ingitdb (false). Document the 'doesn't implement = unknown = treat as no' convention in the godoc on the interface. Verification: datatug-cli's --parallel-streams logic compiles against the new interface and behaves correctly for the three MVP backends.

## Not Doing (and Why)

- A general Capabilities() map or feature-flag registry — over-engineered for what is currently one capability
- Read-vs-write concurrency distinction in MVP — single boolean suffices; refine later if SQLite-style 'concurrent readers, serialized writers' becomes a real consumer ask
- Concurrent-connection LIMIT advertisement (e.g. 'supports up to N') — boolean is enough for the current consumer
- Auto-tuning of stream counts inside DALgo — that's the consumer's job; DALgo only reports capability

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | A single boolean ("supports concurrent connections — yes/no") is sufficient information for current and near-future consumers to make sound concurrency decisions. | Trace the cross-engine-db-copy `--parallel-streams` logic end-to-end with the boolean; confirm it produces the right cap for SQLite, Postgres, and inGitDB. |
| Must-be-true | Consumers that need to make a concurrency decision can reach the `Database` (or `Connection`) handle at the moment of decision, so a type assertion is feasible. | Audit the cross-engine-db-copy decision point; confirm `db` is in scope where worker pool size is chosen. |
| Should-be-true | Treating "interface not implemented" as "unknown, default to no" is a safe default and matches consumer expectations. | Document the convention prominently in godoc; review with maintainer for any consumer that would prefer "unknown = yes." |
| Should-be-true | Drivers' answers are stable for the lifetime of a `Database` value (i.e. the answer doesn't change between calls on the same handle). | Verify on Postgres and SQLite — neither's mode flips at runtime per-connection without a fresh handle. |
| Might-be-true | A second capability (`TransactionAware`, `BatchAware`, …) will appear soon enough to justify a shared design. | Defer; each capability is its own interface; we don't pre-design a registry. |
| Might-be-true | Read-vs-write asymmetry (SQLite-style "concurrent readers, serialized writers") will become a real consumer ask. | Defer; refine the interface (or add a sibling) only when a consumer needs the distinction. |


## SpecScore Integration

- **New Features this would create:**
  - `spec/features/concurrency-capability/` — the `dal.ConcurrencyAware` interface, its godoc convention ("not implemented = unknown = treat as no"), and reference implementations in `dalgo2sql` and `dalgo2ingitdb`.
- **Existing Features affected:**
  - None today — DALgo has no other spec features yet. The sibling Idea `dalgo-schema-modification` is unaffected; the two surfaces are orthogonal.
- **Dependencies:**
  - None upstream. This is a tiny, additive change.
- **Downstream consumer (informational; Synchestra manages `promotes_to`):**
  - [`datatug/datatug-cli` Idea `cross-engine-db-copy`](https://github.com/datatug/datatug-cli/blob/main/spec/ideas/cross-engine-db-copy.md) — `--parallel-streams` defaults to `runtime.NumCPU() - 1` but caps to 1 when either side returns `false` from `ConcurrencyAware.SupportsConcurrentConnections()`. This is the consumer driving the Idea.

## Open Questions

- **Interface name.** `ConcurrencyAware` reads cleanly but does not name the *thing* being queried. Alternatives: `ConcurrentConnectionsSupporter` (verbose), `Concurrent` (terse, lossy). Pick at Feature-spec time.
- **Hang on `Database` or `Connection`?** Both are reasonable. `Database` is the more common consumer entry point; `Connection` is more precise (a `Database` could in principle have differently-configured connections). Default to `Database` unless a real consumer needs per-connection granularity.
- **Default for unknown.** Godoc convention is "not implemented = treat as no." Should the package ship a `dal.SupportsConcurrentConnections(db) bool` helper that encapsulates the type assertion AND the default, so consumers don't replicate the convention?
- **Read vs write asymmetry.** Defer per Not-Doing, but worth a single godoc paragraph explaining why the boolean intentionally doesn't distinguish.

---
*This document follows the https://specscore.md/idea-specification*
