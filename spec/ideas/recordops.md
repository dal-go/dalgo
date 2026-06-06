---
format: https://specscore.md/idea-specification
status: Approved
---

# Idea: recordops — Recordset Operations Package

**Status:** Approved
**Date:** 2026-05-15
**Owner:** alex
**Promotes To:** —
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we give dalgo users a small, focused, streaming toolkit for inspecting and comparing recordsets — starting with a record-level Diff that handles one baseline vs. N candidates in a single pass, reads pull-based streams instead of pre-materialized slices, and reads like a git diff?

## Context

Triggered by an ad-hoc need to compare a baseline recordset against one or more candidate recordsets in tests, snapshots, multi-backend migrations, and divergence reports across databases. Sits alongside existing packages (`record`, `recordset`, `update`). Related approved ideas: `dalgo-schema-modification`, `concurrency-capability` — neither covers analytical / inspection tooling.

A sibling Idea [[dal-records-reader-iter-seq]] proposes aligning dalgo's existing reader idiom with `iter.Seq`. recordops works today via a small bridge regardless of when that lands.

## Recommended Direction

Introduce a new top-level package `recordops` containing pure, dependency-free helpers that operate on streams of `record.WithID`. The first MVP function is `Diff(baseline, candidates, opts...)` which compares one baseline stream against N candidate streams in a single pass and emits diff entries via `iter.Seq2[IDDiff[K], error]` — pull-based, low-memory, error-propagating.

Algorithmically a **K-way merge** over ID-sorted streams: one cursor per input (baseline + N candidates), repeatedly advance the head with the smallest ID, classify per candidate, emit one `IDDiff` per interesting ID, repeat. Memory footprint at any moment: O(N) records (one per stream) plus the in-flight `IDDiff` being emitted. The caller's input streams MUST already be sorted by ID — there is no internal sort step; that's the price of streaming.

Two parallel entrypoints — analogous to `slices.Sort` / `slices.SortFunc`:
- `Diff[K cmp.Ordered](baseline, candidates, opts...)` for the dominant simple cases (`string`/`int`/`int64` IDs).
- `DiffFunc[K comparable](baseline, candidates, less, opts...)` for `[16]byte` UUIDs (using `bytes.Compare(a[:], b[:]) < 0` — native 16-byte comparison) and any other key type that is `comparable` but not `cmp.Ordered`.

Both share one internal merge driver. `Diff` synthesizes `less` from `<` and delegates.

**Output shape — one entry per ID across all candidates, with baseline-as-single-source-of-truth.** Each emitted `IDDiff[K]` carries: (a) the ID; (b) an optional `Baseline *RecordSnapshot` (nil iff baseline lacks this ID, else holding the baseline record's field values); (c) `Candidates []CandidateState` of length `len(candidates)` (parallel-indexed — `Candidates[i]` always corresponds to input `candidates[i]`). Each `CandidateState` has a `Status` (`Missing` / `Extra` / `Matched` / `Changed`) and a `Fields []FieldValue` carrying only deltas — never duplicating baseline values. The same `FieldValue{Name, Value, Absent}` struct is used in both `Baseline.Fields` (where `Value` is baseline's value and `Absent` is always false) and `Candidates[i].Fields` (where `Value` is the candidate's delta value, and `Absent == true` signals "field absent from candidate's record" — structurally distinct from a real Go-nil `Value`). Consumers read the old value from `IDDiff.Baseline.Fields`, the candidate's value from `Candidates[i].Fields`. By default, an `IDDiff` is emitted only when at least one candidate is non-matched; with `WithIncludeMatched()` it's emitted for every ID touched by any input. By default `Baseline.Fields` includes ALL baseline fields (full record context); `WithOnlyChangedFields()` trims baseline to only the fields that have a delta on some candidate. Both `Baseline.Fields` and `Candidates[i].Fields` are slices, not maps, to preserve declared field order and to future-proof for record types with unnamed/positional columns (where the slice can hold entries with empty `Name`).

Three renderers (the structured stream exists once; serializing is trivial):
- `RenderYAMLGitStyle(diffs, candidateIndex, collectionName)` — the must-have anchor, per-candidate git-diff-style YAML.
- `RenderYAMLByID(diffs, collectionName)` — cross-candidate divergence view, one block per ID showing baseline + each candidate side-by-side.
- `RenderYAML(diffs, collectionName)` / `RenderJSON(diffs, collectionName)` — full structured serialization of the stream.

Two bridge helpers ship with the package: `SliceToSeq` (slice → `iter.Seq2`) for tests and ad-hoc usage, and `ReaderToSeq` (`dal.RecordsReader` → `iter.Seq2`) so callers can stream from dalgo queries today, without waiting for `dal.RecordsReader` itself to gain an `iter.Seq2` method (tracked in [[dal-records-reader-iter-seq]]).

## Alternatives Considered

- **In-memory slices for inputs (`[]record.WithID[K]`).** The earlier draft of this Idea. Lost on memory: forces full materialization of every candidate recordset before the diff starts, which the user explicitly rejected — large-scale comparisons (multi-backend migration audits, snapshot diffs over millions of rows) need streaming.
- **Per-candidate output streams (`[]iter.Seq[DiffEntry]`).** Returning N parallel streams looks natural but cannot be produced from a single pass over the baseline without either buffering everything (defeats streaming) or forcing the consumer to advance all N streams in lockstep (hidden gotcha — easy to deadlock). A single combined stream keyed by ID is single-pass-correct and trivial to demultiplex by candidate when needed.
- **Channel-based reader interfaces.** Older Go idiom — harder error propagation, requires goroutines for the producer, harder to cancel. `iter.Seq2[T, error]` is the language-blessed answer in Go 1.23+.
- **Custom `recordops.Reader` interface.** Would create a third reader idiom in dalgo (alongside `dal.RecordsReader` and the streaming-but-not-yet-existing `iter.Seq`-based reader). Better to converge on `iter.Seq2` and migrate dalgo proper later (sibling Idea).
- **Extend `record` or `recordset` packages with a Diff method on each type.** Lost because diffing is a cross-cutting concern, not core to either type's responsibility, and would force every future analytical helper (group-by, joins, schema-check) to wedge itself into already-stable packages.
- **External helper module (separate go module).** Lost because the API needs to evolve in lockstep with `record.WithID` and `dal` types — a separate module would constantly chase dalgo's minor versions, and users would have to import two modules to get one workflow.

## MVP Scope

A two-to-three-week build that lands `recordops/` with:

1. **Two entrypoints** sharing one internal K-way merge driver:
   - `Diff[K cmp.Ordered](baseline iter.Seq2[record.WithID[K], error], candidates []iter.Seq2[record.WithID[K], error], opts ...Option) iter.Seq2[IDDiff[K], error]`
   - `DiffFunc[K comparable](baseline, candidates, less func(a, b K) bool, opts ...Option) iter.Seq2[IDDiff[K], error]`
2. **Output types**: `IDDiff[K]`, `RecordSnapshot`, `CandidateState`, `FieldValue`, `RecordStatus` enum.
3. **Four options**: `WithIgnoreFields(fields ...string)`, `WithIncludeMatched()`, `WithOnlyChangedFields()`, `WithAbsentEqualsNil()`.
4. **Four renderers**: `RenderYAMLGitStyle`, `RenderYAMLByID` (the new cross-candidate divergence view), `RenderYAML`, `RenderJSON`.
5. **Bridge helpers**: `SliceToSeq[K]` and `ReaderToSeq[K]`.
6. **Tests**: added/removed/changed/matched per-candidate, multi-candidate (N=3) interleaving, UUID via `DiffFunc`, `Diff`-delegates-to-`DiffFunc` equivalence, `WithIncludeMatched` and `WithOnlyChangedFields` and `WithIgnoreFields` ACs, unsorted-input detection, stream-error propagation, golden test pinning the git-style YAML against the example from the original request.
7. **One README example** demonstrating end-to-end usage with both the per-candidate and cross-candidate renderers.

No CLI, no schema-aware diffing, no `DiffMap` entrypoint, no patch application.

## Not Doing (and Why)

- Diffing over the Recordset (columnar) interface — MVP targets record.WithID streams only; columnar support deferred until a real use case lands.
- Internal sorting of unsorted input — explicit caller responsibility (use `ORDER BY id` in queries; pre-sort snapshots). Sorting a stream defeats streaming.
- Schema-aware diffing (column adds/drops) — belongs with dalgo-schema-modification, not here.
- CLI tool — library only; users wire their own command if needed.
- Patch application (apply a diff back to a recordset) — analysis only, not mutation.
- `DiffMap[K, V](baseline, candidates ...map[K]V)` entrypoint — keeping the API surface small in MVP; can land later as a real implementation without breakage.
- A hash-map-based variant that drops the ordering requirement entirely (any `K comparable`, no `less` needed) — out of scope; `DiffFunc` covers non-`cmp.Ordered` keys via an explicit comparator, and the renderers want ID-sorted output anyway.
- Pluggable equality (custom `Equal` per field) and JSON-tag-aware field names — `reflect.DeepEqual` only in MVP. (`WithIgnoreFields(...)` IS in MVP.)
- Migrating `dal.RecordsReader` itself to `iter.Seq2` — tracked in [[dal-records-reader-iter-seq]]; this Idea only ships the one-off bridge.

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | Callers can deliver baseline + candidates as ID-sorted streams (SQL `ORDER BY id`; pre-sorted snapshots; `SliceToSeq` after a `slices.SortFunc`). | Spot-check the dominant dalgo use cases. SQL trivially supports `ORDER BY`; NoSQL (Firestore, Datastore) supports ordering keys natively. The remaining case is in-memory snapshots where sorting is cheap. |
| Must-be-true | Record IDs in practice fall into two buckets: `cmp.Ordered` (`int`, `int64`, `string`) — handled by `Diff` — and non-ordered-but-`comparable` (`[16]byte` UUIDs, struct keys) — handled by `DiffFunc` with an explicit `less`. Both buckets get the same algorithmic win. | Spot-check the ID types used in `record.WithID[K]` across dalgo consumers; the most common non-`cmp.Ordered` case is `[16]byte` UUIDs, comparable via `bytes.Compare(a[:], b[:]) < 0`. |
| Must-be-true | Per-field equality must reflect over the value returned by `record.Record.Data()` (the concrete struct/map) — NOT over the `dal.Record` interface header, which carries `hasChanged`/error state that would produce false diffs. | Add an AC that pins this behavior using a real `dal.Record` implementation that mutates non-data fields between calls. |
| Must-be-true | The git-style YAML format is what users actually want for the per-candidate view. | Pin via a golden test against the example from the original request. |
| Should-be-true | The cross-candidate divergence renderer (`RenderYAMLByID`) is the more natural fit for multi-backend audits and will become the default choice for those use cases. | Use it in one end-to-end example in the README; confirm it tells the story without demultiplexing. |
| Should-be-true | Single-pass K-way merge meaningfully outperforms N independent binary streams over the same baseline — saves (K−1) baseline reads. | Micro-benchmark with a 10k-record baseline and K=10 candidates from disk-backed sources; confirm the wall-clock improvement. (Caveat: at K=1 the two approaches are equivalent; the win scales with K.) |
| Should-be-true | The `recordops` package will host Diff plus 2–4 future helpers (e.g., `GroupByID`, `Intersect`, `SortByID`) without becoming a junk drawer. | Sketch the next 3 likely helpers and confirm they share the same shape and dependencies as `Diff`. |
| Might-be-true | The `WithOnlyChangedFields()` option will be the more common consumer choice for terse diffs, even though the default is "include all fields." | Wait and see; flip the default if real usage shows >50% of consumers always pass the option. |
| Might-be-true | Stream-based error propagation via `iter.Seq2[T, error]` (errors mixed into the yield, not separate return) is ergonomic enough for callers. | Test against real consumer code; if too many consumers wrap and unwrap, revisit. |

## SpecScore Integration

- **New Features this would create:** `recordops` umbrella + `recordops/diff` child Feature.
- **Existing Features affected:** none.
- **Dependencies:** [[dal-records-reader-iter-seq]] (informational only — recordops works without it via the bridge).

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/idea-specification*
