# Idea: First-class INNER/LEFT joins in dal's query model

**Status:** Approved
**Date:** 2026-06-05
**Owner:** alexander.trakhimenok
**Promotes To:** —
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we let dal's query AST express INNER and LEFT joins losslessly, so callers and DTQL can build, read, and render a two-table join without field ambiguity?

## Context

Triggered by the sidekick seed 'dalgo-needs-first-class-join-support-in-its-query-model' (captured during spec/features/dtql): DTQL had to defer joins because dal.StructuredQuery cannot express one losslessly. Three concrete gaps exist in dal today: (1) JoinedSource has no join-type distinction (dal/q_recordset_source.go) - you cannot tell INNER from LEFT; (2) JoinedSource.on is unexported with no public constructor, so external callers cannot populate the ON clause; (3) FieldRef is {name, isID} with no source qualifier (dal/q_field_ref.go), so with two tables every field in ON/WHERE/columns/ORDER BY is ambiguous. dalgo2memory is a package in this same module and is the in-repo executing adapter (today it reads only q.From().Base().Name() and ignores Joins()). SQLite execution lives in the sibling repos dalgo2sql and dalgo2sqlite, which consume dal and are out of scope here.

## Recommended Direction

Add the three missing AST pieces to dal additively, and prove the model end-to-end by executing joins in dalgo2memory. (1) A join-type enum carrying INNER and LEFT now, with RIGHT/FULL/CROSS reserved but rejected. (2) A public constructor NewJoinedSource(src, joinType, on...) mirroring the existing NewGroupCondition fix, so callers outside dal can build a join with its ON clause. (3) A source qualifier folded directly into NewFieldRef, whose signature becomes NewFieldRef(src, name) - src may be empty, which resolves to the single From base so single-source queries stay simple. This is an intentional breaking signature change, accepted at this pre-stable stage in exchange for one unified field constructor instead of two. ON conditions are equality-only (equi-joins) for the MVP, reusing the existing Comparison node with qualified FieldRefs on both sides. The end-to-end proof is dalgo2memory: extend it to walk q.From().Joins(), perform a nested-loop join over the in-memory maps, and resolve qualified FieldRefs to the correct source - returning correct rows for INNER and the null-filled right side for LEFT. This is the smallest change that makes a join expressible AND runnable in one repo, while leaving SQL rendering (dalgo2sql/dalgo2sqlite) and DTQL join serialization as clean follow-ons that the new AST unblocks.

## Alternatives Considered

- **Add a second constructor `Field(src, name)` and leave `NewFieldRef(name)` untouched (non-breaking).** Avoids migrating any existing call site. Lost because it leaves two constructors for one concept and a `FieldRef` that is sometimes qualifiable and sometimes not; with breaking changes acceptable at this pre-stable stage, folding `src` into `NewFieldRef(src, name)` — empty allowed — is the simpler long-term API.
- **Prove execution on SQLite first (`dalgo2sql`/`dalgo2sqlite`).** Closer to a "real" relational join. Lost because those are separate repos that consume `dal`: they cannot even compile against a join AST that does not exist yet, so this inverts the dependency order. `dalgo2memory` lives in this module and proves the model in one repo; SQLite becomes a clean follow-on.
- **Skip `dal` and let DTQL model joins in its own YAML layer.** Would unblock DTQL serialization fastest. Lost because it pushes join semantics into a consumer, so every other adapter (memory, SQL, datastore) would reinvent them incompatibly — the exact lossy-translation problem the seed was filed to prevent. Joins belong in the shared AST.

## MVP Scope

A short spike: dal can express a single INNER and a single LEFT equi-join between two root collections, with source-qualified fields in the ON clause, built entirely from outside the dal package; and dalgo2memory executes both via nested-loop, returning correct rows for INNER and null-filled right-side rows for LEFT. Verified by a table test over two small in-memory collections. All existing dal + dalgo2memory + dtql call sites are migrated to the new NewFieldRef(src, name) signature and the full suite returns green, proving the breaking change is fully absorbed.

## Not Doing (and Why)

- RIGHT / FULL / CROSS joins - the enum reserves them but INNER+LEFT is the 90% case per the seed
- Non-equality or arbitrary ON conditions - equi-join (field = field) only for the MVP
- SQL rendering and execution (dalgo2sql, dalgo2sqlite) - separate repos, follow-on once the dal AST lands
- Requiring a non-empty source on every field - empty src stays valid and resolves to the single From base, so single-source queries need no source name
- Multiple or chained joins (3+ tables) - single join (two tables) for the MVP
- DTQL serialization of joins - follow-on, unblocked once dal can express the join

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | Changing `NewFieldRef` to `NewFieldRef(src, name)` is a bounded, mechanical migration of every call site in `dal`, `dalgo2memory`, `dtql`, `dbschema`, and end2end. | Grep all `NewFieldRef(` call sites, update them, and run the full `dal` + `dalgo2memory` + `dtql` suites; all must return green. |
| Must-be-true | An equi-join expressed purely through public `dal` constructors is executable by a consumer without reaching into unexported state. | Build an INNER and a LEFT join entirely from the `dalgo2memory` package (outside `dal`) and execute it; if any field needs unexported access, the public API is incomplete. |
| Should-be-true | A nested-loop join over the in-memory maps is enough to prove correctness, including LEFT-join null-filling of the right side. | Table test over two small collections: assert INNER drops unmatched rows and LEFT keeps left rows with nil right fields. |
| Should-be-true | Source qualification can reuse `RecordsetSource.Alias()`/`Name()` as the key, and an empty `src` unambiguously resolves to `From().Base()` for single-source queries. | Prototype `NewFieldRef` resolving against `From().Base()` (empty `src`) and `Joins()[i]` by alias/name; confirm no new aliasing machinery is required. |
| Might-be-true | Equality-only ON clauses cover the dominant real join need, so richer ON (ranges, AND/OR) can wait. | Review intended consumers (DTQL, datatug) for any non-equi-join requirement before widening scope. |


## SpecScore Integration

- **New Features this would create:** `dal` join AST (join-type enum, `NewJoinedSource`, qualified `Field`); `dalgo2memory` nested-loop join execution. Likely two Features.
- **Existing Features affected:** `dtql` — its subset gate currently rejects any query with joins; unaffected until a later cycle extends DTQL to serialize the new AST.
- **Dependencies:** Upstream — sidekick seed `dalgo-needs-first-class-join-support-in-its-query-model`. Downstream (unblocked by this) — SQL rendering in `dalgo2sql`/`dalgo2sqlite`, and DTQL join serialization.

## Open Questions

- Ergonomics of the new signature: is positional `NewFieldRef(src, name)` enough, or is a helper warranted for the common empty-`src` case (e.g. a thin single-arg wrapper)? — settle at spec time.
- How a source is named in the qualifier: reuse `RecordsetSource.Alias()`/`Name()` as the key, or introduce explicit per-query aliases for self-joins?
- Shape of the join-type enum: exported typed constant (e.g. `dal.JoinInner`) vs. a string — and whether reserved-but-unsupported types (RIGHT/FULL/CROSS) reject at construction or at execution.
- Does the additive qualifier need to propagate into `OrderBy`/`Columns` rendering in `dalgo2memory` for the MVP, or only into ON resolution?

---
*This document follows the https://specscore.md/idea-specification*
