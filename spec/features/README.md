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
| [concurrency-capability](concurrency-capability/README.md) | Implemented | Add `dal.ConcurrencyAware` capability interface embedded in `dal.DB`, plus `NoConcurrency`/`ConcurrencyAvailable` embeddable helper structs, so consumers can size worker pools without engine-specific knowledge. |
| [dbschema](dbschema/README.md) | Implemented | Umbrella for the schema-description vocabulary AND the read-side (introspection) capability (new top-level `dbschema` package). Tier-1 types (`FieldDef`, `CollectionDef`, `IndexDef`, `Type`, `Precision`, `DefaultExpr`), `SchemaReader` capability interface + helpers, and the shared `NotSupportedError` typed error. Designed for three-tier composition: Tier-2 engine extensions in driver repos; Tier-3 app wrappers in consumers (datatug-cli, etc.). |
| [ddl](ddl/README.md) | Implemented | Umbrella for the schema-modification execution surface (new top-level `ddl` package). 3-method `SchemaModifier` capability interface (`CreateCollection`, `DropCollection`, `AlterCollection`); composable `AlterOp` model with six constructors (`AddField`, `DropField`, `ModifyField`, `RenameField`, `AddIndex`, `DropIndex`) — all accept `Option` for opt-in idempotency; `TransactionalDDL` capability for atomicity advertisement; `PartialSuccessError` for non-transactional partial failures. Imports `dbschema` for types AND for the shared `NotSupportedError`. |
| [recordops](recordops/README.md) | Implemented | Umbrella for the `recordops` package — pure, dependency-free analytical / inspection helpers over dalgo record collections. MVP introduces one child: [diff](recordops/diff/README.md) — one baseline vs. N candidates via K-way merge over ID-sorted `iter.Seq2` streams, with four renderers (git-style YAML, by-ID YAML, plain YAML, JSON) and bridge helpers `SliceToSeq` + `ReaderToSeq`. |
| [transaction-message](transaction-message/README.md) | Approved | — |
| [recordset-computed-columns](recordset-computed-columns/README.md) | Approved | — |
| [dtql](dtql/README.md) | Approved | — |
| [query-joins](query-joins/README.md) | Approved | — |
| [qualified-orderby-resolution](qualified-orderby-resolution/README.md) | Approved | — |
| [query-column-projection](query-column-projection/README.md) | Stable | — |
| [query-group-by-aggregation](query-group-by-aggregation/README.md) | Stable | — |
| [storage-engine-seam](storage-engine-seam/README.md) | Stable | — |
| [serialized-storage](serialized-storage/README.md) | Stable | — |
| [columnar-storage](columnar-storage/README.md) | Stable | — |
| [columnar-mixed-mode-maps](columnar-mixed-mode-maps/README.md) | Stable | — |
| [typed-collection](typed-collection/README.md) | Approved | — |
| [collection-generated-insert](collection-generated-insert/README.md) | Approved | — |
| [typed-collection-extras](typed-collection-extras/README.md) | Approved | — |
| [dalgo2namecheap-namecheap-api-adapter](dalgo2namecheap-namecheap-api-adapter/README.md) | Approved | — |
| [access-policies](access-policies/README.md) | Stable | — |

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/features-index-specification*
