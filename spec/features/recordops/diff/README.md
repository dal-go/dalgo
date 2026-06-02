# Feature: `recordops` — Diff

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/recordops/diff?op=explore) | [Edit](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/recordops/diff?op=edit) | [Ask question](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/recordops/diff?op=ask) | [Request change](https://specscore.studio/app/github.com/dal-go/dalgo/spec/features/recordops/diff?op=request-change) |

**Status:** Implemented
**Source Idea:** [`recordops`](../../../ideas/recordops.md)
**Parent Feature:** [`recordops`](../README.md)

## Summary

The first capability of the `recordops` package: `Diff`, which compares one **baseline stream** against N **candidate streams** in a single pass via K-way merge over ID-sorted inputs, and emits one `IDDiff` per ID where at least one candidate diverges. Inputs and output are `iter.Seq2[..., error]` (Go 1.23+ pull-based iteration with error propagation). Memory footprint: O(N) records in flight.

Three orthogonal options control output content: `WithIgnoreFields` (skip named fields in comparison), `WithIncludeMatched` (also emit IDs where every candidate matches baseline), `WithOnlyChangedFields` (trim baseline snapshot to only fields that have a candidate delta).

Four renderers consume the diff stream: `RenderYAMLGitStyle` (per-candidate git-diff-style — the must-have anchor matching the source Idea's example), `RenderYAMLByID` (cross-candidate divergence view — one block per ID showing baseline and all candidates side-by-side), `RenderYAML`, `RenderJSON`. Two bridge helpers — `SliceToSeq` and `ReaderToSeq` — make the streaming entrypoints usable from in-memory slices and from existing `dal.RecordsReader` instances.

## Problem

Dalgo consumers need to compare recordsets — produced by tests, snapshots, migrations, and multi-backend audits — but today they hand-roll comparisons every time. Output is unprincipled (often a `cmp.Diff` dump), illegible to non-Go reviewers, and inefficient: each ad-hoc tool either fully materializes everything in memory or invokes a separate one-baseline-vs-one-candidate routine N times.

For the multi-backend audit case (e.g., comparing a Firestore snapshot against three SQL backends), the natural mental model is "for each user ID, show me where the backends disagree." There's no shared shape for this in dalgo today, and no API that streams the comparison so it works on million-row recordsets without loading everything into memory first.

`recordops.Diff` solves both: a single K-way merge over sorted streams yielding one `IDDiff` per divergent ID; a structured tree expressing baseline truth once and per-candidate deltas; four renderers covering both the "per-backend git diff" and "per-ID cross-backend audit" use cases.

## Behavior

### REQ: input-streams

`Diff` and `DiffFunc` MUST accept inputs as `iter.Seq2[record.WithID[K], error]`. The package MUST define an alias for readability:

```go
type RecordSeq[K comparable] = iter.Seq2[record.WithID[K], error]
```

Streams MUST yield records in **ascending ID order**. The package MUST validate monotonicity as it iterates each stream: on observing a `Next()` whose ID is less than or equal to the previously yielded ID from the same stream, it MUST emit a single terminal `(zero, ErrUnsortedInput)` pair on the output stream and stop. Equal IDs are considered duplicates and rejected (`ErrDuplicateID`).

A `(zero, err)` pair from any input stream MUST be propagated to the output stream as `(zero, err)` and iteration MUST stop.

#### AC-1: monotonic-acceptance

**Given** a baseline stream yielding three records with IDs `"a"`, `"b"`, `"c"` (each paired with `nil` error) and a candidate stream of equivalent shape
**When** `Diff(baseline, []RecordSeq{candidate})` is iterated to completion
**Then** the consumer observes a finite, non-error iteration (zero or more `IDDiff` items, all with `err == nil`).

#### AC-2: non-monotonic-rejected

**Given** a baseline stream that yields IDs `"a"`, `"c"`, `"b"` (out of order)
**When** `Diff(baseline, ...)` is iterated
**Then** the iteration yields at most one in-flight `IDDiff` (for an ID earlier than `"b"`) before yielding `(zero, err)` where `errors.Is(err, recordops.ErrUnsortedInput)`, and then iteration stops.

#### AC-3: duplicate-id-rejected

**Given** a baseline stream that yields IDs `"a"`, `"a"`, `"b"`
**When** `Diff(baseline, ...)` is iterated
**Then** the iteration yields `(zero, err)` where `errors.Is(err, recordops.ErrDuplicateID)` and stops.

#### AC-4: upstream-stream-error-propagated

**Given** a candidate stream that yields one valid record `(r, nil)`, then yields `(zero, io.ErrUnexpectedEOF)`
**When** `Diff(baseline, []RecordSeq{candidate})` is iterated
**Then** the iteration eventually yields `(zero, err)` where `errors.Is(err, io.ErrUnexpectedEOF)` and stops.

### REQ: id-diff-shape

The package MUST export:

```go
type IDDiff[K comparable] struct {
    ID         K
    Baseline   *RecordSnapshot  // nil iff baseline lacks this ID
    Candidates []CandidateState // ALWAYS parallel to input candidates; len == len(candidates)
}

type RecordSnapshot struct {
    Fields []FieldValue  // baseline's field values; one source of truth
}

type CandidateState struct {
    Status RecordStatus
    Fields []FieldValue  // only deltas from baseline; semantics per Status (see below)
}

// FieldValue is used in BOTH RecordSnapshot.Fields and CandidateState.Fields.
// In RecordSnapshot.Fields, Value is the baseline's value for the named field; Absent is always false.
// In CandidateState.Fields, Value is the candidate's value for the named field
// (when the candidate has a delta vs. baseline). When the field exists in
// baseline but is absent from the candidate's record (only possible inside a
// Changed candidate), Absent is true and Value is the zero value — consumers
// MUST NOT interpret Value when Absent is true.
// The "old" value for a Changed candidate is found by looking up the same
// Name in IDDiff.Baseline.Fields.
type FieldValue struct {
    Name   string  // field name. May be empty when the data source has unnamed columns (e.g., raw tabular SELECT) — position in the slice is then significant.
    Value  any
    Absent bool    // true iff this field is absent from the candidate record (Changed candidates only). Always false in RecordSnapshot.Fields and in Extra candidates' Fields.
}

type RecordStatus int8

const (
    Missing RecordStatus = iota  // this candidate lacks the surfaced ID (regardless of baseline). The ID can be surfaced because another candidate has it.
    Extra                         // this candidate has the ID; baseline doesn't
    Matched                       // both this candidate AND baseline have the ID; all fields equal
    Changed                       // both this candidate AND baseline have the ID; at least one field differs
)
```

**`Candidates` length invariant.** When an `IDDiff` is emitted, `len(Candidates) == len(candidates)` (the input slice length). `Candidates[i]` always describes the input `candidates[i]`'s state for this ID — never omitted, never reordered. Consumers can use slice indexing without lookup or filtering.

**`RecordSnapshot.Fields` and `CandidateState.Fields` are slices, not maps.** Three reasons: (1) preserves declared field order from the record (important for renderers that want stable output and for record types where field order is semantic); (2) future-proofs the data model for record types with unnamed/positional columns (e.g., raw SQL `SELECT 1, 2, 3` without aliases) — a slice tolerates positional duplicates and empty names, a map cannot; (3) typical records have <50 fields, so the O(n) name lookup cost (when consumers cross-reference `Baseline.Fields` from a candidate delta) is negligible. **MVP does NOT produce `Name == ""` entries** — every comparison path emits a non-empty name (struct field name, map key, or the virtual `_value` for non-struct/non-map data). The empty-name shape is reserved for a future helper that ingests positional/unnamed-column records; renderers MAY assume non-empty names in MVP.

**`Baseline.Fields` semantics:**
- Default mode: includes every field of the baseline record (full snapshot).
- `WithOnlyChangedFields()` mode: includes only fields that have at least one delta across the `Candidates` slice. Fields that every candidate agrees on with baseline are omitted.
- If baseline lacks this ID (e.g., an ID emitted because some candidate has Extra), `Baseline` is `nil`.

**`CandidateState.Fields` semantics per `Status`:**

| Status | Fields contents |
|---|---|
| `Missing` | `nil` (no record exists on this side). |
| `Extra` | Full list of the candidate's fields, each carrying `Value`. (Baseline has nothing to compare against.) |
| `Matched` | `nil`. The candidate agrees with baseline on every field — consumer reads `IDDiff.Baseline.Fields` for the values. |
| `Changed` | Only the fields whose value differs from baseline. Each entry carries the candidate's `Value`. The old value is found by looking up the same `Name` in `IDDiff.Baseline.Fields`. |

**Field-absence encoding** (Changed status only). When a field exists in baseline but is absent from the candidate (the candidate's record has fewer fields), the candidate's `Fields` entry for that name MUST be present with `Absent == true` and `Value` left as the zero value. This is a structural distinction — `Absent == true` is NOT the same as `Value == nil` (which represents a real Go-nil value that the candidate explicitly holds). Consumers and renderers MUST branch on `Absent` before interpreting `Value`.

**`Baseline.Fields == nil` edge case.** When `WithIncludeMatched()` is set and no candidate has any delta for an ID (i.e., the IDDiff is emitted solely because of completeness mode), `Baseline.Fields` MUST be `nil` regardless of whether `WithOnlyChangedFields()` was passed — there is nothing to show. This matches the default-include behavior of `WithOnlyChangedFields()` ("only fields with at least one candidate delta") at its degenerate case.

#### AC-1: candidates-slice-parallel-indexed

**Given** `len(candidates) == 3` and any emitted `IDDiff`
**When** the consumer reads `idDiff.Candidates`
**Then** `len(idDiff.Candidates) == 3`, and `idDiff.Candidates[i]` describes the state for input `candidates[i]` for every `i in [0, 3)`.

#### AC-2: baseline-nil-when-id-absent-in-baseline

**Given** a baseline lacking `ID == "x"` and a candidate containing `ID == "x"` with one field
**When** `Diff` emits the `IDDiff` for `"x"`
**Then** `idDiff.Baseline == nil`, `idDiff.Candidates[0].Status == Extra`, and `idDiff.Candidates[0].Fields` contains the candidate's field with `Value` set.

#### AC-3: matched-fields-nil

**Given** an `IDDiff` for `ID == "u1"` where some candidate is Changed (so the `IDDiff` is emitted), and another candidate is Matched for `u1`
**When** the consumer reads `idDiff.Candidates[matchedIdx].Fields`
**Then** the value is `nil`.

#### AC-4: changed-fields-carry-only-deltas

**Given** baseline `u3 = {first_name:"Alex", last_name:"Smith", age:30}` and candidate `u3 = {first_name:"Alexander", last_name:"Smith", age:30}` (only `first_name` differs)
**When** `Diff` emits the `IDDiff` for `u3`
**Then** the relevant `CandidateState.Fields` contains exactly one entry `{Name:"first_name", Value:"Alexander"}` — `last_name` and `age` are NOT duplicated.

#### AC-5: field-absent-in-candidate-encoded-with-flag

**Given** baseline `u1.Data() == map[string]any{"first_name":"Alex", "nickname":"Al"}` and candidate `u1.Data() == map[string]any{"first_name":"Alex"}` (candidate lacks `nickname`)
**When** `Diff` emits the `IDDiff` for `u1`
**Then** `Candidates[0].Status == Changed` and `Candidates[0].Fields` contains exactly one entry `{Name:"nickname", Absent:true}` — the absent field is signaled by the flag (and `Baseline.Fields` carries `{Name:"nickname", Value:"Al"}` for the consumer to look up the old value).

#### AC-5b: nil-valued-field-distinct-from-absent

**Given** baseline `u1.Data() == map[string]any{"first_name":"Alex"}` and candidate `u1.Data() == map[string]any{"first_name":nil}` (candidate has the field but with a Go-nil value)
**When** `Diff` emits the `IDDiff` for `u1`
**Then** `Candidates[0].Fields` contains exactly one entry `{Name:"first_name", Value:nil, Absent:false}` — `Absent` is `false` because the field exists; consumers can tell this case apart from genuine absence.

#### AC-6: unnamed-columns-preserved-by-position

**Given** a baseline record whose `Data()` returns `[]any{1, "x", true}` (a slice — no field names) and a candidate record with the same ID whose `Data()` returns `[]any{1, "y", true}`
**When** `Diff` emits the `IDDiff` for that ID
**Then** the field comparison hits the `other` kind-bucket per REQ `field-comparison` step 6, and the `IDDiff.Candidates[0].Fields` contains exactly one entry `{Name:"_value", Value: <the candidate's slice>}` — the `_value` virtual field is the positional fallback when no field names exist. (Per-index column diffing is intentionally NOT performed in MVP.)

### REQ: emission-rules

`Diff` MUST emit an `IDDiff` for every ID seen across baseline and all candidate streams, subject to filtering:

- **Default mode:** emit iff at least one `Candidates[i].Status` is not `Matched`. IDs where every candidate is `Matched` are NOT emitted.
- **`WithIncludeMatched()` mode:** emit for every ID touched by any input — including fully-matched IDs. (Useful for completeness audits and for proving "everything matches.")

For every emitted `IDDiff`, `Candidates` MUST be fully populated (length == `len(candidates)`, every position carrying the correct `Status` and `Fields` for the input candidate at that index).

Output ordering: `IDDiff` entries MUST be emitted in **ascending ID order** (the K-way merge yields IDs in sorted order naturally).

#### AC-1: skip-fully-matched-by-default

**Given** baseline `[u1, u2]` and a single candidate stream containing `[u1, u2]` with identical field values
**When** `Diff(baseline, candidates)` is iterated
**Then** zero `IDDiff` items are yielded (only the iteration-end signal).

#### AC-2: include-matched-emits-all

**Given** the same identical streams
**When** `Diff(baseline, candidates, WithIncludeMatched())` is iterated
**Then** exactly two `IDDiff` items are yielded — one for `u1`, one for `u2`; each has `Candidates[0].Status == Matched` and `Candidates[0].Fields == nil`.

#### AC-3: id-ordering

**Given** baseline `[u1, u4]` and candidates `[ [u2, u3], [u3, u5] ]` with some divergence at every ID
**When** `Diff` is iterated
**Then** the IDs of the yielded `IDDiff` values appear in ascending order: `u1`, `u2`, `u3`, `u4`, `u5`.

### REQ: diff-entrypoints

The package MUST export two entrypoints — analogous to `slices.Sort` / `slices.SortFunc`:

```go
func Diff[K cmp.Ordered](
    baseline RecordSeq[K],
    candidates []RecordSeq[K],
    opts ...Option,
) iter.Seq2[IDDiff[K], error]

func DiffFunc[K comparable](
    baseline RecordSeq[K],
    candidates []RecordSeq[K],
    less func(a, b K) bool,
    opts ...Option,
) iter.Seq2[IDDiff[K], error]
```

`Diff` is the convenience path for `cmp.Ordered` keys (`string`, `int`, `int64`, etc.) — it internally calls `DiffFunc` with `less := func(a, b K) bool { return a < b }`. `DiffFunc` is the explicit-comparator path for keys that are `comparable` but not `cmp.Ordered` — most commonly UUIDs typed as `[16]byte` (`github.com/google/uuid`, `github.com/gofrs/uuid`), where the natural comparator is `bytes.Compare(a[:], b[:]) < 0` — a native 16-byte comparison.

Both entrypoints share one internal K-way merge driver; the only difference is the source of `less`.

`less` MUST be a strict weak order. The package does NOT validate this property; callers passing a non-strict-weak-order get undefined output (same contract as `slices.SortFunc`).

If `DiffFunc` is called with `less == nil`, the returned `iter.Seq2` MUST yield a single `(zero, err)` pair where `errors.Is(err, recordops.ErrInvalidArgument)` and stop.

#### AC-1: diff-with-string-keys

**Given** `K = string`, baseline `[{ID:"u1"}]` (singleton via `SliceToSeq`) and candidates `[ SliceToSeq([{ID:"u2"}]) ]`
**When** `Diff(baseline, candidates)` is iterated
**Then** iteration yields exactly two `IDDiff` values in order: one for `u1` (`Candidates[0].Status == Missing`) and one for `u2` (`Candidates[0].Status == Extra`).

#### AC-2: diff-func-with-uuid-keys

**Given** `K = [16]byte`, baseline and a single candidate of three random UUIDs each with one shared and one unique-per-side
**When** `DiffFunc(baseline, candidates, func(a, b [16]byte) bool { return bytes.Compare(a[:], b[:]) < 0 })` is iterated
**Then** the call compiles and yields exactly two `IDDiff` values (one Missing, one Extra), in ascending byte order.

#### AC-3: diff-delegates-to-diff-func

**Given** any string-keyed baseline and candidates fed through `SliceToSeq`
**When** consuming `Diff(baseline, candidates)` and `DiffFunc(baseline, candidates, func(a, b string) bool { return a < b })` independently
**Then** the collected slices of `IDDiff` values are deeply equal.

#### AC-4: diff-func-nil-less

**Given** any valid baseline and candidates streams
**When** `DiffFunc(baseline, candidates, nil)` is iterated
**Then** the iteration yields exactly one `(zero, err)` pair where `errors.Is(err, recordops.ErrInvalidArgument)` and stops.

#### AC-5: zero-candidates

**Given** any baseline stream and `candidates == nil` (or empty slice)
**When** `Diff(baseline, candidates)` is iterated
**Then** zero `IDDiff` items are yielded (the iteration completes cleanly).

### REQ: field-comparison

For IDs present in baseline AND in a candidate, the package MUST extract field-level data via `record.Record.Data()` and compare values — NOT the `dal.Record` interface itself. Resolution:

1. Call `wid.Record.Data()` to obtain the underlying value.
2. If the returned value is a pointer, dereference it.
3. **Kind-bucket classification.** Classify each side into one of three buckets: `struct`, `map-with-string-keys`, or `other` (scalar / slice / array / func / chan / nil / map-with-non-string-keys). If the two sides fall into different buckets, do NOT attempt per-field comparison; treat as a single virtual-field difference under `Name == "_value"` and skip steps 4–5.
4. If both sides are in the `struct` bucket: iterate exported fields. A field that fails `reflect.DeepEqual` produces a delta entry in the candidate's `Fields`.
5. If both sides are in the `map-with-string-keys` bucket: iterate the union of keys. A key whose value differs (or is present on only one side) produces a delta entry.
6. If both sides are in the `other` bucket: emit a single virtual `Name == "_value"` delta if `reflect.DeepEqual` fails on the whole.

Baseline's `RecordSnapshot.Fields` is populated separately from the same `Data()` extraction — full field list under default mode; only-fields-with-deltas under `WithOnlyChangedFields()`.

Field names emitted MUST match the Go struct field name (no JSON-tag rewriting in MVP — documented in package godoc as a known limitation).

If `reflect.DeepEqual` panics (e.g., a `func` or `chan` field), the panic MUST be recovered and surfaced through the output stream as a terminal `(zero, err)` pair wrapping `ErrIncomparableField` with the offending field name and record ID in the message.

#### AC-1: data-extracted-via-record-data

**Given** a `dal.Record` implementation whose `Data()` returns `&User{FirstName:"Alex"}` and whose other interface methods (e.g., `HasChanged()`) return non-zero values that differ from a sibling
**When** two `record.WithID[string]` values with the same `ID` but different non-`Data` interface state are diffed
**Then** the resulting candidate `Status == Matched` (non-`Data` interface state is not compared).

#### AC-2: struct-vs-map-kind-mismatch

**Given** baseline `u1.Data() == User{FirstName:"Alex"}` (struct) and candidate `u1.Data() == map[string]any{"FirstName":"Alex"}` (map)
**When** `Diff` is iterated
**Then** the emitted `IDDiff` has `Candidates[0].Status == Changed` with `Fields == [{Name:"_value", Value: <the candidate's map>}]`.

#### AC-3: incomparable-field-recovered

**Given** records whose `Data()` returns a struct containing a `func()` field
**When** `Diff` is iterated
**Then** the iteration yields `(zero, err)` where `errors.Is(err, recordops.ErrIncomparableField)` and stops (no panic propagates).

#### AC-4: slice-data-emits-value-field

**Given** baseline `u1.Data() == []int{1,2,3}` and candidate `u1.Data() == []int{1,2,4}`
**When** `Diff` is iterated
**Then** the emitted `IDDiff.Candidates[0].Fields` has exactly one entry with `Name == "_value"` and `Value == <the candidate's slice>` (per-index slice diffing is intentionally NOT performed in MVP).

### REQ: options

The package MUST export the option type and four options:

```go
type Option func(*options)

func WithIgnoreFields(fieldNames ...string) Option
func WithIncludeMatched() Option
func WithOnlyChangedFields() Option
func WithAbsentEqualsNil() Option
```

**`WithIgnoreFields(fields...)`** — instructs `Diff` to omit named fields from comparison. Matching is by Go struct field name (when `Data()` returns a struct) or map key (when `Data()` returns `map[string]any`). Case-sensitive. Multiple calls compose additively. Unknown names are silently ignored.

**`WithIncludeMatched()`** — emit `IDDiff` for every ID touched by any input, including fully-matched IDs (where every candidate is `Matched`). Default is to skip those.

**`WithOnlyChangedFields()`** — trim `IDDiff.Baseline.Fields` to only the fields that have a delta in at least one candidate. Default is to populate the full baseline record snapshot.

**`WithAbsentEqualsNil()`** — during field comparison, treat "field absent from a record" as equivalent to "field present with `nil` value." Default is to distinguish the two (per the `Absent` flag on `FieldValue`). Use this when the dataset is sourced from heterogeneous backends where one stores "no value" as an absent column and another stores it as `NULL`, and the consumer wants them to compare equal. When the option is set: a baseline field with `nil` value and a candidate that lacks the field (or vice versa) does NOT produce a delta. Records whose differences all reduce to absent-vs-nil report `Status == Matched`.

The four options are orthogonal; consumers may combine any subset.

#### AC-1: with-ignore-fields-skips-comparison

**Given** baseline `u1 = {FirstName:"Alex", UpdatedAt:t1}` and candidate `u1 = {FirstName:"Alex", UpdatedAt:t2}` (only timestamp differs)
**When** `Diff(b, c, WithIgnoreFields("UpdatedAt"))` is iterated
**Then** the only emitted `IDDiff` (if any — under default mode, none, since the only diff was ignored) confirms `Candidates[0].Status == Matched`, OR no `IDDiff` is emitted at all.

#### AC-2: with-include-matched-emits-matched

**Given** baseline and a candidate that fully match
**When** `Diff(b, c, WithIncludeMatched())` is iterated
**Then** every baseline ID produces an `IDDiff` with `Candidates[0].Status == Matched`, `Candidates[0].Fields == nil`, and `Baseline.Fields` populated.

#### AC-3: with-only-changed-fields-trims-baseline

**Given** baseline `u3 = {A:1, B:2, C:3}` and candidate `u3 = {A:1, B:9, C:3}` (only `B` differs)
**When** `Diff(b, c, WithOnlyChangedFields())` is iterated and the `IDDiff` for `u3` is read
**Then** `Baseline.Fields == [{Name:"B", Value:1}]` only — `A` and `C` are omitted from baseline. `Candidates[0].Fields == [{Name:"B", Value:9}]`.

#### AC-4: full-baseline-by-default

**Given** the same inputs as AC-3
**When** `Diff(b, c)` is iterated (no `WithOnlyChangedFields`)
**Then** `Baseline.Fields` contains three entries (`A`, `B`, `C`) in stable order; `Candidates[0].Fields` still contains only `B` (deltas only on candidate side, by design).

#### AC-5: options-compose

**Given** baseline + candidates where the only differences are in fields named `UpdatedAt`
**When** `Diff(b, c, WithIgnoreFields("UpdatedAt"), WithIncludeMatched(), WithOnlyChangedFields())` is iterated
**Then** every ID emits with `Candidates[i].Status == Matched`, `Baseline.Fields == nil` (no candidate had a delta after ignoring `UpdatedAt`), `Candidates[i].Fields == nil`.

#### AC-6: with-absent-equals-nil-collapses-absent-to-match

**Given** baseline `u1.Data() == map[string]any{"nickname":nil}` and candidate `u1.Data() == map[string]any{}` (candidate lacks `nickname` entirely)
**When** `Diff(b, c, WithAbsentEqualsNil())` is iterated
**Then** no `IDDiff` is emitted for `u1` (default mode skips fully-matched), OR if combined with `WithIncludeMatched()`: `Candidates[0].Status == Matched` and `Candidates[0].Fields == nil`.

#### AC-7: with-absent-equals-nil-only-collapses-when-baseline-is-nil

**Given** baseline `u1.Data() == map[string]any{"nickname":"Al"}` (non-nil value) and candidate `u1.Data() == map[string]any{}` (candidate lacks the field)
**When** `Diff(b, c, WithAbsentEqualsNil())` is iterated
**Then** `Candidates[0].Status == Changed` and `Candidates[0].Fields` contains `{Name:"nickname", Absent:true}` — the option does NOT erase the delta when baseline has a real non-nil value.

### REQ: bridge-helpers

The package MUST export two helpers for constructing `RecordSeq` streams from common sources:

```go
// SliceToSeq turns an already-sorted slice into a RecordSeq.
// The slice MUST be sorted ascending by ID; SliceToSeq does NOT sort.
func SliceToSeq[K comparable](records []record.WithID[K]) RecordSeq[K]

// ReaderToSeq adapts a dalgo dal.RecordsReader to a RecordSeq.
// idOf extracts the ID from each dal.Record yielded by the reader.
// Reader errors propagate via the seq2 error channel.
// Close() is called when iteration completes (success or error).
func ReaderToSeq[K comparable](
    r dal.RecordsReader,
    idOf func(dal.Record) (K, error),
) RecordSeq[K]
```

#### AC-1: slice-to-seq-yields-in-order

**Given** a slice `[{ID:"a"}, {ID:"b"}, {ID:"c"}]`
**When** iterated via `SliceToSeq(slice)`
**Then** the yields are `(records[0], nil), (records[1], nil), (records[2], nil)` in that order, then iteration ends.

#### AC-2: reader-to-seq-propagates-error

**Given** a mock `dal.RecordsReader` that yields one record then returns `io.ErrUnexpectedEOF`
**When** iterated via `ReaderToSeq(reader, idOf)`
**Then** the iteration yields one valid `(record.WithID, nil)` pair followed by `(zero, err)` where `errors.Is(err, io.ErrUnexpectedEOF)`.

#### AC-3: reader-to-seq-closes-reader

**Given** a mock `dal.RecordsReader` whose `Close()` increments a counter
**When** iteration over `ReaderToSeq(reader, idOf)` ends — whether by exhausting records, by the consumer breaking out of the range loop early, or by an upstream stream error (e.g., the mock returning `io.ErrUnexpectedEOF` mid-stream)
**Then** `Close()` was called exactly once across all three end conditions.

### REQ: renderer-stream-consumption

Each renderer iterates its input stream **exactly once**. A given `iter.Seq2[IDDiff[K], error]` returned by `Diff` / `DiffFunc` cannot be passed to multiple renderers — its underlying source (a `dal.RecordsReader`, a `SliceToSeq` over a slice that's already been drained, a chained pipeline) is typically one-shot. Consumers that need multiple views (e.g., both the per-candidate git-style and the cross-candidate by-ID renderers) MUST materialize first via `slices.Collect`-equivalent into `[]IDDiff[K]`, then wrap each copy with a fresh `iter.Seq2` (e.g., a small loop yielding from the slice). The Feature does NOT ship a dedicated `Collect` helper in MVP — Go 1.23+'s `iter` package provides the patterns directly.

#### AC-1: one-shot-stream-documented

**Given** the `recordops` package
**When** inspected via `go doc github.com/dal-go/dalgo/recordops` (package doc) and the godoc of each renderer
**Then** each doc block notes that the renderer consumes the stream once and directs multi-view consumers to materialize first.

### REQ: absent-field-rendering

When a Changed candidate's `FieldValue` has `Absent == true` (per the field-absence encoding in REQ `id-diff-shape`), renderers MUST emit a representation that is structurally distinct from a real Go-nil value:

- `RenderYAMLGitStyle`: emit ONLY the `-   <name>: <oldValue>\n` line (from baseline). Suppress the `+` line entirely. This matches git-diff semantics — a removed line is just a deletion. The rule applies symmetrically: a field present in the candidate that doesn't exist in baseline within a Changed record is rare under MVP (the kind-bucket dispatch handles whole-record bucket changes), but if it occurs, emit only the `+   <name>: <newValue>\n` line.
- `RenderYAMLByID`: emit the field as a nested map `<name>:\n  absent: true\n` instead of `<name>: <value>`. The `absent: true` form is unambiguous — a real Go-nil value renders as `<name>: null` (or `~`), structurally distinct.
- `RenderYAML` and `RenderJSON`: serialize the full `FieldValue` struct including the `Absent` field. JSON tag is `"absent"`; consumers reading the structured output check `Absent` before interpreting `Value`. JSON output for an absent field: `{"Name":"nickname","Value":null,"Absent":true}`.

The package does NOT export any "absent" sentinel constant — `FieldValue.Absent` carries the structural information; renderers handle the visual representation per their format.

#### AC-1: git-style-omits-plus-line-for-absent

**Given** baseline `u1.Data() == map[string]any{"first_name":"Alex", "nickname":"Al"}` and candidate `u1.Data() == map[string]any{"first_name":"Alex"}` (candidate lacks `nickname`)
**When** `RenderYAMLGitStyle(Diff(b, c), 0, "users")` is invoked
**Then** the rendered output for `u1` includes the line `-   nickname: Al\n` and does NOT include any corresponding `+   nickname:` line.

#### AC-2: by-id-renders-absent-flag

**Given** the same inputs as AC-1
**When** `RenderYAMLByID(Diff(b, c), "users")` is invoked
**Then** the rendered YAML for `u1`'s candidate 0 contains a `nickname:` block with `absent: true` (a nested map), distinguishing it from a real null value.

#### AC-3: structured-renderers-preserve-absent-flag

**Given** the same inputs as AC-1
**When** `RenderJSON(Diff(b, c), "users")` is invoked and the output is parsed back via `encoding/json`
**Then** the parsed result contains a `FieldValue` entry with `Absent == true` (and `Value == nil`). The same holds for `RenderYAML` (parsed back via `gopkg.in/yaml.v3`).

#### AC-4: nil-value-distinct-from-absent-in-rendering

**Given** baseline `u1.Data() == map[string]any{"first_name":"Alex"}` and candidate `u1.Data() == map[string]any{"first_name":nil}` (candidate has the field with a real Go-nil value)
**When** `RenderYAMLByID(Diff(b, c), "users")` is invoked
**Then** the rendered YAML for `u1`'s candidate 0 contains `first_name: null` (or `~`) — NOT `first_name: { absent: true }`. The two cases are visibly distinct.

### REQ: render-yaml-git-style

The package MUST export:

```go
func RenderYAMLGitStyle[K comparable](
    diffs iter.Seq2[IDDiff[K], error],
    candidateIndex int,
    collectionName string,
) (string, error)
```

The renderer emits the per-candidate view for `candidateIndex` only, in git-diff-style YAML matching the source Idea's example. It pulls from the diff stream once and demultiplexes internally — the caller never sees other candidates' state.

Output rules for each `IDDiff` whose `Candidates[candidateIndex]` is non-`Matched`:

- `Status == Missing`: emit `- <id>\n`
- `Status == Extra`: emit `+ <id>:\n`, then one line per `FieldValue` in the candidate's `Fields` (alphabetical by `Name`): four leading spaces, `<name>: <value>\n` where `<value>` is `fmt.Sprintf("%v", v.Value)`.
- `Status == Changed`: emit `<id>:\n`, then for each `FieldValue` in the candidate's `Fields` (alphabetical by `Name`), emit: a `-   <name>: <oldValue>\n` line (looking up the baseline `Value` by `Name` in `IDDiff.Baseline.Fields`), AND a `+   <name>: <newValue>\n` line (using the candidate's `Value`) — UNLESS `Absent == true` for the candidate FieldValue, in which case the `+` line is omitted entirely (the absence is expressed by the lone `-` line, matching git-diff semantics). Marker + 3 spaces; lines aligned at column 5. See REQ `absent-field-rendering`.

`Matched` candidates and IDs where this candidate has no entry are silently skipped.

The first line is `<collectionName>:\n`. If no `IDDiff` produces output, the final returned string is `<collectionName>: {}\n` (explicit empty collection).

`collectionName == ""` produces a malformed first line starting with `:` — does NOT panic. Documented in godoc.

If the diff stream yields an error, the renderer returns `("", err)`.

#### AC-1: matches-idea-example

**Given** a baseline producing `[{ID:"u1", Data: nil}, {ID:"u3", Data: &User{first_name:"Alex"}}]` and a single candidate producing `[{ID:"u2", Data: &User{first_name:"Jack", gender:"male"}}, {ID:"u3", Data: &User{first_name:"Alexander"}}]`
**When** `RenderYAMLGitStyle(Diff(b, c), 0, "users")` is called
**Then** the returned string is byte-equal to:

```yaml
users:
- u1
+ u2:
    first_name: Jack
    gender: male
u3:
-   first_name: Alex
+   first_name: Alexander
```

(trailing newline included; 4 spaces of indent under `+ u2:`; marker + 3 spaces (column 5) for changed-field lines).

#### AC-2: empty-diff-renders-cleanly

**Given** a baseline and a single candidate that fully match
**When** `RenderYAMLGitStyle(Diff(b, c), 0, "users")` is called
**Then** the returned string is `"users: {}\n"`.

#### AC-3: matched-candidate-skipped

**Given** an IDDiff stream emitted under `WithIncludeMatched()` containing only `Matched` candidates at the target index
**When** `RenderYAMLGitStyle(..., targetIdx, "users")` is called
**Then** the returned string is `"users: {}\n"` (Matched contributes nothing to the per-candidate view).

#### AC-4: stream-error-propagated

**Given** a diff stream that yields one valid `IDDiff` and then `(zero, io.ErrUnexpectedEOF)`
**When** `RenderYAMLGitStyle(stream, 0, "users")` is called
**Then** the returned `(string, error)` is `("", err)` where `errors.Is(err, io.ErrUnexpectedEOF)`.

### REQ: render-yaml-by-id

The package MUST export:

```go
func RenderYAMLByID[K comparable](
    diffs iter.Seq2[IDDiff[K], error],
    collectionName string,
) (string, error)
```

This is the cross-candidate divergence view — one block per emitted `IDDiff`, showing baseline (if present) and each candidate (in index order) with its status and any deltas. The format is human-oriented and need not be byte-equal across runs; it MUST be valid YAML and MUST be deterministic for a given input.

Each block has shape (illustrative):

```yaml
u3:
  baseline:
    first_name: Charlie
    nickname: Chuck
  candidates:
    "0":
      status: changed
      fields:
        first_name:
          new: Charles
        nickname:
          absent: true   # field absent from candidate 0 — per REQ `absent-field-rendering`
    "1":
      status: missing
```

#### AC-1: produces-valid-yaml

**Given** any diff stream
**When** `RenderYAMLByID(stream, "users")` is called and parsed via `gopkg.in/yaml.v3`
**Then** parsing succeeds.

#### AC-2: includes-all-candidates

**Given** an `IDDiff` with `len(Candidates) == 3`
**When** rendered
**Then** the YAML block for that ID contains three candidate sub-blocks keyed `"0"`, `"1"`, `"2"` (parallel index preserved).

#### AC-3: deterministic

**Given** a materialized `[]IDDiff` re-wrapped twice into independent `iter.Seq2` streams
**When** `RenderYAMLByID(stream1, "users")` and `RenderYAMLByID(stream2, "users")` are called
**Then** the two returned strings are byte-equal.

### REQ: render-yaml-and-json

The package MUST export:

```go
func RenderYAML[K comparable](
    diffs iter.Seq2[IDDiff[K], error],
    collectionName string,
) (string, error)

func RenderJSON[K comparable](
    diffs iter.Seq2[IDDiff[K], error],
    collectionName string,
) (string, error)
```

These produce the full structured serialization of the diff stream via `gopkg.in/yaml.v3` and `encoding/json` respectively. Output MUST be deterministic for a given input.

#### AC-1: json-round-trips

**Given** any diff stream produced from scalar-typed records
**When** `RenderJSON(stream, "users")` is called and the output is parsed back via `encoding/json`
**Then** parsing succeeds and the parsed value preserves the ID list and Status values of the original stream.

#### AC-2: render-determinism

**Given** the same materialized diff `[]IDDiff` re-wrapped twice into separate `iter.Seq2`
**When** `RenderYAML` is called on each and `RenderJSON` is called on each
**Then** the two YAML outputs are byte-equal, and the two JSON outputs are byte-equal.

### REQ: godoc

Each exported symbol — package doc, `Diff`, `DiffFunc`, `RecordSeq`, `IDDiff`, `RecordSnapshot`, `CandidateState`, `FieldValue`, `RecordStatus`, the four `RecordStatus` constants, `Option`, `WithIgnoreFields`, `WithIncludeMatched`, `WithOnlyChangedFields`, `WithAbsentEqualsNil`, `SliceToSeq`, `ReaderToSeq`, `RenderYAMLGitStyle`, `RenderYAMLByID`, `RenderYAML`, `RenderJSON`, `ErrUnsortedInput`, `ErrDuplicateID`, `ErrIncomparableField`, `ErrInvalidArgument` — MUST carry a godoc comment that explains its purpose in one or two sentences.

The package doc MUST explain the K-way merge model, the streaming contract (sorted input required), and the per-`IDDiff` shape. The `DiffFunc` godoc MUST be accompanied by `func ExampleDiffFunc()` in `diff_example_test.go` (external test package `recordops_test`) demonstrating UUID-keyed usage with `bytes.Compare(a[:], b[:]) < 0` — this keeps the production package free of a `bytes` import.

#### AC-1: godoc-present

**Given** the `recordops` package
**When** inspected via `go doc github.com/dal-go/dalgo/recordops` and `go doc github.com/dal-go/dalgo/recordops <Symbol>` for each exported symbol listed above
**Then** each command returns a non-empty doc block.

## Architecture

| File | Change |
|---|---|
| `recordops/doc.go` (new) | Package doc + conceptual overview. |
| `recordops/errors.go` (new) | Sentinel errors: `ErrUnsortedInput`, `ErrDuplicateID`, `ErrIncomparableField`, `ErrInvalidArgument`. (No `AbsentValue` constant — absence is signaled via `FieldValue.Absent` per REQ `absent-field-rendering`.) |
| `recordops/options.go` (new) | `Option`, internal `options` struct, `WithIgnoreFields`, `WithIncludeMatched`, `WithOnlyChangedFields`, `WithAbsentEqualsNil`. |
| `recordops/diff.go` (new) | `Diff` + `DiffFunc` entrypoints; both delegate to an unexported `diffFunc` that holds the K-way merge driver. |
| `recordops/compare.go` (new) | `compareRecords` — `Data()` extraction, deref, kind-bucket dispatch, panic recovery. |
| `recordops/types.go` (new) | `IDDiff`, `RecordSnapshot`, `CandidateState`, `FieldValue`, `RecordStatus` + constants, `RecordSeq` alias. |
| `recordops/bridge.go` (new) | `SliceToSeq`, `ReaderToSeq`. |
| `recordops/render_yaml_gitstyle.go` (new) | `RenderYAMLGitStyle`. |
| `recordops/render_yaml_byid.go` (new) | `RenderYAMLByID`. |
| `recordops/render_yaml.go` (new) | `RenderYAML`. |
| `recordops/render_json.go` (new) | `RenderJSON`. |
| `recordops/diff_test.go` (new) | Tests for `Diff` / `DiffFunc` covering ACs of REQs `input-streams`, `diff-entrypoints`, `id-diff-shape`, `emission-rules`. |
| `recordops/compare_test.go` (new) | Tests for `field-comparison` ACs. |
| `recordops/options_test.go` (new) | Tests for the three options + composition. |
| `recordops/bridge_test.go` (new) | Tests for `SliceToSeq`, `ReaderToSeq` (incl. Close() bookkeeping). |
| `recordops/render_yaml_gitstyle_test.go` (new) | Golden test pinning the source-Idea example + edge cases (empty diff, malformed name, stream error). |
| `recordops/render_yaml_byid_test.go` (new) | Cross-candidate divergence renderer tests. |
| `recordops/render_test.go` (new) | YAML / JSON renderer determinism + round-trip. |
| `recordops/diff_example_test.go` (new, package `recordops_test`) | `func ExampleDiffFunc()` with UUID + `bytes.Compare`. |
| `recordops/README.md` (new) | Package-level README with one worked end-to-end example covering both the per-candidate and cross-candidate renderers. |
| `go.mod` | Add `gopkg.in/yaml.v3` dependency. |

No other dalgo packages are touched.

### Data flow

```
baseline iter.Seq2          candidates[0] iter.Seq2     candidates[N-1] iter.Seq2
   (sorted by ID)                  (sorted)        ...        (sorted)
       │                              │                          │
       ▼                              ▼                          ▼
  ┌─────────────────────────────────────────────────────────────────┐
  │             K-way merge driver (diff.go + compare.go)            │
  │   - monotonicity + duplicate validation per stream               │
  │   - find smallest ID across all heads                            │
  │   - classify per candidate (Missing / Extra / Matched / Changed) │
  │   - compute baseline snapshot + per-candidate deltas             │
  │   - apply WithIgnoreFields / WithIncludeMatched /                │
  │     WithOnlyChangedFields filters                                │
  │   - emit one IDDiff per ID that survives filtering               │
  └─────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
                  iter.Seq2[IDDiff[K], error]
                                  │
              ┌───────────────────┼────────────────────┬──────────────────┐
              ▼                   ▼                    ▼                  ▼
   RenderYAMLGitStyle      RenderYAMLByID        RenderYAML          RenderJSON
   (per-candidate,         (cross-candidate      (structured        (structured
    git-diff style)         divergence view)      YAML serial)       JSON serial)
```

## Error handling & failure modes

| Condition | Behavior |
|---|---|
| `baseline` or any `candidates[i]` is nil | Treated as empty stream; no error. |
| `candidates == nil` | Returns a stream that yields zero items (zero `IDDiff`s, no error). |
| `DiffFunc` called with `less == nil` | Stream yields exactly one `(zero, fmt.Errorf("recordops.DiffFunc: less must not be nil: %w", ErrInvalidArgument))` and stops. |
| Unsorted input on any stream | Stream yields `(zero, fmt.Errorf("recordops: stream out of order at id %v in %s: %w", id, where, ErrUnsortedInput))` and stops. |
| Duplicate ID on any stream | Stream yields `(zero, fmt.Errorf("recordops: duplicate id %v in %s: %w", id, where, ErrDuplicateID))` and stops. |
| Upstream stream error (e.g., DB read failure) | Propagated as `(zero, err)` and iteration stops. |
| `reflect.DeepEqual` panics on an incomparable field | Recovered; stream yields `(zero, fmt.Errorf("recordops: incomparable field %q in record %v: %w", field, id, ErrIncomparableField))` and stops. |
| `RenderYAMLGitStyle("", ...)` empty collection name | Does not panic; renders a malformed first line starting with `:`. |
| Renderer encounters a stream error | Returns `("", err)`. |

`where` is `"baseline"` or `fmt.Sprintf("candidates[%d]", i)`. Error prefixes are neutral (`recordops:`) for shared validation paths and function-prefixed (`recordops.DiffFunc:`) only for entrypoint-specific errors — keeps `Diff` ↔ `DiffFunc` delegation equivalence trivial.

## Out of Scope (this Feature)

Carried from the source Idea plus Feature-level cuts:

- **Diffing over the columnar `recordset.Recordset` interface** — different shape; future Feature if a real consumer appears.
- **Internal sort of unsorted input** — caller's responsibility (SQL `ORDER BY id`; `slices.SortFunc` upstream).
- **Schema-aware diffing (column adds/drops)** — belongs with `dalgo-schema-modification`.
- **CLI tool.**
- **Patch application** (apply a diff back to a recordset).
- **Pluggable equality (custom `Equal` per field) and JSON-tag-aware field names** — `reflect.DeepEqual` only in MVP. (`WithIgnoreFields` ships.)
- **`DiffMap[K, V]` entrypoint** — `map`-keyed inputs deferred; can land later as a real implementation.
- **A hash-map-based variant that drops the ordering requirement** — `DiffFunc` covers non-`cmp.Ordered` keys via explicit comparator, and the renderers want ID-sorted output anyway.
- **Per-index diffing for slice-valued fields** — slices compared as a single virtual `_value` (REQ `field-comparison` AC-4).
- **`dal.RecordsReader → iter.Seq2` migration in dalgo proper** — tracked in `dal-records-reader-iter-seq` Idea; recordops ships its own bridge.
- **Surfacing `dal.Reader.Cursor()` through `ReaderToSeq`** — the bridge calls `Close()` exactly once on iteration end but does NOT expose the underlying reader's pagination cursor. Callers needing pagination MUST drive the reader directly until the sibling iter.Seq migration lands.
- **Streaming the renderer output to an `io.Writer`** — renderers currently materialize the full output string. If giant per-candidate diffs become a real concern, an `io.Writer`-based variant lands as a follow-up.

## Testing strategy

All tests are in-tree Go tests using `testing` + `testify`:

- **Stream-contract tests** for monotonicity, duplicate-rejection, upstream-error propagation, and zero-candidates / nil-baseline edges.
- **Behavioral tests** for the five `diff-entrypoints` ACs (string keys, UUID via `DiffFunc`, delegation equivalence, nil-less rejection, zero candidates).
- **Shape tests** for the four `id-diff-shape` ACs (parallel index, baseline-nil for Extra-only IDs, Matched fields nil, Changed fields carry only deltas).
- **Emission tests** for the three `emission-rules` ACs (skip-fully-matched, include-matched, id-ordering).
- **Field-comparison tests** for the four `field-comparison` ACs.
- **Options tests** for the five `options` ACs including composition.
- **Bridge tests** for `SliceToSeq` ordering and `ReaderToSeq` Close() / error propagation.
- **Renderer tests**: golden byte-equality for `RenderYAMLGitStyle` against the source-Idea example; valid-YAML parsing for `RenderYAMLByID`; round-trip + determinism for `RenderYAML` / `RenderJSON`.
- **Example test**: `ExampleDiffFunc` in the external test package demonstrating UUID usage.

No external integration tests at this layer.

## Rehearse Integration

All ACs are testable via Go's built-in test runner. AC verification lands in the test files listed in the Architecture table. Rehearse stub files are intentionally skipped — the entire Feature is verifiable in `go test ./recordops/...`.

## Assumption Carryover

From the source Idea:

| Idea assumption | Status in Feature |
|---|---|
| Must-be-true: callers can deliver sorted streams. | Carried; enforced by REQ `input-streams` (monotonicity check + `ErrUnsortedInput`). |
| Must-be-true: ID type buckets — `cmp.Ordered` via `Diff`, others via `DiffFunc`. | Carried; pinned by REQs `diff-entrypoints` AC-1 and AC-2 (UUID via `DiffFunc`). |
| Must-be-true: per-field equality via `Record.Data()` not interface header. | Carried and pinned by REQ `field-comparison` AC-1. |
| Must-be-true: git-style YAML is the must-have format. | Carried; pinned by REQ `render-yaml-git-style` AC-1 (golden test). |
| Should-be-true: cross-candidate divergence view is more natural for multi-backend audits. | Carried; addressed by REQ `render-yaml-by-id`. |
| Should-be-true: K-way merge over streams meaningfully beats N independent diffs. | Carried at the Idea level; not testable as an AC (it's an algorithmic property of the implementation). |
| Should-be-true: `recordops` will host 2–4 future helpers. | Carried by parent umbrella; future helpers are sibling Features. |
| Might-be-true: `WithOnlyChangedFields()` will become the more common consumer choice. | Carried as the default-include design tenet; the option is in scope (REQ `options`). |
| Might-be-true: `iter.Seq2`-based error propagation is ergonomic. | Carried via REQ `input-streams` AC-4 + the renderers' stream-error tests. |

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
