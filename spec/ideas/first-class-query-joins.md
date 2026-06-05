# Idea: First-class INNER/LEFT joins in dal's query model

**Status:** Draft
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

Add the three missing AST pieces to dal additively, and prove the model end-to-end by executing joins in dalgo2memory. (1) A join-type enum carrying INNER and LEFT now, with RIGHT/FULL/CROSS reserved but rejected. (2) A public constructor NewJoinedSource(src, joinType, on...) mirroring the existing NewGroupCondition fix, so callers outside dal can build a join with its ON clause. (3) A source-qualified field constructor Field(src, name) added alongside today's unqualified NewFieldRef - the qualifier is optional, so every existing single-table call site and adapter keeps working unchanged when it is empty. ON conditions are equality-only (equi-joins) for the MVP, reusing the existing Comparison node with qualified FieldRefs on both sides. The end-to-end proof is dalgo2memory: extend it to walk q.From().Joins(), perform a nested-loop join over the in-memory maps, and resolve qualified FieldRefs to the correct source - returning correct rows for INNER and the null-filled right side for LEFT. This is the smallest change that makes a join expressible AND runnable in one repo, while leaving SQL rendering (dalgo2sql/dalgo2sqlite) and DTQL join serialization as clean follow-ons that the new AST unblocks.

## Alternatives Considered

- **Make the field qualifier mandatory (rewrite `FieldRef` to always carry a source).** Cleaner, unambiguous AST with no "empty qualifier" special case. Lost because it breaks every existing single-table call site plus `dalgo2memory`, `dtql`, and `dbschema` that build or read `FieldRef` today — a large, risky blast radius for a feature whose stated sweet spot is the two-table 90% case. Additive `Field(src, name)` buys the same expressiveness without the break.
- **Prove execution on SQLite first (`dalgo2sql`/`dalgo2sqlite`).** Closer to a "real" relational join. Lost because those are separate repos that consume `dal`: they cannot even compile against a join AST that does not exist yet, so this inverts the dependency order. `dalgo2memory` lives in this module and proves the model in one repo; SQLite becomes a clean follow-on.
- **Skip `dal` and let DTQL model joins in its own YAML layer.** Would unblock DTQL serialization fastest. Lost because it pushes join semantics into a consumer, so every other adapter (memory, SQL, datastore) would reinvent them incompatibly — the exact lossy-translation problem the seed was filed to prevent. Joins belong in the shared AST.

## MVP Scope

A short spike: dal can express a single INNER and a single LEFT equi-join between two root collections, with source-qualified fields in the ON clause, built entirely from outside the dal package; and dalgo2memory executes both via nested-loop, returning correct rows for INNER and null-filled right-side rows for LEFT. Verified by a table test over two small in-memory collections. The entire existing dal + dalgo2memory + dtql test suite stays green, proving the additive FieldRef qualifier broke nothing.

## Not Doing (and Why)

- RIGHT / FULL / CROSS joins - the enum reserves them but INNER+LEFT is the 90% case per the seed
- Non-equality or arbitrary ON conditions - equi-join (field = field) only for the MVP
- SQL rendering and execution (dalgo2sql, dalgo2sqlite) - separate repos, follow-on once the dal AST lands
- Mandatory field qualification - Field(src,name) is additive; unqualified NewFieldRef stays valid so the single-table path is untouched
- Multiple or chained joins (3+ tables) - single join (two tables) for the MVP
- DTQL serialization of joins - follow-on, unblocked once dal can express the join

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | Adding an optional source qualifier to `FieldRef` is non-breaking — when empty it behaves exactly as today for single-table queries, adapters, and DTQL. | Make the qualifier additive, then run the full existing `dal` + `dalgo2memory` + `dtql` suites unchanged; all must stay green with zero call-site edits. |
| Must-be-true | An equi-join expressed purely through public `dal` constructors is executable by a consumer without reaching into unexported state. | Build an INNER and a LEFT join entirely from the `dalgo2memory` package (outside `dal`) and execute it; if any field needs unexported access, the public API is incomplete. |
| Should-be-true | A nested-loop join over the in-memory maps is enough to prove correctness, including LEFT-join null-filling of the right side. | Table test over two small collections: assert INNER drops unmatched rows and LEFT keeps left rows with nil right fields. |
| Should-be-true | Source qualification can reuse the existing `RecordsetSource.Alias()` / `Name()` as the join key rather than inventing a new alias concept. | Prototype `Field(src, name)` resolving against `From().Base()` and `Joins()[i]` by alias/name; confirm no new aliasing machinery is required. |
| Might-be-true | Equality-only ON clauses cover the dominant real join need, so richer ON (ranges, AND/OR) can wait. | Review intended consumers (DTQL, datatug) for any non-equi-join requirement before widening scope. |


## SpecScore Integration

- **New Features this would create:** `dal` join AST (join-type enum, `NewJoinedSource`, qualified `Field`); `dalgo2memory` nested-loop join execution. Likely two Features.
- **Existing Features affected:** `dtql` — its subset gate currently rejects any query with joins; unaffected until a later cycle extends DTQL to serialize the new AST.
- **Dependencies:** Upstream — sidekick seed `dalgo-needs-first-class-join-support-in-its-query-model`. Downstream (unblocked by this) — SQL rendering in `dalgo2sql`/`dalgo2sqlite`, and DTQL join serialization.

## Open Questions

- Final spelling of the qualifier API: a `Field(src, name)` constructor vs. methods on `FieldRef` — settle at spec time.
- How a source is named in the qualifier: reuse `RecordsetSource.Alias()`/`Name()` as the key, or introduce explicit per-query aliases for self-joins?
- Shape of the join-type enum: exported typed constant (e.g. `dal.JoinInner`) vs. a string — and whether reserved-but-unsupported types (RIGHT/FULL/CROSS) reject at construction or at execution.
- Does the additive qualifier need to propagate into `OrderBy`/`Columns` rendering in `dalgo2memory` for the MVP, or only into ON resolution?

---
*This document follows the https://specscore.md/idea-specification*
