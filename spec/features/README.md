# Features

> [View in SpecStudio](https://specstudio.synchestra.io/project/features?id=dalgo@dal-go@github.com&path=spec%2Ffeatures) — graph, discussions, approvals

This directory will hold feature specifications for [DALgo](https://github.com/dal-go/dalgo) as they are written.

The Feature format follows [SpecScore](https://specscore.md/feature-specification).

## Index

| Feature | Status | Summary |
|---|---|---|
| [concurrency-capability](concurrency-capability/README.md) | Implemented | Add `dal.ConcurrencyAware` capability interface embedded in `dal.DB`, plus `NoConcurrency`/`ConcurrencyAvailable` embeddable helper structs, so consumers can size worker pools without engine-specific knowledge. |
| [dbschema](dbschema/README.md) | Approved | Umbrella for the schema-description vocabulary AND the read-side (introspection) capability (new top-level `dbschema` package). Tier-1 types (`FieldDef`, `CollectionDef`, `IndexDef`, `Type`, `Precision`, `DefaultExpr`), `SchemaReader` capability interface + helpers, and the shared `NotSupportedError` typed error. Designed for three-tier composition: Tier-2 engine extensions in driver repos; Tier-3 app wrappers in consumers (datatug-cli, etc.). |
| [ddl](ddl/README.md) | Approved | Umbrella for the schema-modification execution surface (new top-level `ddl` package). 3-method `SchemaModifier` capability interface (`CreateCollection`, `DropCollection`, `AlterCollection`); composable `AlterOp` model with six constructors (`AddField`, `DropField`, `ModifyField`, `RenameField`, `AddIndex`, `DropIndex`) — all accept `Option` for opt-in idempotency; `TransactionalDDL` capability for atomicity advertisement; `PartialSuccessError` for non-transactional partial failures. Imports `dbschema` for types AND for the shared `NotSupportedError`. |

## Outstanding Questions

None at this time.

---
*This document follows the https://specscore.md/features-index-specification*
