---
format: https://specscore.md/idea-specification
status: Approved
---

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

Add a one-method capability interface `dal.ConcurrencyAware { SupportsConcurrentConnections() bool }` to DALgo and **embed it into the live `dal.DB` interface** alongside the existing `TransactionCoordinator` and `ReadSession` compositions. Every `DB` implementation therefore answers the question — there is no "unknown" state. Two reusable zero-value embeddable structs in `dal/` — `NoConcurrency` (returns false) and `ConcurrencyAvailable` (returns true) — let drivers opt into the conservative or permissive answer with a single embedded field and no method body. The previously-considered "opt-in via type assertion" pattern (Go-stdlib style) was reconsidered at Feature-spec time: the dalgo `DB` interface already embeds capability-like interfaces as required composition, every driver has a concurrency answer to give (it is not an optional feature), and the embed pattern dissolves the helper-function question by removing the type assertion. Reference implementations in MVP: dalgo2sql returns false for SQLite, true for PostgreSQL; dalgo2ingitdb returns false until stress-tested. The interface lives in `dal/` (not a sub-package) because it is a single method, broadly applicable, and there is no second capability today to justify a sibling package.

## Alternatives Considered

- **General `Capabilities() map[string]bool` (or struct).** Rejected — there is one capability today. A map invites adding capabilities ad-hoc with string keys (no compile-time check) and over-generalizes ahead of need. If a second capability appears, add a second interface (`dal.TransactionAware`, etc.); Go-style capability interfaces compose without a registry.
- **Static lookup table in the consumer.** ("I know SQLite doesn't, I know Postgres does.") Lost because every consumer would maintain its own table — defeating the abstraction. The consumer-side table is exactly what this Idea removes.
- **Read capability from a connection URL scheme.** ("`sqlite://...` → false, `postgres://...` → true.") Rejected — the URL scheme is a transport concern, not a capability one. SQLite over WAL with a single connection has different semantics than SQLite in WAL-shared mode; the URL doesn't tell you which.
- **Opt-in capability interface (Go-stdlib `Hijacker`-style).** Originally recommended at Idea-time; revisited at Feature-spec time and rejected because (a) dalgo's `DB` interface already embeds capability-like interfaces as required composition, (b) every driver has a concurrency answer ("unknown" is a fake state), and (c) opt-in introduces a helper-function question that embedding dissolves. See Feature `concurrency-capability` for the full reasoning.

## MVP Scope

Land `dal.ConcurrencyAware` plus its two embeddable helper structs (`NoConcurrency`, `ConcurrencyAvailable`) in dal-go/dalgo; embed `ConcurrencyAware` into `dal.DB`. Update in-tree mocks under `mocks/` to embed `NoConcurrency` as the conservative default. Implement on dalgo2sql/sqlite (false), dalgo2sql/postgres (true), and dalgo2ingitdb (false) — driver-side adoption is a follow-up per-repo. Godoc covers: the contract, the stability guarantee, the deliberate absence of read/write asymmetry, and the embed-helper pattern. Verification in dalgo: in-package Go tests against mock `DB` types that embed each helper struct. Downstream verification: datatug-cli's `--parallel-streams` logic compiles against the new interface and behaves correctly for the three MVP backends once each driver adopts.

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
| Should-be-true (INVALIDATED at Feature-spec time) | ~~Treating "interface not implemented" as "unknown, default to no" is a safe default.~~ Embedding into `dal.DB` makes every implementation answer — there is no "unknown" state. | n/a — superseded by Feature decision. |
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

All four original questions were resolved when this Idea was promoted to the Feature spec at [`spec/features/concurrency-capability/`](../features/concurrency-capability/README.md):

- ~~Interface name~~ → `ConcurrencyAware` (kept).
- ~~`Database` vs `Connection`~~ → `dal.DB` (live interface; `dal.Connection` is dead commented-out code).
- ~~Helper function~~ → Not needed. Embedding into `DB` removes the type assertion; helper structs `NoConcurrency`/`ConcurrencyAvailable` cover the embed-it-for-me case.
- ~~Read/write asymmetry godoc~~ → Required by Feature REQ `godoc`.

---
*This document follows the https://specscore.md/idea-specification*
