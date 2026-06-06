---
format: https://specscore.md/feature-specification
status: Implemented
---

# Feature: `recordops` package

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/recordops?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/recordops?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/recordops?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/recordops?op=request-change) |

**Status:** Implemented
**Source Idea:** [`recordops`](../../ideas/recordops.md)

## Summary

Introduce a new top-level dalgo sub-package `github.com/dal-go/dalgo/recordops` — a home for pure, dependency-free analytical and inspection helpers that operate on collections of dalgo records. The package has no DB-driver dependencies; everything is in-memory, side-effect free, and operates over `record.WithID[K]` (and, in a deferred capacity, `map[K]V`).

This umbrella Feature establishes the package shell (the package directory, package doc, and design conventions); the actual capabilities are specified in child Features. MVP ships exactly one child: [`diff`](diff/README.md) — record-level Diff with three renderers.

## Problem

Today, dalgo consumers who want analytical helpers over recordsets (compare, group, intersect, sort) reinvent them locally — usually inside test packages, never reused. There is no shared home for these helpers, and no shared idiom for shape or output format. `recordops` becomes that home, starting with the most-asked capability: Diff.

## Children

Listed in implementation order (MVP first):

| Feature | Status | Summary |
|---|---|---|
| [diff/](diff/README.md) | Implemented | `Diff` (for `cmp.Ordered` keys) + `DiffFunc` (for any `comparable` key with an explicit `less`, e.g., `[16]byte` UUIDs) — one baseline vs. N candidates via K-way merge over ID-sorted `iter.Seq2` streams, returning `iter.Seq2[IDDiff[K], error]` with parallel-indexed `Candidates`; baseline-as-single-source-of-truth shape; four renderers (git-style YAML, by-ID YAML, plain YAML, JSON); bridge helpers `SliceToSeq` and `ReaderToSeq` for adapting in-memory slices and `dal.RecordsReader`. |

Future children, deferred until a real consumer asks (each becomes its own sibling Feature):

- `group-by-id` — bucket records by a derived key.
- `intersect` — set intersection over `record.WithID[K]` by ID.
- `sort-by-id` — stable sort by ID with type-aware comparator hooks.

These are **not** committed scope of this Feature — they're listed to make the package's purpose legible and to confirm the design-tenet test ("a small package, not a junk drawer").

## Package Design Tenets

These tenets bind every child Feature added under `recordops/`:

1. **No DB-driver dependencies.** The package MUST NOT import any package under `dalgo2*`, `ddl`, `dbschema/schema-reader`, or `dal` runtime-only types. Importing `record`, `dal.RecordsReader` (interface only, for the bridge), and `dbschema` Tier-1 types is permitted.
2. **Pure helpers; I/O lives in inputs.** Helpers MUST NOT start goroutines, plumb `context.Context`, or perform direct file/network I/O. Streaming I/O is delegated to the caller via `iter.Seq2` inputs — recordops just pulls. (If a future helper needs context-aware cancellation, it lands as a `context.Context`-accepting variant alongside the pure one — never replacing it.)
3. **Generics-friendly.** Entry points are generic over `K comparable` (record ID type). Where ordering is required (Diff family), a `cmp.Ordered` convenience entry pairs with a `*Func`-suffixed variant that takes an explicit `less` — mirroring `slices.Sort` / `slices.SortFunc`.
4. **Determinism.** Every helper that returns a collection (or a stream backed by deterministic inputs) MUST return it in a deterministic order across runs.
5. **Errors are structured.** Sentinel errors live in a single `recordops/errors.go`; helpers wrap them with `fmt.Errorf("recordops/<helper>: ...: %w", ...)`.

## Out of Scope (this umbrella Feature)

- **Specific helper implementations** — each lives in a child Feature.
- **Re-exporting helpers at the dalgo root.** Consumers import `github.com/dal-go/dalgo/recordops` explicitly.
- **Columnar `recordset.Recordset` support.** Distinct shape; if/when a consumer needs it, a sibling package (e.g., `recordsetops`) is the right answer — not retrofitting `recordops`.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
