# Idea: DALgo Schema Modification (DDL) Surface

**Status:** Approved
**Date:** 2026-05-12
**Owner:** alex
**Promotes To:** —
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we add (a) a portable schema-description vocabulary to DALgo so collections, fields, and indexes can be described in engine-neutral terms, and (b) a schema-modification (DDL) capability surface on top of that vocabulary so any backend can be provisioned from the same code path — CREATE COLLECTION, CREATE INDEX, primary-key declaration — without callers reaching into engine-specific drivers?

## Context

DALgo today is an ORM-shaped read/write abstraction: dal.Schema describes how keys map to fields and how data shapes resolve to keys (see dal/schema.go), but there is no notion of declaring a collection (table), an index, or a primary key. That is fine for backends like Firestore that are schema-on-read, but it leaves SQL backends (via dalgo2sql) and inGitDB (via dalgo2ingitdb) without a portable way to set up their storage. The trigger is the DataTug 'cross-engine db copy' Idea (see datatug-cli/spec/ideas/cross-engine-db-copy.md), which needs to auto-create target schemas from a source on the fly — that work cannot land cleanly until DALgo grows DDL.

**Scope refinement (post-initial-approval):** at Feature-spec time we realized the schema-description types (FieldDef, CollectionDef, IndexDef, Type, Precision, DefaultExpr) have multiple consumers beyond DDL execution — schema browsers, query builders, future read-side introspection. The types belong to a separate concern from the execution surface. The work therefore lands as TWO sub-packages in dalgo: `dbschema` (the description vocabulary, used by any tool that needs to talk about schema) and `ddl` (the execution surface, which imports `dbschema` and adds the SchemaModifier capability interface plus helper functions). Both DataTug and future tools depend on `dbschema`; only callers that actually mutate schema depend on `ddl`.

Backend-side persistence (where and how schema is stored on disk for file-based backends like inGitDB) is explicitly out of scope here — each backend owns its persistence layout. A separate Idea in `ingitdb/ingitdb-cli` is expected to capture inGitDB's choice (likely [INGR](https://ingr.io/) as the default file format, configurable per project — a Git-merge-friendly compact format).

## Recommended Direction

Ship two additive top-level sub-packages in dalgo (siblings of `dal/`, matching the existing convention used by `orm/`, `update/`, `recordset/`, etc.):

**`dbschema`** — the portable schema-description vocabulary AND the read-side (introspection) capability. Owns:

- **Types** (Tier 1, engine-neutral core): `FieldDef`, `CollectionDef`, `IndexDef`, `Type` (an `int8` enum with `String()`: Null/Bool/Int/Float/String/Bytes/Time/Decimal), `Precision`, and the sealed `DefaultExpr` interface (with `DefaultLiteral{Value any}` and `DefaultCurrentTimestamp{}` concretes). Reuses `dal.FieldName` for field-name identifiers. Optional `Length *int`/`Precision *Precision`/`Nullable bool`/`Default DefaultExpr`/`AutoIncrement bool` ride on `FieldDef` from day 1. `IndexDef` ships with `Unique bool` from day 1.
- **Reader** (Tier 1, engine-neutral capability): the `SchemaReader` capability interface (`ListCollections`, `DescribeCollection`, `ListIndexes`, plus optional `ListConstraints`/`ListReferrers` that may return `*NotSupportedError`). Drivers opt in via type assertion. Top-level helper functions (`dbschema.DescribeCollection(ctx, db, ref)` etc.) hide the type assertion.
- **Error model**: `NotSupportedError` typed error (Op, Backend, Reason) that wraps `dal.ErrNotSupported`. Used by both read and write sides.

**`ddl`** — the schema-modification execution surface. Imports `dbschema`. Defines:

- `SchemaModifier` capability interface with THREE methods: `CreateCollection(ctx, c, opts ...Option)` (creates table + inline indexes), `DropCollection(ctx, name, opts ...Option)` (drops table + its indexes), `AlterCollection(ctx, name, ops ...AlterOp)`. Index-level operations after initial creation go through `AlterCollection` as `AddIndex`/`DropIndex` AlterOps — `CreateIndex`/`DropIndex` are NOT standalone methods.
- Sealed `AlterOp` interface with six constructors: `AddField(FieldDef)`, `DropField(FieldName)`, `ModifyField(FieldName, FieldDef)` (full-replacement signature; driver diffs old vs new), `RenameField(old, new FieldName)`, `AddIndex(IndexDef)`, `DropIndex(name string)`. The unified composable model batches field-level and index-level alterations in one call; engines that support atomic multi-op ALTER (PostgreSQL) bundle them into one statement.
- `TransactionalDDL` capability interface (single method `SupportsTransactionalDDL() bool`) — mirrors `ConcurrencyAware`. Drivers that guarantee all-or-nothing for batch ops return `true`; others return `false` (or don't implement the interface). The default consumer expectation is **best-effort with partial success**; consumers MUST check the capability before relying on rollback.
- `PartialSuccessError` typed error returned when a non-transactional driver partway-fails a batch — carries `Applied []AlterOp`, `FirstFailed AlterOp`, `NotAttempted []AlterOp`, `Cause error`. Distinct from `dbschema.NotSupportedError` (which is shared with the read side).
- Top-level helper functions that type-assert against `SchemaModifier` and dispatch (3 helpers); functional-options for opt-in idempotency. `IfNotExists()` applies to `CreateCollection` and to Add* AlterOps (`AddField`, `AddIndex`). `IfExists()` applies to `DropCollection` and to Drop* AlterOps (`DropField`, `DropIndex`). `ModifyField` and `RenameField` accept `Option` for surface symmetry but the options are no-ops on those constructors (you usually want a real error if the target field is gone).

Non-implementers cause helpers to return `*dbschema.NotSupportedError`. Reference implementations land in dalgo2sql (SQLite AND PostgreSQL — both MVP) and dalgo2ingitdb.

**Three-tier composition.** The types in `dbschema` are designed to be **embedded** by engine-specific extensions in each driver repo, which in turn are embedded by application-specific wrappers in consumer repos:

```
Tier 1 (engine-neutral core, in dalgo):       dbschema.FieldDef { Name, Type, Length, Precision, Nullable, Default, AutoIncrement }
Tier 2 (engine-specific extensions):          dalgo2sql/dbschema.FieldDef { dbschema.FieldDef; DbType string; CharMaxLength *int; CharacterSet *string; ... }
Tier 3 (app-specific wrappers, in consumers): datatug.ColumnModel { sqlext.FieldDef; ByEnv; Checks; ... }
```

Same pattern for `IndexDef` (Tier 2 adds `IsClustered`, `IsXML`, `IsColumnStore`, `IsHash`, `IsPartial`, `Origin`, …) and `CollectionDef` (Tier 3 wraps for project-file storage and per-environment state).

**Migration scope** (across four repos):

- **dalgo**: this Feature batch — types + reader interfaces + writer interfaces (Tier 1).
- **dalgo2sql** (follow-up Feature in that repo): SQL-extension types (Tier 2 SQL); moved `sqliteschema/`, `mssqlschema/`, `sqlinfoschema/` from `datatug-cli/pkg/schemers/`.
- **dalgo2firestore** (follow-up Feature in that repo): Firestore-extension types (Tier 2 Firestore); moved `firestoreschema/` from `datatug-cli/pkg/schemers/`.
- **datatug-cli** (follow-up Feature): retire `pkg/datatug-core/schemer/` (move upstream); retire `pkg/schemers/*` (move to driver repos); convert `pkg/datatug-core/datatug/db_objects.go` types into Tier 3 wrappers that compose Tier 2.

The first three are sequenced spec-and-prototype-first via the existing `replace` directive pattern. datatug-cli migration follows once the upstream types land.

The package split is deliberate. `dbschema` is consumed by multiple downstream tools (schema browser, query builder, the cross-engine `db copy` writer, datatug's comparator) that have different mixes of read/write needs. Splitting types+reads from writes keeps consumer dependency surfaces minimal. `dal.Schema` already exists for the ORM concern in the parent package; placing the new types in a sub-package avoids confusable neighbors.

Sequencing is **spec-and-prototype-first**: this Idea is promoted to a SpecScore Feature tree in `dal-go/dalgo`, a working prototype lands on a DALgo branch, and `datatug-cli` activates its existing `replace` directive (`//replace github.com/dal-go/dalgo => ../../dal-go/dalgo`) to consume the branch while both sides iterate. DALgo merges and tags only when both sides are happy; `datatug-cli` removes the `replace` directive at that point and depends on the tagged release. Real-consumer feedback shapes the contract before it freezes.

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
- (Removed Non-Goal: per-`AlterOp` idempotency is now in scope. All six AlterOp constructors accept `opts ...Option`; `IfNotExists()` makes Add* AlterOps no-op when the target already exists; `IfExists()` makes Drop* AlterOps no-op when the target is absent. Modify/Rename accept `Option` for surface symmetry but the options are functionally no-ops on those constructors.)

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | A small portable type-set (Int, Float, String, Bytes, Bool, Time, Decimal, Null + optional length/precision hints) can faithfully describe the column types DataTug encounters on real SQLite, PostgreSQL, and inGitDB workloads. | Enumerate the type matrix on the Chinook fixture and on a representative DataTug user project; flag any type that needs a per-backend escape. |
| Must-be-true | DDL can be expressed as a capability interface (`SchemaModifier` or similar) that backends opt in to, with a structured `ErrUnsupported` for backends that do not. | Sketch the interface; confirm `dalgo2sql` + `dalgo2ingitdb` can both implement it and that the no-op case is one method on a stub. |
| Must-be-true | `dalgo2ingitdb` can persist tables, primary keys, and indexes in a Git-versionable file layout without requiring a schema-server (i.e. schema is itself a set of files in the repo). | Prototype the on-disk schema layout (`schema/<table>.yaml` or similar); confirm round-trip with a serial-baseline test. |
| Should-be-true | The DDL surface can land additively in `dal-go/dalgo` without breaking any existing consumer (datatug-cli, datatug-core, downstream projects). | Run the existing test suite plus a fresh `go test ./...` on datatug-cli/datatug-core after the change. |
| Should-be-true | SQLite's lack of column-drop / column-rename in older versions does not block the MVP. | ALTER is in scope (via `AlterCollection` + `AlterOp`); engines that physically cannot perform a given alteration return `*dbschema.NotSupportedError` per op. Drivers running modern SQLite (≥ 3.25) implement the full set; older versions return ErrNotSupported on DROP COLUMN / RENAME COLUMN. The MVP requires three engines (SQLite, PostgreSQL, inGitDB) to support enough of the surface for the cross-engine `db copy` consumer; gaps are tracked per-engine in the driver-repo Features. |
| Might-be-true | DDL eventually grows ALTER TABLE (add column, change type) for migration use cases. | Defer; collect evidence after the MVP ships and consumers ask. |
| Might-be-true | A separate `ddl` sub-package is preferable to hanging methods directly off `Database`. | Defer; decide at PR-design time based on import-cycle and consumer ergonomics. |


## SpecScore Integration

- **New Features this would create (in `dal-go/dalgo`):**
  - `spec/features/dbschema/` — umbrella for the schema-description vocabulary AND read-side capability, with 7 child Features: `types`, `default-expr`, `field-def`, `index-def`, `collection-def`, `errors` (shared `NotSupportedError` typed error), `schema-reader` (introspection capability + helpers).
  - `spec/features/ddl/` — umbrella for the execution surface, with 6 child Features: `schema-modifier` (3-method interface: `CreateCollection`, `DropCollection`, `AlterCollection`), `alter-ops` (sealed `AlterOp` interface + six constructors `AddField` / `DropField` / `ModifyField` / `RenameField` / `AddIndex` / `DropIndex`, each accepting `opts ...Option` for opt-in idempotency), `transactional-ddl` (capability interface for atomicity guarantees), `options` (IfNotExists/IfExists for `CreateCollection`/`DropCollection` AND all six AlterOps), `operations` (three top-level helpers), `errors` (`PartialSuccessError` typed error for partway-failed batches). `dbschema.NotSupportedError` is shared with the read side and lives there; `ddl` imports it.
  - Reference-implementation features in sibling repos (separate Features, follow-up work): `dalgo2sql/spec/features/dbschema/` (Tier-2 SQL extensions + per-engine SchemaReader implementations + SchemaModifier impl), `dalgo2firestore/spec/features/dbschema/` (Firestore SchemaReader impl), and inGitDB driver adoption.
- **Existing Features affected:**
  - None today — DALgo has no other spec features yet. When `dal/schema.go` (the ORM-shape `Schema`) gets specified, that spec will cross-reference this one to make the separation of concerns explicit.
- **Dependencies:**
  - None upstream. This is the foundational change.
- **Downstream consumer:** [`datatug/datatug-cli` Idea `cross-engine-db-copy`](https://github.com/datatug/datatug-cli/blob/main/spec/ideas/cross-engine-db-copy.md) is the driving consumer and the one that validates the surface. (Synchestra will manage `promotes_to`; do not edit manually.)

## Open Questions


---
*This document follows the https://specscore.md/idea-specification*
