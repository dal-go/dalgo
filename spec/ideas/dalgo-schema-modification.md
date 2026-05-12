# Idea: DALgo Schema Modification (DDL) Surface

**Status:** Approved
**Date:** 2026-05-12
**Owner:** alex
**Promotes To:** —
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we add a portable schema-modification (DDL) surface to DALgo so any backend can be provisioned from the same code path — CREATE TABLE, CREATE INDEX, primary-key declaration — without callers reaching into engine-specific drivers?

## Context

DALgo today is an ORM-shaped read/write abstraction: dal.Schema describes how keys map to fields and how data shapes resolve to keys (see dal/schema.go), but there is no notion of declaring a table, an index, or a primary key. That is fine for backends like Firestore that are schema-on-read, but it leaves SQL backends (via dalgo2sql) and inGitDB (via dalgo2ingitdb) without a portable way to set up their storage. The trigger is the DataTug 'cross-engine db copy' Idea (see datatug-cli/spec/ideas/cross-engine-db-copy.md), which needs to auto-create target schemas from a source on the fly — that work cannot land cleanly until DALgo grows DDL. The same surface unlocks fixture seeding for tests and one-shot provisioning for any tool built on DALgo.

Backend-side persistence (where and how schema is stored on disk for file-based backends like inGitDB) is explicitly out of scope here — each backend owns its persistence layout. A separate Idea in `ingitdb/ingitdb-cli` is expected to capture inGitDB's choice (likely [INGR](https://ingr.io/) as the default file format, configurable per project — a Git-merge-friendly compact format).

## Recommended Direction

Add a small, additive DDL surface as a new sub-package `dal/ddl` — explicitly separate from `dal.Schema` (which is ORM-shape: key↔field mapping). The sub-package owns `ddl.Modifier` (capability interface), `ddl.Table`, `ddl.Column`, `ddl.Index`, and helper functions like `ddl.CreateTable(ctx, db, t)` that internally type-assert `db` against `ddl.Modifier`. The MUST surface covers: declaring a table with typed columns; declaring a primary key (single or composite); declaring secondary indexes (with `Unique bool` from day 1); dropping a table. Types use a small portable type-set (Int, Float, String, Bytes, Bool, Time, Decimal, Null). `ddl.Column` ships from day 1 with optional `Length *int` (for String/Bytes) and `Precision *Precision` ({Total, Scale} for Decimal) hints — backends honor them if they care (PostgreSQL), ignore them if they don't (SQLite, inGitDB). This avoids a public-API break later when PostgreSQL→PostgreSQL fidelity becomes a real requirement. Backends opt in by implementing `ddl.Modifier`. Drivers that cannot or do not implement DDL return an error that satisfies `errors.Is(err, ddl.ErrUnsupported)` — typically a `*ddl.UnsupportedError` (struct carrying `Op`, `Backend`, `Reason`) that unwraps to the `ErrUnsupported` sentinel. Callers can do a coarse `errors.Is` check or `errors.As` the typed error for diagnostic detail. Drivers that simply do not satisfy the `ddl.Modifier` interface cause `ddl.CreateTable` (and siblings) to return `ErrUnsupported` on the caller's behalf. Reference implementations land in dalgo2sql (SQLite AND PostgreSQL — both MVP) and dalgo2ingitdb. The surface is designed by writing the consumer first (the datatug-cli `db copy` command) and the smallest API that makes that consumer compile, then implemented behind it. Sequencing is **spec-and-prototype-first**: this Idea is promoted to a SpecScore Feature in `dal-go/dalgo`, a working prototype lands on a DALgo branch, and `datatug-cli` activates its existing `replace` directive (`//replace github.com/dal-go/dalgo => ../../dal-go/dalgo`) to consume the branch while both sides iterate. DALgo merges and tags only when both sides are happy; `datatug-cli` removes the `replace` directive at that point and depends on the tagged release. Real-consumer feedback shapes the DDL contract before it freezes. Sub-package placement (rather than putting these types in `dal/`) is deliberate: `dal.Schema` already exists for the ORM concern, and crowding `dal/` with `dal.SchemaModifier` / `dal.Table` / `dal.Column` would create confusable neighbors. The `ddl` package name carries the namespace.

## Alternatives Considered

- **Extend `dal.Schema` with table/column/index methods.** Rejected because `dal.Schema` is the ORM-shape (key↔field mapping) and is consumed at every read/write call. Loading it with DDL responsibilities conflates two concerns and inflates the cost of every consumer. Keep the existing `Schema` ORM-shaped; put DDL on a new, opt-in surface.
- **Skip a portable type system; pass raw engine-specific column DDL strings.** Faster to ship, but breaks the core DALgo promise: a backend swap should not require call-site changes. A consumer that wants to provision a table on either SQLite or inGitDB would have to know both dialects — which is exactly what DALgo exists to abstract away.
- **Defer DDL entirely; require pre-provisioned targets in DataTug.** Rejected on the consumer side (see `cross-engine-db-copy`): every E2E fixture would need a hand-rolled schema-setup, defeating the point of an automation primitive.

## MVP Scope

A focused PR on dal-go/dalgo adding the DDL surface (table create/drop, primary key, secondary index, portable type set), plus reference implementations in dalgo2sql (SQLite AND PostgreSQL) and dalgo2ingitdb. All three backends are MVP — no fast-follow. Verification: a single integration test in this repo round-trips a 3-table fixture (create-tables, insert-rows, read-back) on each of the three backends. Backends beyond these (Firestore, dalgo2fs, mocks) return ErrUnsupported until a consumer needs them.

## Not Doing (and Why)

- Schema migration or diffing — out of scope; this is one-shot DDL, not reconcile-target-with-source
- Foreign-key declarations in MVP — defer; cross-engine FK semantics are a rabbit hole, datatug copy does not need them for E2E fixtures
- Views, triggers, stored procedures — out of scope, possibly forever for DALgo
- Per-engine escape hatches (raw DDL passthrough) — out of scope; would defeat portability
- All current DALgo backends in MVP — only dalgo2sql (SQLite + PostgreSQL) and dalgo2ingitdb are required; others (Firestore, dalgo2fs, mocks) can return ErrUnsupported until needed

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | A small portable type-set (Int, Float, String, Bytes, Bool, Time, Decimal, Null + optional length/precision hints) can faithfully describe the column types DataTug encounters on real SQLite, PostgreSQL, and inGitDB workloads. | Enumerate the type matrix on the Chinook fixture and on a representative DataTug user project; flag any type that needs a per-backend escape. |
| Must-be-true | DDL can be expressed as a capability interface (`SchemaModifier` or similar) that backends opt in to, with a structured `ErrUnsupported` for backends that do not. | Sketch the interface; confirm `dalgo2sql` + `dalgo2ingitdb` can both implement it and that the no-op case is one method on a stub. |
| Must-be-true | `dalgo2ingitdb` can persist tables, primary keys, and indexes in a Git-versionable file layout without requiring a schema-server (i.e. schema is itself a set of files in the repo). | Prototype the on-disk schema layout (`schema/<table>.yaml` or similar); confirm round-trip with a serial-baseline test. |
| Should-be-true | The DDL surface can land additively in `dal-go/dalgo` without breaking any existing consumer (datatug-cli, datatug-core, downstream projects). | Run the existing test suite plus a fresh `go test ./...` on datatug-cli/datatug-core after the change. |
| Should-be-true | SQLite's lack of column-drop / column-rename in older versions does not block the MVP (MVP needs CREATE/DROP, not ALTER). | Confirm scope: ALTER stays out. CREATE TABLE + DROP TABLE + CREATE INDEX is all MVP requires. |
| Might-be-true | DDL eventually grows ALTER TABLE (add column, change type) for migration use cases. | Defer; collect evidence after the MVP ships and consumers ask. |
| Might-be-true | A separate `dal/ddl` sub-package is preferable to hanging methods directly off `Database`. | Defer; decide at PR-design time based on import-cycle and consumer ergonomics. |


## SpecScore Integration

- **New Features this would create:**
  - `spec/features/ddl/` — the portable DDL surface in `dal-go/dalgo` (type set, `SchemaModifier` capability interface, table/index operations, `ErrUnsupported`).
  - Reference-implementation features in sibling repos: `dalgo2sql/spec/features/ddl/` and `dalgo2ingitdb`'s spec tree (paths TBD when those repos adopt SpecScore).
- **Existing Features affected:**
  - None today — DALgo has no other spec features yet. When `dal/schema.go` (the ORM-shape `Schema`) gets specified, that spec will cross-reference this one to make the separation of concerns explicit.
- **Dependencies:**
  - None upstream. This is the foundational change.
- **Downstream consumer:** [`datatug/datatug-cli` Idea `cross-engine-db-copy`](https://github.com/datatug/datatug-cli/blob/main/spec/ideas/cross-engine-db-copy.md) is the driving consumer and the one that validates the surface. (Synchestra will manage `promotes_to`; do not edit manually.)

## Open Questions


---
*This document follows the https://specscore.md/idea-specification*
