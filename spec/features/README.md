---
format: https://specscore.md/features-index-specification
---

# Features

> [View in SpecStudio](https://specstudio.synchestra.io/project/features?id=dalgo@dal-go@github.com&path=spec%2Ffeatures) — graph, discussions, approvals

This directory will hold feature specifications for [DALgo](https://github.com/dal-go/dalgo) as they are written.

The Feature format follows [SpecScore](https://specscore.md/feature-specification).

## Index

| Feature | Status | Summary |
|---|---|---|
| [Concurrency Capability](concurrency-capability/README.md) | Implemented | Add `dal.ConcurrencyAware` capability interface embedded in `dal.DB`, plus `NoConcurrency`/`ConcurrencyAvailable` embeddable helper structs, so consumers can size worker pools without engine-specific knowledge. |
| [Schema Description Vocabulary & Read Capability (`dbschema`)](dbschema/README.md) | Implemented | Umbrella for the schema-description vocabulary AND the read-side (introspection) capability (new top-level `dbschema` package). Tier-1 types (`FieldDef`, `CollectionDef`, `IndexDef`, `Type`, `Precision`, `DefaultExpr`), `SchemaReader` capability interface + helpers, and the shared `NotSupportedError` typed error. Designed for three-tier composition: Tier-2 engine extensions in driver repos; Tier-3 app wrappers in consumers (datatug-cli, etc.). |
| [Schema Modification (DDL) Execution Surface (`ddl`)](ddl/README.md) | Implemented | Umbrella for the schema-modification execution surface (new top-level `ddl` package). 3-method `SchemaModifier` capability interface (`CreateCollection`, `DropCollection`, `AlterCollection`); composable `AlterOp` model with six constructors (`AddField`, `DropField`, `ModifyField`, `RenameField`, `AddIndex`, `DropIndex`) — all accept `Option` for opt-in idempotency; `TransactionalDDL` capability for atomicity advertisement; `PartialSuccessError` for non-transactional partial failures. Imports `dbschema` for types AND for the shared `NotSupportedError`. |
| [`recordops` package](recordops/README.md) | Implemented | Umbrella for the `recordops` package — pure, dependency-free analytical / inspection helpers over dalgo record collections. MVP introduces one child: [diff](recordops/diff/README.md) — one baseline vs. N candidates via K-way merge over ID-sorted `iter.Seq2` streams, with four renderers (git-style YAML, by-ID YAML, plain YAML, JSON) and bridge helpers `SliceToSeq` + `ReaderToSeq`. |
| [Transaction Message](transaction-message/README.md) | Approved | — |
| [Computed Columns in recordset (neutral evaluator contract)](recordset-computed-columns/README.md) | Approved | — |
| [DTQL — YAML serialization of dal.StructuredQuery](dtql/README.md) | Approved | — |
| [First-class INNER/LEFT joins in dal's query model](query-joins/README.md) | Approved | — |
| [Source-qualified, multi-key ORDER BY resolution in dalgo2memory](qualified-orderby-resolution/README.md) | Approved | — |
| [Column selection in the query builder, projected by dalgo2memory](query-column-projection/README.md) | Stable | — |
| [GROUP BY with aggregation in the query builder, executed by dalgo2memory](query-group-by-aggregation/README.md) | Stable | — |
| [Pluggable per-collection storage engine seam (dalgo2memory)](storage-engine-seam/README.md) | Stable | — |
| [Serialized storage engine (dalgo2memory default)](serialized-storage/README.md) | Stable | — |
| [Columnar storage engine (dalgo2memory)](columnar-storage/README.md) | Stable | — |
| [Mixed-mode columnar storage for map[string]any collections (dalgo2memory)](columnar-mixed-mode-maps/README.md) | Stable | — |
| [Typed Collection[K, T] convenience layer (point CRUD)](typed-collection/README.md) | Approved | — |
| [Generated-ID Insert for Collection[T] (approach C)](collection-generated-insert/README.md) | Approved | — |
| [Collection[K, T] batch insert and Count/Exists/First terminals](typed-collection-extras/README.md) | Approved | — |
| [dalgo2namecheap: NameCheap API Adapter](dalgo2namecheap-namecheap-api-adapter/README.md) | Approved | — |
| [Hierarchical access and audit policies](access-policies/README.md) | Stable | — |
| [Triggers and webhooks: change events, transactional outbox, dispatcher](triggers/README.md) | Draft | — |

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/features-index-specification*
