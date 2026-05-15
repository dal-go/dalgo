# recordops/diff — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land a new top-level dalgo sub-package `recordops` with one capability: `Diff` — a streaming, one-baseline-vs-N-candidates comparison over ID-sorted `iter.Seq2` inputs, returning `iter.Seq2[IDDiff[K], error]`. Ship two parallel entrypoints (`Diff` for `cmp.Ordered` keys; `DiffFunc` for `comparable` with explicit `less`), four options (`WithIgnoreFields`, `WithIncludeMatched`, `WithOnlyChangedFields`, `WithAbsentEqualsNil`), four renderers (git-style YAML, by-ID YAML, plain YAML, JSON), and two bridge helpers (`SliceToSeq`, `ReaderToSeq`).

**Architecture:** Single new package `recordops/` with no DB-driver dependencies. Imports only `record`, `dal` (interface-only for the `ReaderToSeq` bridge), `cmp`, `iter`, standard library, plus `gopkg.in/yaml.v3` for YAML serialization. Both `Diff` and `DiffFunc` delegate to one internal merge-join driver — the only difference is the source of the `less` function. `DiffResult` no longer exists; the structured output is the `iter.Seq2[IDDiff[K], error]` stream.

**Tech Stack:** Go 1.24+, `iter` package (Go 1.23+ — already on the dalgo floor), `cmp.Ordered` (Go 1.21+), `gopkg.in/yaml.v3`, `testing` + `github.com/stretchr/testify/assert` (matches existing dalgo test convention).

**Source-code linking (REQUIRED on every new file).** Every new `.go` and `.md` file added under `recordops/` MUST carry a SpecScore link comment near the top, pointing at the feature this code implements. Use either form:

```go
// specscore: feat-recordops/diff
```

or the bare-URL form:

```go
// https://specscore.md/dalgo@dal-go@github.com/spec/features/recordops/diff
```

The plan itself is also linkable from code via the plan ID (`plan-2026-05-15-recordops-diff`) when a particular file maps to a specific task. Place the comment either as a stand-alone line near the top of the file (after the package clause) or attached to the primary symbol the file defines. The `specscore code deps` command scans for these annotations to produce traceability reports — implementations that skip the comment will not appear in the audit.

For Markdown files (e.g., `recordops/README.md`), use an HTML comment: `<!-- specscore: feat-recordops/diff -->`.

**Spec:** [`spec/features/recordops/diff/README.md`](../features/recordops/diff/README.md)

**Source Idea:** [`spec/ideas/recordops.md`](../ideas/recordops.md)

**Related Idea (out of scope for this plan):** [`spec/ideas/dal-records-reader-iter-seq.md`](../ideas/dal-records-reader-iter-seq.md) — when this lands, `ReaderToSeq` may become a thin wrapper or be deleted.

---

## File Map

| Action | File | Change |
|--------|------|--------|
| Create | `recordops/doc.go` | Package doc + conceptual overview (K-way merge, sorted-input contract, per-IDDiff shape). |
| Create | `recordops/errors.go` | Sentinels: `ErrUnsortedInput`, `ErrDuplicateID`, `ErrIncomparableField`, `ErrInvalidArgument`. |
| Create | `recordops/types.go` | `RecordSeq[K]` alias, `IDDiff[K]`, `RecordSnapshot`, `CandidateState`, `FieldValue`, `RecordStatus` + four constants. |
| Create | `recordops/options.go` | `Option` type, internal `options` struct, `WithIgnoreFields`, `WithIncludeMatched`, `WithOnlyChangedFields`, `WithAbsentEqualsNil`. |
| Create | `recordops/bridge.go` | `SliceToSeq`, `ReaderToSeq` (adapts `dal.RecordsReader`, calls `Close()` exactly once on any termination). |
| Create | `recordops/compare.go` | `compareRecords` — `Data()` extraction, pointer deref, kind-bucket dispatch (struct/map/other), absence encoding, panic recovery. |
| Create | `recordops/diff.go` | `Diff` + `DiffFunc` entrypoints; internal K-way merge driver `diffFunc`. |
| Create | `recordops/render_yaml_gitstyle.go` | `RenderYAMLGitStyle` — per-candidate; absent fields omit the `+` line entirely (git-diff semantics for removal). See REQ `absent-field-rendering`. |
| Create | `recordops/render_yaml_byid.go` | `RenderYAMLByID` — cross-candidate divergence view. |
| Create | `recordops/render_yaml.go` | `RenderYAML` — structured YAML via `gopkg.in/yaml.v3`. |
| Create | `recordops/render_json.go` | `RenderJSON` — structured JSON via `encoding/json`. |
| Create | `recordops/diff_test.go` | Tests for input streams, entrypoints, id-diff shape, emission rules. |
| Create | `recordops/compare_test.go` | Tests for field-comparison ACs. |
| Create | `recordops/options_test.go` | Tests for the three options and their composition. |
| Create | `recordops/bridge_test.go` | Tests for `SliceToSeq` and `ReaderToSeq` (incl. `Close()` across all termination paths). |
| Create | `recordops/render_yaml_gitstyle_test.go` | Golden test pinning the source-Idea example; empty diff; empty collection name; absent-value rendering; stream-error propagation. |
| Create | `recordops/render_yaml_byid_test.go` | Cross-candidate renderer: validity, includes-all-candidates, determinism, absent-value. |
| Create | `recordops/render_test.go` | YAML/JSON: round-trip, parses-as-valid-yaml, determinism, nil-as-language-native. |
| Create | `recordops/diff_example_test.go` (package `recordops_test`) | `ExampleDiffFunc` with UUID + `bytes.Compare` — keeps production package free of `bytes`. |
| Create | `recordops/README.md` | Package-level README: one worked end-to-end example covering per-candidate and cross-candidate renderers. |
| Modify | `go.mod` | Add `gopkg.in/yaml.v3` dependency. |

No other dalgo packages are touched.

---

## Task 1: Errors, sentinels, and types

**Files:**
- Create: `recordops/errors.go`
- Create: `recordops/types.go`
- Create: `recordops/options.go`
- Create: `recordops/doc.go`
- Create: `recordops/options_test.go` (initial smoke tests only — option semantics tested in Task 5)

- [ ] **Step 1: Write the failing smoke test for type/option presence**

Create `recordops/options_test.go`:

```go
// specscore: feat-recordops/diff
package recordops

import (
	"errors"
	"testing"
)

// Smoke tests: every exported type/sentinel exists and has the expected zero value.
// Semantic tests for options live in Task 5.

func TestSentinels_Exist(t *testing.T) {
	for _, e := range []error{
		ErrUnsortedInput,
		ErrDuplicateID,
		ErrIncomparableField,
		ErrInvalidArgument,
	} {
		if e == nil {
			t.Fatalf("nil sentinel error")
		}
		// Self-Is invariant
		if !errors.Is(e, e) {
			t.Fatalf("sentinel %v is not errors.Is-itself", e)
		}
	}
}

func TestRecordStatus_Constants(t *testing.T) {
	if Missing != 0 || Extra != 1 || Matched != 2 || Changed != 3 {
		t.Fatalf("RecordStatus constant order is load-bearing: Missing=%d Extra=%d Matched=%d Changed=%d", Missing, Extra, Matched, Changed)
	}
}

func TestOptionsConstructors_DoNotPanic(t *testing.T) {
	_ = WithIgnoreFields()
	_ = WithIgnoreFields("a", "b")
	_ = WithIncludeMatched()
	_ = WithOnlyChangedFields()
	_ = WithAbsentEqualsNil()
}
```

- [ ] **Step 2: Run test to verify it fails (compile error — package doesn't exist yet)**

```bash
cd /Users/alexandertrakhimenok/projects/dal-go/dalgo
go test ./recordops/ 2>&1
```

Expected: `package github.com/dal-go/dalgo/recordops: build failed` or similar.

- [ ] **Step 3: Create the package skeleton**

Create `recordops/doc.go`:

```go
// Package recordops provides pure, dependency-free analytical helpers
// over collections of dalgo records.
//
// The first and only capability in MVP is [Diff] (and its sibling
// [DiffFunc]) — a streaming, single-pass comparison of one baseline
// recordset against N candidate recordsets. Inputs are pull-based
// iter.Seq2 streams that MUST be sorted ascending by ID. Output is
// also iter.Seq2: one [IDDiff] per ID where at least one candidate
// diverges from baseline. Use [WithIncludeMatched] to emit fully
// matched IDs too.
//
// The algorithm is a K-way merge over the N+1 input streams. Memory
// footprint at any point: O(N) records (one current per stream) plus
// the in-flight [IDDiff] being yielded.
//
// Each [IDDiff] carries the baseline snapshot once (the single source
// of truth) and per-candidate deltas — never duplicates of baseline
// values across candidates.
//
// Renderers translate the structured stream into output formats:
// [RenderYAMLGitStyle] (per-candidate git-diff style — the visual
// anchor that matches the source idea spec/ideas/recordops.md),
// [RenderYAMLByID] (cross-candidate divergence view), [RenderYAML]
// and [RenderJSON] (structured serialization).
//
// Renderers consume the input stream exactly once; consumers that
// need multiple views must materialize first via slices.Collect or
// equivalent.
//
// specscore: feat-recordops/diff
package recordops
```

Create `recordops/errors.go`:

```go
// specscore: feat-recordops/diff
package recordops

import "errors"

// ErrUnsortedInput indicates an input stream yielded a record whose ID
// is not strictly greater than the previously yielded ID from the same
// stream. Diff requires ID-sorted input streams.
var ErrUnsortedInput = errors.New("recordops: input stream not sorted ascending by ID")

// ErrDuplicateID indicates an input stream yielded two records with
// the same ID. Within a single stream, IDs must be unique.
var ErrDuplicateID = errors.New("recordops: duplicate ID in input stream")

// ErrIncomparableField indicates field comparison via reflect.DeepEqual
// panicked (e.g., a func or chan field). The panic is recovered and
// surfaced as a stream error wrapping this sentinel.
var ErrIncomparableField = errors.New("recordops: incomparable field")

// ErrInvalidArgument indicates a programmer error in calling Diff/DiffFunc
// (e.g., nil less function passed to DiffFunc).
var ErrInvalidArgument = errors.New("recordops: invalid argument")
```

(Note: no `AbsentValue` constant. The field-absence signal lives on `FieldValue.Absent` — see `types.go` below. Renderers handle the visual representation per their format; there's no magic string in the data model.)

Create `recordops/types.go`:

```go
// specscore: feat-recordops/diff
package recordops

import (
	"iter"

	"github.com/dal-go/dalgo/record"
)

// RecordSeq is the streaming input shape for Diff and DiffFunc.
// Implementations MUST yield records sorted ascending by ID and MUST
// propagate any source error as a (zero, err) pair (after which
// iteration stops).
type RecordSeq[K comparable] = iter.Seq2[record.WithID[K], error]

// IDDiff is the per-ID emission of Diff/DiffFunc. It carries the
// baseline snapshot (if baseline had this ID) and each candidate's
// state for this ID, in parallel-index order with the input candidates
// slice — Candidates[i] always describes input candidates[i].
type IDDiff[K comparable] struct {
	ID         K
	Baseline   *RecordSnapshot
	Candidates []CandidateState
}

// RecordSnapshot is baseline's record contents for a given ID — the
// single source of truth for field values. Candidates carry only
// deltas; consumers reading "the old value for a changed field"
// look it up here by Name.
type RecordSnapshot struct {
	Fields []FieldValue
}

// CandidateState describes one candidate's state for one ID:
//   - Status: Missing | Extra | Matched | Changed
//   - Fields: deltas only — never duplicates baseline values.
//     See the per-Status semantics in the Feature spec
//     (spec/features/recordops/diff/ REQ id-diff-shape).
type CandidateState struct {
	Status RecordStatus
	Fields []FieldValue
}

// FieldValue is used in BOTH RecordSnapshot.Fields and CandidateState.Fields.
// In RecordSnapshot.Fields, Value is the baseline's value for Name; Absent is always false.
// In CandidateState.Fields, Value is the candidate's value (only for Extra and
// Changed statuses; Missing and Matched have Fields == nil). When a field
// exists in baseline but is absent from a Changed candidate's record, Absent
// is true and Value is the zero value — consumers MUST NOT interpret Value
// when Absent is true. This is structurally distinct from Value == nil with
// Absent == false (a real Go-nil value the candidate explicitly holds).
//
// Name may be empty for future helpers that ingest positional/unnamed-column
// records; MVP comparison paths always produce non-empty Name.
type FieldValue struct {
	Name   string `json:"name"             yaml:"name"`
	Value  any    `json:"value,omitempty"  yaml:"value,omitempty"`
	Absent bool   `json:"absent,omitempty" yaml:"absent,omitempty"`
}

// RecordStatus classifies one candidate's relationship to baseline for one ID.
type RecordStatus int8

const (
	// Missing — baseline has this ID; this candidate doesn't.
	Missing RecordStatus = iota
	// Extra — this candidate has this ID; baseline doesn't.
	Extra
	// Matched — both have the ID; all fields equal.
	Matched
	// Changed — both have the ID; at least one field differs.
	Changed
)
```

Create `recordops/options.go`:

```go
// specscore: feat-recordops/diff
package recordops

// Option configures Diff/DiffFunc behavior. The package exports four
// orthogonal options:
//
//   - WithIgnoreFields(names...) — exclude named fields from comparison.
//   - WithIncludeMatched() — emit IDDiff for every ID, including fully matched.
//   - WithOnlyChangedFields() — trim Baseline.Fields to only fields with deltas.
//   - WithAbsentEqualsNil() — treat field-absent as equivalent to field-with-nil-value during comparison.
type Option func(*options)

// options is the internal aggregated configuration. Unexported.
type options struct {
	ignoreFields       map[string]struct{}
	includeMatched     bool
	onlyChangedFields  bool
	absentEqualsNil    bool
}

// WithIgnoreFields instructs Diff to omit named fields from comparison.
// Matching is by Go struct field name (when Record.Data() returns a struct)
// or by map key (when Record.Data() returns a map[string]any). Case-sensitive.
// Multiple calls compose additively. Unknown names are silently ignored.
//
// Canonical use case: WithIgnoreFields("UpdatedAt") drops a timestamp field
// that always changes between snapshots.
func WithIgnoreFields(names ...string) Option {
	return func(o *options) {
		if o.ignoreFields == nil {
			o.ignoreFields = make(map[string]struct{}, len(names))
		}
		for _, n := range names {
			o.ignoreFields[n] = struct{}{}
		}
	}
}

// WithIncludeMatched instructs Diff to emit IDDiff for every ID touched
// by any input — including IDs where every candidate is Matched. Default
// is to skip those.
func WithIncludeMatched() Option {
	return func(o *options) {
		o.includeMatched = true
	}
}

// WithOnlyChangedFields trims IDDiff.Baseline.Fields to only the fields
// that have a delta on at least one candidate. Default is to populate
// the full baseline record snapshot for context.
func WithOnlyChangedFields() Option {
	return func(o *options) {
		o.onlyChangedFields = true
	}
}

// WithAbsentEqualsNil instructs Diff to treat "field absent from a record"
// as equivalent to "field present with nil value" during comparison.
// Default is to distinguish the two via FieldValue.Absent. Use this when
// the dataset is sourced from heterogeneous backends where one stores
// "no value" as an absent column and another stores it as NULL.
//
// When set: a baseline field with nil value and a candidate that lacks
// the field (or vice versa) produces no delta. Records whose differences
// all reduce to absent-vs-nil report Status == Matched.
func WithAbsentEqualsNil() Option {
	return func(o *options) {
		o.absentEqualsNil = true
	}
}

// resolveOptions applies the option functions and returns the aggregated config.
func resolveOptions(opts ...Option) options {
	o := options{}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}
```

- [ ] **Step 4: Run smoke tests to verify they pass**

```bash
go test ./recordops/ -run 'TestSentinels|TestRecordStatus|TestOptionsConstructors' -v
```

Expected: all four tests PASS.

- [ ] **Step 5: Run `go vet ./recordops/` and confirm no warnings**

```bash
go vet ./recordops/
```

Expected: empty output (no warnings).

- [ ] **Step 6: Commit**

```bash
git add recordops/doc.go recordops/errors.go recordops/types.go recordops/options.go recordops/options_test.go
git commit -m "feat(recordops): add types, sentinels, and option constructors

Skeleton for the new recordops package: package doc, RecordSeq alias,
IDDiff/RecordSnapshot/CandidateState/FieldValue/RecordStatus, four
sentinel errors, and the four option constructors (WithIgnoreFields,
WithIncludeMatched, WithOnlyChangedFields, WithAbsentEqualsNil). All
exported symbols carry godoc. Option semantics tested in a later task.

Spec: spec/features/recordops/diff/ REQ id-diff-shape, options, godoc.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 2: Bridge helpers — SliceToSeq and ReaderToSeq

**Files:**
- Create: `recordops/bridge.go`
- Create: `recordops/bridge_test.go`

- [ ] **Step 1: Write failing tests**

Create `recordops/bridge_test.go`:

```go
// specscore: feat-recordops/diff
package recordops

import (
	"errors"
	"io"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSliceToSeq_YieldsInOrder(t *testing.T) {
	in := []record.WithID[string]{
		{ID: "a"}, {ID: "b"}, {ID: "c"},
	}
	seq := SliceToSeq(in)
	var got []string
	for r, err := range seq {
		require.NoError(t, err)
		got = append(got, r.ID)
	}
	assert.Equal(t, []string{"a", "b", "c"}, got)
}

func TestSliceToSeq_EmptyAndNil(t *testing.T) {
	for _, in := range [][]record.WithID[string]{nil, {}} {
		count := 0
		for _, err := range SliceToSeq(in) {
			require.NoError(t, err)
			count++
		}
		assert.Zero(t, count)
	}
}

// fakeReader is a minimal dal.RecordsReader for ReaderToSeq tests.
type fakeReader struct {
	records   []dal.Record
	idx       int
	err       error    // returned at position errAt (after records[errAt-1] is yielded)
	errAt     int
	closeCnt  int
}

func (r *fakeReader) Next() (dal.Record, error) {
	if r.err != nil && r.idx == r.errAt {
		return nil, r.err
	}
	if r.idx >= len(r.records) {
		return nil, dal.ErrNoMoreRecords
	}
	rec := r.records[r.idx]
	r.idx++
	return rec, nil
}
func (r *fakeReader) Cursor() (string, error) { return "", nil }
func (r *fakeReader) Close() error            { r.closeCnt++; return nil }

func TestReaderToSeq_PropagatesUpstreamError(t *testing.T) {
	rec1 := record.NewWithID("a", nil, nil).Record
	r := &fakeReader{
		records: []dal.Record{rec1},
		err:     io.ErrUnexpectedEOF,
		errAt:   1,
	}
	idOf := func(_ dal.Record) (string, error) { return "a", nil }

	var firstErr error
	count := 0
	for _, err := range ReaderToSeq[string](r, idOf) {
		count++
		if err != nil {
			firstErr = err
			break
		}
	}
	require.NotNil(t, firstErr)
	assert.True(t, errors.Is(firstErr, io.ErrUnexpectedEOF), "got %v", firstErr)
}

func TestReaderToSeq_ClosesOnAllPaths(t *testing.T) {
	tests := []struct {
		name string
		setup func() *fakeReader
		consume func(seq RecordSeq[string])
	}{
		{
			name: "exhaustion",
			setup: func() *fakeReader {
				rec := record.NewWithID("a", nil, nil).Record
				return &fakeReader{records: []dal.Record{rec}}
			},
			consume: func(seq RecordSeq[string]) {
				for range seq { /* drain */ }
			},
		},
		{
			name: "early-break",
			setup: func() *fakeReader {
				rec1 := record.NewWithID("a", nil, nil).Record
				rec2 := record.NewWithID("b", nil, nil).Record
				return &fakeReader{records: []dal.Record{rec1, rec2}}
			},
			consume: func(seq RecordSeq[string]) {
				for range seq { break /* take first only */ }
			},
		},
		{
			name: "upstream-error",
			setup: func() *fakeReader {
				return &fakeReader{
					records: []dal.Record{record.NewWithID("a", nil, nil).Record},
					err:     io.ErrUnexpectedEOF,
					errAt:   1,
				}
			},
			consume: func(seq RecordSeq[string]) {
				for range seq { /* drain — error terminates */ }
			},
		},
	}

	idOf := func(_ dal.Record) (string, error) { return "a", nil }

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.setup()
			tc.consume(ReaderToSeq[string](r, idOf))
			assert.Equal(t, 1, r.closeCnt, "Close() must be called exactly once")
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (compile error)**

```bash
go test ./recordops/ -run 'TestSliceToSeq|TestReaderToSeq' 2>&1
```

Expected: build failure with `undefined: SliceToSeq` / `undefined: ReaderToSeq`.

- [ ] **Step 3: Implement bridge helpers**

Create `recordops/bridge.go`:

```go
// specscore: feat-recordops/diff
package recordops

import (
	"errors"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/record"
)

// SliceToSeq turns an already-sorted slice into a RecordSeq.
// The slice MUST be sorted ascending by ID; SliceToSeq does NOT sort.
// A nil or empty slice produces a stream that yields zero items.
func SliceToSeq[K comparable](records []record.WithID[K]) RecordSeq[K] {
	return func(yield func(record.WithID[K], error) bool) {
		for _, r := range records {
			if !yield(r, nil) {
				return
			}
		}
	}
}

// ReaderToSeq adapts a dalgo dal.RecordsReader to a RecordSeq.
// idOf extracts the ID from each dal.Record yielded by the reader.
// Reader errors propagate via the seq2 error channel.
//
// The underlying reader is Closed exactly once when iteration ends —
// whether by exhausting records (dal.ErrNoMoreRecords), by the consumer
// breaking out of the range loop early, or by any upstream stream error.
//
// dal.Reader.Cursor() is NOT surfaced through this bridge in MVP;
// callers needing pagination must drive the reader directly. See
// spec/ideas/dal-records-reader-iter-seq.md.
func ReaderToSeq[K comparable](r dal.RecordsReader, idOf func(dal.Record) (K, error)) RecordSeq[K] {
	return func(yield func(record.WithID[K], error) bool) {
		defer func() { _ = r.Close() }()
		var zero record.WithID[K]
		for {
			rec, err := r.Next()
			if err != nil {
				if errors.Is(err, dal.ErrNoMoreRecords) {
					return
				}
				yield(zero, err)
				return
			}
			id, err := idOf(rec)
			if err != nil {
				yield(zero, err)
				return
			}
			wid := record.NewWithID(id, nil, nil)
			wid.Record = rec
			if !yield(wid, nil) {
				return
			}
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./recordops/ -run 'TestSliceToSeq|TestReaderToSeq' -v
```

Expected: all four `TestSliceToSeq_*` and `TestReaderToSeq_*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add recordops/bridge.go recordops/bridge_test.go
git commit -m "feat(recordops): add SliceToSeq and ReaderToSeq bridges

SliceToSeq lets callers stream from an already-sorted in-memory slice.
ReaderToSeq adapts a dalgo dal.RecordsReader to iter.Seq2, closing the
reader exactly once on any termination (exhaustion, early break, error).

Spec: REQ bridge-helpers.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 3: K-way merge core — Diff and DiffFunc

**Files:**
- Create: `recordops/diff.go`
- Create: `recordops/diff_test.go`

Implements the K-way merge driver with monotonicity/duplicate validation, parallel-index emission, default-mode filtering (skip fully-matched IDs). Field comparison is stubbed to return Matched-or-Changed via `==`; deeper semantics land in Task 4. Option semantics land in Task 5.

- [ ] **Step 1: Write failing tests covering input contract + entrypoints + emission rules**

Create `recordops/diff_test.go`:

```go
// specscore: feat-recordops/diff
package recordops

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mkRec[K comparable](id K, data any) record.WithID[K] {
	return record.NewWithID(id, nil, data)
}

func collect[K comparable](t *testing.T, seq iter.Seq2[IDDiff[K], error]) []IDDiff[K] {
	t.Helper()
	var out []IDDiff[K]
	for d, err := range seq {
		require.NoError(t, err)
		out = append(out, d)
	}
	return out
}

func TestDiff_NoCandidates(t *testing.T) {
	baseline := SliceToSeq([]record.WithID[string]{mkRec("a", nil)})
	got := collect(t, Diff[string](baseline, nil))
	assert.Empty(t, got)
}

func TestDiff_AddedDetected(t *testing.T) {
	baseline := SliceToSeq([]record.WithID[string]{mkRec("u1", nil)})
	cand := SliceToSeq([]record.WithID[string]{
		mkRec("u1", nil),
		mkRec("u2", map[string]any{"first_name": "Jack", "gender": "male"}),
	})
	got := collect(t, Diff[string](baseline, []RecordSeq[string]{cand}))
	require.Len(t, got, 1)
	assert.Equal(t, "u2", got[0].ID)
	require.Len(t, got[0].Candidates, 1)
	assert.Equal(t, Extra, got[0].Candidates[0].Status)
}

func TestDiff_RemovedDetected(t *testing.T) {
	baseline := SliceToSeq([]record.WithID[string]{mkRec("u1", nil)})
	cand := SliceToSeq([]record.WithID[string]{})
	got := collect(t, Diff[string](baseline, []RecordSeq[string]{cand}))
	require.Len(t, got, 1)
	assert.Equal(t, "u1", got[0].ID)
	assert.Equal(t, Missing, got[0].Candidates[0].Status)
}

func TestDiff_MultiCandidateParallelIndex(t *testing.T) {
	baseline := SliceToSeq([]record.WithID[string]{mkRec("u1", nil), mkRec("u2", nil)})
	c0 := SliceToSeq([]record.WithID[string]{mkRec("u1", nil)})
	c1 := SliceToSeq([]record.WithID[string]{mkRec("u1", nil), mkRec("u2", nil), mkRec("u3", nil)})
	c2 := SliceToSeq([]record.WithID[string]{})

	got := collect(t, Diff[string](baseline, []RecordSeq[string]{c0, c1, c2}))

	// Expected emissions: u1 (c2 Missing), u2 (c0 Missing, c2 Missing), u3 (c1 Extra)
	require.Len(t, got, 3)
	for _, d := range got {
		assert.Len(t, d.Candidates, 3, "parallel-index invariant: len(Candidates) must match len(candidates)")
	}
	assert.Equal(t, "u1", got[0].ID)
	assert.Equal(t, Matched, got[0].Candidates[0].Status)
	assert.Equal(t, Matched, got[0].Candidates[1].Status)
	assert.Equal(t, Missing, got[0].Candidates[2].Status)

	assert.Equal(t, "u2", got[1].ID)
	assert.Equal(t, Missing, got[1].Candidates[0].Status)
	assert.Equal(t, Matched, got[1].Candidates[1].Status)
	assert.Equal(t, Missing, got[1].Candidates[2].Status)

	assert.Equal(t, "u3", got[2].ID)
	assert.Equal(t, Missing, got[2].Candidates[0].Status) // baseline lacks u3, c0 lacks u3 → both Missing from base perspective
	assert.Equal(t, Extra, got[2].Candidates[1].Status)
	assert.Equal(t, Missing, got[2].Candidates[2].Status)
}

func TestDiff_DuplicateIDRejected(t *testing.T) {
	baseline := SliceToSeq([]record.WithID[string]{mkRec("u1", nil), mkRec("u1", nil)})
	cand := SliceToSeq([]record.WithID[string]{})
	var lastErr error
	for _, err := range Diff[string](baseline, []RecordSeq[string]{cand}) {
		if err != nil {
			lastErr = err
			break
		}
	}
	require.NotNil(t, lastErr)
	assert.True(t, errors.Is(lastErr, ErrDuplicateID))
}

func TestDiff_UnsortedRejected(t *testing.T) {
	baseline := SliceToSeq([]record.WithID[string]{mkRec("a", nil), mkRec("c", nil), mkRec("b", nil)})
	cand := SliceToSeq([]record.WithID[string]{})
	var lastErr error
	for _, err := range Diff[string](baseline, []RecordSeq[string]{cand}) {
		if err != nil {
			lastErr = err
			break
		}
	}
	require.NotNil(t, lastErr)
	assert.True(t, errors.Is(lastErr, ErrUnsortedInput))
}

func TestDiffFunc_NilLess(t *testing.T) {
	baseline := SliceToSeq([]record.WithID[string]{})
	var lastErr error
	for _, err := range DiffFunc[string](baseline, nil, nil) {
		if err != nil {
			lastErr = err
			break
		}
	}
	require.NotNil(t, lastErr)
	assert.True(t, errors.Is(lastErr, ErrInvalidArgument))
}

func TestDiff_DelegatesToDiffFunc(t *testing.T) {
	baseline1 := SliceToSeq([]record.WithID[string]{mkRec("a", nil), mkRec("c", nil)})
	cand1 := SliceToSeq([]record.WithID[string]{mkRec("a", nil), mkRec("b", nil)})

	baseline2 := SliceToSeq([]record.WithID[string]{mkRec("a", nil), mkRec("c", nil)})
	cand2 := SliceToSeq([]record.WithID[string]{mkRec("a", nil), mkRec("b", nil)})

	viaDiff := collect(t, Diff[string](baseline1, []RecordSeq[string]{cand1}))
	viaFunc := collect(t, DiffFunc[string](baseline2, []RecordSeq[string]{cand2}, func(a, b string) bool { return a < b }))

	assert.True(t, reflect.DeepEqual(viaDiff, viaFunc), "Diff and DiffFunc with `<` must produce identical output")
}

func TestDiffFunc_UUIDKeys(t *testing.T) {
	type uuid = [16]byte
	u1 := uuid{0x01}
	u2 := uuid{0x02}
	baseline := SliceToSeq([]record.WithID[uuid]{mkRec(u1, nil)})
	cand := SliceToSeq([]record.WithID[uuid]{mkRec(u2, nil)})

	less := func(a, b uuid) bool { return bytes.Compare(a[:], b[:]) < 0 }
	got := collect(t, DiffFunc[uuid](baseline, []RecordSeq[uuid]{cand}, less))
	require.Len(t, got, 2)
	assert.Equal(t, u1, got[0].ID)
	assert.Equal(t, Missing, got[0].Candidates[0].Status)
	assert.Equal(t, u2, got[1].ID)
	assert.Equal(t, Extra, got[1].Candidates[0].Status)
}

func TestDiff_UpstreamErrorPropagated(t *testing.T) {
	errBoom := errors.New("boom")
	baseline := func(yield func(record.WithID[string], error) bool) {
		if !yield(mkRec("a", nil), nil) { return }
		yield(record.WithID[string]{}, errBoom)
	}
	cand := SliceToSeq([]record.WithID[string]{})

	var lastErr error
	for _, err := range Diff[string](baseline, []RecordSeq[string]{cand}) {
		if err != nil {
			lastErr = err
			break
		}
	}
	require.NotNil(t, lastErr)
	assert.True(t, errors.Is(lastErr, errBoom))
}

// Sanity: id ordering across multi-candidate merge.
func TestDiff_IDOrdering(t *testing.T) {
	baseline := SliceToSeq([]record.WithID[string]{mkRec("u1", nil), mkRec("u4", nil)})
	c0 := SliceToSeq([]record.WithID[string]{mkRec("u2", nil), mkRec("u3", nil)})
	c1 := SliceToSeq([]record.WithID[string]{mkRec("u3", nil), mkRec("u5", nil)})

	got := collect(t, Diff[string](baseline, []RecordSeq[string]{c0, c1}))
	ids := make([]string, len(got))
	for i, d := range got {
		ids[i] = d.ID
	}
	assert.Equal(t, []string{"u1", "u2", "u3", "u4", "u5"}, ids)
}
```

Add the `iter` import to the test file at the top (the linter will catch missing imports).

- [ ] **Step 2: Run tests to verify they fail (compile error — Diff/DiffFunc don't exist)**

```bash
go test ./recordops/ -run 'TestDiff' 2>&1
```

Expected: build failure with `undefined: Diff` / `undefined: DiffFunc`.

- [ ] **Step 3: Implement the K-way merge driver**

Create `recordops/diff.go`. The driver uses a slice of stream cursors (baseline + N candidates) and repeatedly emits an IDDiff for the smallest current ID. For Task 3 the field comparison is delegated to a stub `compareFields` that returns `Matched` if both wrapped values are `reflect.DeepEqual`, `Changed` otherwise — with no Fields populated. The full comparator lands in Task 4.

```go
// specscore: feat-recordops/diff
package recordops

import (
	"cmp"
	"fmt"
	"iter"

	"github.com/dal-go/dalgo/record"
)

// Diff compares baseline against candidates via K-way merge over
// ID-sorted streams and yields one IDDiff per ID where at least one
// candidate diverges (default) or every ID touched by any input
// (with WithIncludeMatched).
//
// Inputs MUST be sorted ascending by ID. Monotonicity is validated
// per stream; violations terminate with ErrUnsortedInput. Duplicate
// IDs within a stream terminate with ErrDuplicateID. Upstream stream
// errors propagate verbatim.
//
// Diff requires K to be cmp.Ordered (string/int/float/etc.). For
// types that are comparable but not orderable (e.g., [16]byte UUIDs),
// use DiffFunc with an explicit less function.
//
// See Package recordops doc for the K-way merge model and memory
// footprint. See spec/features/recordops/diff for the full contract.
//
// Renderers consume the returned stream once; multi-view consumers
// must materialize first (slices.Collect-equivalent).
func Diff[K cmp.Ordered](
	baseline RecordSeq[K],
	candidates []RecordSeq[K],
	opts ...Option,
) iter.Seq2[IDDiff[K], error] {
	return DiffFunc[K](baseline, candidates, func(a, b K) bool { return a < b }, opts...)
}

// DiffFunc is Diff for any K comparable, with caller-supplied strict
// weak order. For UUID-keyed records typed as [16]byte, pass
// bytes.Compare(a[:], b[:]) < 0 as less — see ExampleDiffFunc.
//
// less MUST be a strict weak order (irreflexive, antisymmetric,
// transitive). If less is nil, the returned stream yields exactly
// one (zero, ErrInvalidArgument) and stops.
func DiffFunc[K comparable](
	baseline RecordSeq[K],
	candidates []RecordSeq[K],
	less func(a, b K) bool,
	opts ...Option,
) iter.Seq2[IDDiff[K], error] {
	cfg := resolveOptions(opts...)
	return func(yield func(IDDiff[K], error) bool) {
		var zero IDDiff[K]
		if less == nil {
			yield(zero, fmt.Errorf("recordops.DiffFunc: less must not be nil: %w", ErrInvalidArgument))
			return
		}

		// One cursor per input stream: index 0 = baseline, indexes 1..N = candidates.
		// Each cursor tracks "next" by pulling from the iter.Seq2 lazily.
		baseCur := newCursor(baseline, "baseline")
		candCurs := make([]*streamCursor[K], len(candidates))
		for i, c := range candidates {
			candCurs[i] = newCursor(c, fmt.Sprintf("candidates[%d]", i))
		}

		// Defer stop on all cursors so iter.Seq2 source can clean up.
		defer baseCur.stop()
		defer func() {
			for _, c := range candCurs {
				c.stop()
			}
		}()

		// Prime: pull one record from each stream.
		if err := baseCur.advance(less); err != nil {
			yield(zero, err)
			return
		}
		for _, c := range candCurs {
			if err := c.advance(less); err != nil {
				yield(zero, err)
				return
			}
		}

		// Merge loop: repeatedly find smallest live ID across all cursors.
		for {
			id, anyLive := smallestID(baseCur, candCurs, less)
			if !anyLive {
				return
			}

			diff, err := assembleIDDiff(id, baseCur, candCurs, cfg, less)
			if err != nil {
				yield(zero, err)
				return
			}

			// Advance every cursor that was at this id.
			if baseCur.live && baseCur.id == id {
				if err := baseCur.advance(less); err != nil {
					yield(zero, err)
					return
				}
			}
			for _, c := range candCurs {
				if c.live && c.id == id {
					if err := c.advance(less); err != nil {
						yield(zero, err)
						return
					}
				}
			}

			// Emit iff non-Matched OR WithIncludeMatched.
			if shouldEmit(diff, cfg) {
				if !yield(diff, nil) {
					return
				}
			}
		}
	}
}

// streamCursor wraps an iter.Seq2 with explicit "current id" and
// monotonicity-checking advance. Uses iter.Pull2 for one-step access.
type streamCursor[K comparable] struct {
	name string
	next func() (record.WithID[K], error, bool)
	stop func()
	live bool
	id   K
	rec  record.WithID[K]
	prev K
	hasPrev bool
}

func newCursor[K comparable](s RecordSeq[K], name string) *streamCursor[K] {
	next, stop := iter.Pull2(s)
	return &streamCursor[K]{name: name, next: next, stop: stop, live: true}
}

func (c *streamCursor[K]) advance(less func(a, b K) bool) error {
	r, err, ok := c.next()
	if !ok {
		c.live = false
		return nil
	}
	if err != nil {
		c.live = false
		return err
	}
	if c.hasPrev {
		// Strict ascending: less(prev, r.ID) must hold. If not, either
		// equal (duplicate) or reversed (unsorted).
		if !less(c.prev, r.ID) {
			if c.prev == r.ID {
				return fmt.Errorf("recordops: duplicate ID %v in %s: %w", r.ID, c.name, ErrDuplicateID)
			}
			return fmt.Errorf("recordops: stream out of order at id %v in %s: %w", r.ID, c.name, ErrUnsortedInput)
		}
	}
	c.prev = r.ID
	c.hasPrev = true
	c.id = r.ID
	c.rec = r
	return nil
}

func smallestID[K comparable](
	base *streamCursor[K],
	cands []*streamCursor[K],
	less func(a, b K) bool,
) (K, bool) {
	var smallest K
	any := false
	consider := func(c *streamCursor[K]) {
		if !c.live {
			return
		}
		if !any || less(c.id, smallest) {
			smallest = c.id
			any = true
		}
	}
	consider(base)
	for _, c := range cands {
		consider(c)
	}
	return smallest, any
}

func assembleIDDiff[K comparable](
	id K,
	base *streamCursor[K],
	cands []*streamCursor[K],
	cfg options,
	less func(a, b K) bool,
) (IDDiff[K], error) {
	out := IDDiff[K]{ID: id, Candidates: make([]CandidateState, len(cands))}

	var baseRec *record.WithID[K]
	if base.live && base.id == id {
		baseRec = &base.rec
	}

	for i, c := range cands {
		var candRec *record.WithID[K]
		if c.live && c.id == id {
			candRec = &c.rec
		}
		state, err := classify(baseRec, candRec, cfg)
		if err != nil {
			return IDDiff[K]{}, err
		}
		out.Candidates[i] = state
	}

	// Baseline snapshot. Full population by default; trimmed under WithOnlyChangedFields.
	if baseRec != nil {
		out.Baseline = buildBaselineSnapshot(*baseRec, out.Candidates, cfg)
	}

	return out, nil
}

// classify returns the CandidateState for one (baseline, candidate) pair.
// Task 3 placeholder: produces correct Status but empty Fields when Changed.
// Task 4 wires in compareRecords for field-level deltas.
func classify[K comparable](base, cand *record.WithID[K], cfg options) (CandidateState, error) {
	switch {
	case base == nil && cand == nil:
		// Shouldn't happen for an emitted ID.
		return CandidateState{Status: Matched}, nil
	case base == nil:
		// candidate-only → Extra. Fields populated in Task 4.
		return CandidateState{Status: Extra}, nil
	case cand == nil:
		// baseline-only → Missing.
		return CandidateState{Status: Missing}, nil
	default:
		// Both present. Task 3 stub: Matched if Data() pointers/values are
		// reflect.DeepEqual on Record itself; Changed otherwise. Real per-field
		// comparison lands in Task 4.
		if recordEqualStub(base.Record, cand.Record) {
			return CandidateState{Status: Matched}, nil
		}
		return CandidateState{Status: Changed}, nil
	}
}

func recordEqualStub(a, b any) bool {
	// Stub: cheap surface-level equality. Replaced in Task 4.
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func buildBaselineSnapshot[K comparable](base record.WithID[K], cs []CandidateState, cfg options) *RecordSnapshot {
	// Task 3 placeholder: empty snapshot. Real field extraction in Task 4.
	// WithOnlyChangedFields trimming applied there too.
	return &RecordSnapshot{}
}

func shouldEmit[K comparable](d IDDiff[K], cfg options) bool {
	if cfg.includeMatched {
		return true
	}
	for _, c := range d.Candidates {
		if c.Status != Matched {
			return true
		}
	}
	return false
}
```

(The dup-vs-unsorted check uses `c.prev == r.ID` for the equality test because `K comparable` allows it. This is safe because the previous strict-`less` failure means the two IDs are not less-than in either direction; for a strict weak order that's exactly equality.)

- [ ] **Step 4: Run tests**

```bash
go test ./recordops/ -run 'TestDiff' -v
```

Expected: ALL tests in this task's test file PASS. Field-level assertion tests (`TestDiff_FieldChangeDetected` will be added in Task 4; not present here).

- [ ] **Step 5: Run `go vet`**

```bash
go vet ./recordops/
```

Expected: empty output.

- [ ] **Step 6: Commit**

```bash
git add recordops/diff.go recordops/diff_test.go
git commit -m "feat(recordops): K-way merge driver and Diff/DiffFunc entrypoints

Implements the streaming merge over N+1 sorted input streams.
Detects ErrUnsortedInput and ErrDuplicateID per stream. Yields one
IDDiff per ID with parallel-indexed Candidates (len == len(candidates)).
Default mode skips fully-matched IDs. Field-level deltas and the full
field-comparison logic land in the next task; this commit stops at
record-level Matched/Changed via a placeholder equality check.

Spec: REQ input-streams, diff-entrypoints, emission-rules, id-diff-shape.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 4: Field comparison — compareRecords with Data() extraction

**Files:**
- Create: `recordops/compare.go`
- Create: `recordops/compare_test.go`
- Modify: `recordops/diff.go` (replace the Task 3 stubs)

- [ ] **Step 1: Write failing tests**

Create `recordops/compare_test.go` with tests for: (a) `Data()` extraction (not the interface header), (b) struct vs map kind-mismatch → `_value`, (c) panic recovery, (d) slice → `_value`, (e) field-absence encoding (Value=nil), (f) full baseline snapshot population, (g) struct field-level comparison.

(Test bodies omitted here for plan brevity — implementer writes one assertion per AC under REQ `field-comparison` in the Feature spec, plus one per the field-absence-encoded AC under REQ `id-diff-shape` AC-5 AND AC-5b — the AC-5b test specifically asserts that a real Go-nil candidate value produces `FieldValue{Name, Value:nil, Absent:false}` rather than `Absent:true`, locking in the absent-vs-nil distinction.)

- [ ] **Step 2: Run tests; expect failures (stubs from Task 3 still in place)**

```bash
go test ./recordops/ -run 'TestCompare|TestDiff_FieldChange' 2>&1
```

- [ ] **Step 3: Implement `compare.go` with the 6-step kind-bucket dispatch from REQ `field-comparison`**

Create `recordops/compare.go`:

```go
// specscore: feat-recordops/diff
package recordops

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/dal-go/dalgo/dal"
)

// compareRecords returns the per-field deltas between baseline's Data() and
// candidate's Data(). The returned slice is sorted by Name ascending. A
// candidate field that is absent from the candidate's record (but present in
// baseline) is encoded as FieldValue{Name, Absent: true} — structurally
// distinct from FieldValue{Name, Value: nil, Absent: false} (a real Go-nil
// value the candidate explicitly holds).
//
// On panic during reflect.DeepEqual (e.g., a func/chan field), returns
// (nil, err wrapping ErrIncomparableField).
func compareRecords(baseID any, baseRec, candRec dal.Record, cfg options) (deltas []FieldValue, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recordops: incomparable field in record %v: %v: %w", baseID, r, ErrIncomparableField)
			deltas = nil
		}
	}()

	bv := deref(reflect.ValueOf(baseRec.Data()))
	cv := deref(reflect.ValueOf(candRec.Data()))

	bk, ck := bucket(bv), bucket(cv)
	if bk != ck {
		return []FieldValue{{Name: "_value", Value: candRec.Data()}}, nil
	}

	switch bk {
	case bucketStruct:
		return compareStruct(bv, cv, cfg), nil
	case bucketMap:
		return compareMap(bv, cv, cfg), nil
	default:
		if !reflect.DeepEqual(baseRec.Data(), candRec.Data()) {
			return []FieldValue{{Name: "_value", Value: candRec.Data()}}, nil
		}
		return nil, nil
	}
}

// baselineFields extracts ALL field values from baseRec for the snapshot;
// trimmed to "only fields with at least one candidate delta" if cfg.onlyChangedFields.
func baselineFields(baseRec dal.Record, perCandidateDeltas [][]FieldValue, cfg options) []FieldValue {
	bv := deref(reflect.ValueOf(baseRec.Data()))
	var all []FieldValue
	switch bucket(bv) {
	case bucketStruct:
		t := bv.Type()
		for i := 0; i < bv.NumField(); i++ {
			if !t.Field(i).IsExported() {
				continue
			}
			all = append(all, FieldValue{Name: t.Field(i).Name, Value: bv.Field(i).Interface()})
		}
	case bucketMap:
		keys := bv.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		for _, k := range keys {
			all = append(all, FieldValue{Name: k.String(), Value: bv.MapIndex(k).Interface()})
		}
	default:
		all = []FieldValue{{Name: "_value", Value: baseRec.Data()}}
	}
	if !cfg.onlyChangedFields {
		return all
	}
	// Trim to fields with at least one delta across candidates.
	deltaNames := make(map[string]struct{})
	for _, deltas := range perCandidateDeltas {
		for _, d := range deltas {
			deltaNames[d.Name] = struct{}{}
		}
	}
	if len(deltaNames) == 0 {
		return nil
	}
	trimmed := all[:0]
	for _, fv := range all {
		if _, ok := deltaNames[fv.Name]; ok {
			trimmed = append(trimmed, fv)
		}
	}
	return trimmed
}

type kindBucket int8
const (
	bucketStruct kindBucket = iota
	bucketMap
	bucketOther
)

func bucket(v reflect.Value) kindBucket {
	if !v.IsValid() {
		return bucketOther
	}
	switch v.Kind() {
	case reflect.Struct:
		return bucketStruct
	case reflect.Map:
		if v.Type().Key().Kind() == reflect.String {
			return bucketMap
		}
	}
	return bucketOther
}

func deref(v reflect.Value) reflect.Value {
	if v.IsValid() && v.Kind() == reflect.Pointer && !v.IsNil() {
		return v.Elem()
	}
	return v
}

func compareStruct(bv, cv reflect.Value, cfg options) []FieldValue {
	t := bv.Type()
	var deltas []FieldValue
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if _, ignored := cfg.ignoreFields[f.Name]; ignored {
			continue
		}
		bf := bv.Field(i).Interface()
		cf := cv.Field(i).Interface()
		if !reflect.DeepEqual(bf, cf) {
			deltas = append(deltas, FieldValue{Name: f.Name, Value: cf})
		}
	}
	sort.Slice(deltas, func(i, j int) bool { return deltas[i].Name < deltas[j].Name })
	return deltas
}

func compareMap(bv, cv reflect.Value, cfg options) []FieldValue {
	keys := unionKeys(bv, cv)
	sort.Strings(keys)
	var deltas []FieldValue
	for _, k := range keys {
		if _, ignored := cfg.ignoreFields[k]; ignored {
			continue
		}
		bk := bv.MapIndex(reflect.ValueOf(k))
		ck := cv.MapIndex(reflect.ValueOf(k))
		switch {
		case bk.IsValid() && ck.IsValid():
			if !reflect.DeepEqual(bk.Interface(), ck.Interface()) {
				deltas = append(deltas, FieldValue{Name: k, Value: ck.Interface()})
			}
		case bk.IsValid() && !ck.IsValid():
			// Field present in baseline, absent from candidate.
			// WithAbsentEqualsNil: if baseline value is nil, the two sides
			// are considered equivalent — emit no delta. Otherwise (or when
			// the option is off) emit FieldValue{Absent: true}.
			if cfg.absentEqualsNil && isNilLike(bk) {
				continue
			}
			deltas = append(deltas, FieldValue{Name: k, Absent: true})
		case !bk.IsValid() && ck.IsValid():
			// Field absent from baseline, present in candidate.
			// WithAbsentEqualsNil: if candidate value is nil, equivalent.
			if cfg.absentEqualsNil && isNilLike(ck) {
				continue
			}
			deltas = append(deltas, FieldValue{Name: k, Value: ck.Interface()})
		}
	}
	return deltas
}

// isNilLike reports whether a reflect.Value carries a nil/zero-like value.
// Used by WithAbsentEqualsNil to decide whether an absent-vs-present pair
// counts as equivalent.
func isNilLike(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	iv := v.Interface()
	if iv == nil {
		return true
	}
	rv := reflect.ValueOf(iv)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return rv.IsNil()
	}
	return false
}

func unionKeys(a, b reflect.Value) []string {
	seen := make(map[string]struct{})
	for _, k := range a.MapKeys() {
		seen[k.String()] = struct{}{}
	}
	for _, k := range b.MapKeys() {
		seen[k.String()] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	return out
}
```

- [ ] **Step 4: Replace the Task 3 stubs in `diff.go`**

Update `classify` to call `compareRecords` for the "both present" branch:

```go
func classify[K comparable](base, cand *record.WithID[K], cfg options) (CandidateState, error) {
	switch {
	case base == nil && cand == nil:
		return CandidateState{Status: Matched}, nil
	case base == nil:
		// Extra: emit candidate's full fields as deltas.
		deltas, err := extractFullFields(cand.Record, cfg)
		if err != nil {
			return CandidateState{}, err
		}
		return CandidateState{Status: Extra, Fields: deltas}, nil
	case cand == nil:
		return CandidateState{Status: Missing}, nil
	default:
		deltas, err := compareRecords(base.ID, base.Record, cand.Record, cfg)
		if err != nil {
			return CandidateState{}, err
		}
		if len(deltas) == 0 {
			return CandidateState{Status: Matched}, nil
		}
		return CandidateState{Status: Changed, Fields: deltas}, nil
	}
}
```

Where `extractFullFields` mirrors `baselineFields` but emits the candidate's values for the Extra case (no `cfg.onlyChangedFields` interaction since there are no baseline values to omit).

Replace the `buildBaselineSnapshot` stub similarly to call `baselineFields(base.Record, gatherPerCandidateDeltas(out.Candidates), cfg)`.

- [ ] **Step 5: Run tests**

```bash
go test ./recordops/ -v
```

Expected: all tests so far PASS.

- [ ] **Step 6: Commit**

```bash
git add recordops/compare.go recordops/compare_test.go recordops/diff.go
git commit -m "feat(recordops): field-level comparison with kind-bucket dispatch

Implements compareRecords: extracts via Record.Data(), dereferences
pointers, dispatches by reflect.Kind to struct/map/other buckets.
Mismatched buckets emit a single _value delta. Panic on incomparable
fields recovers and returns ErrIncomparableField. Field absent from
candidate encodes as FieldValue.Absent=true (structurally distinct from
a real Go-nil Value).

Spec: REQ field-comparison, id-diff-shape (field absence, AC-5).

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 5: Wire option semantics

**Files:**
- Modify: `recordops/diff.go` and/or `recordops/compare.go` if needed
- Modify: `recordops/options_test.go` (full semantic tests)

`WithIgnoreFields` is already plumbed through `compareRecords` (Task 4). This task adds semantic tests, wires `WithIncludeMatched` (already done in `shouldEmit`), `WithOnlyChangedFields` (already done in `baselineFields`), and `WithAbsentEqualsNil` (NEW — extend `compareMap` / `compareStruct` to collapse absent-vs-nil when `cfg.absentEqualsNil`). This task is partly wiring + verification.

- [ ] **Step 1: Wire WithAbsentEqualsNil into compareMap / compareStruct**

In `compareMap`, when one side has the key with nil value and the other lacks it, check `cfg.absentEqualsNil`: if true, skip the delta (no entry produced — the candidate ends up Matched on this field); if false, emit `{Name:k, Absent:true}` (when baseline has it, candidate doesn't) or `{Name:k, Value:nil}` (when candidate has it nil-valued and baseline lacks it). The two directions are symmetric — both are collapsed to "no delta" when `absentEqualsNil` is set. Same logic applies in `compareStruct` if/when a field is nil-valued on one side; structs always have every declared field present, so the branch is mostly a no-op for structs (only relevant for pointer-valued struct fields).

- [ ] **Step 2: Add semantic tests for each option and their composition (AC-1 through AC-7 of REQ `options`)**

In particular, add tests for AC-6 (`with-absent-equals-nil-collapses-absent-to-match`) and AC-7 (`with-absent-equals-nil-only-collapses-when-baseline-is-nil`).

- [ ] **Step 3: Run tests; if anything fails, fix in `diff.go` / `compare.go`**

```bash
go test ./recordops/ -v
```

- [ ] **Step 4: Commit**

```bash
git add recordops/options_test.go recordops/compare.go
git commit -m "feat(recordops): WithAbsentEqualsNil + pin all four options semantics

Wires WithAbsentEqualsNil into compareMap (and compareStruct as a no-op
in practice). Adds the seven ACs under REQ options including composition
and the two absent-equals-nil cases. The other three options were
anticipated in compare.go / diff.go in earlier tasks; this commit pins
their behavior with tests.

Spec: REQ options.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 6: RenderYAMLGitStyle (per-candidate, with Absent handling)

**Files:**
- Create: `recordops/render_yaml_gitstyle.go`
- Create: `recordops/render_yaml_gitstyle_test.go`

- [ ] **Step 1: Write the golden test pinning the source-Idea example, plus edge-case tests (empty diff, empty name, absent-field omits-plus-line, stream error)**

The golden output must be byte-equal to:
```
users:
- u1
+ u2:
    first_name: Jack
    gender: male
u3:
-   first_name: Alex
+   first_name: Alexander
```

For the absent-field case (REQ `absent-field-rendering` AC-1): when a Changed candidate's `FieldValue.Absent == true`, emit ONLY the `-` line. Example: baseline `{first_name:"Alex", nickname:"Al"}`, candidate `{first_name:"Alex"}` — output should contain `-   nickname: Al` with NO matching `+   nickname:` line.

- [ ] **Step 2: Implement the renderer**

```go
// specscore: feat-recordops/diff
package recordops

import (
	"fmt"
	"iter"
	"sort"
	"strings"
)

// RenderYAMLGitStyle renders the per-candidate git-diff-style view for
// candidateIndex. See spec/features/recordops/diff REQ render-yaml-git-style.
//
// The stream is consumed once. To produce multiple renderings (e.g., for
// multiple candidates), materialize first via slices.Collect-equivalent and
// re-wrap each copy in a fresh iter.Seq2.
//
// Absent fields (FieldValue.Absent == true on a Changed candidate) emit only
// the `-` line; the `+` line is suppressed entirely — matching git-diff
// semantics for a removed line. See REQ absent-field-rendering.
func RenderYAMLGitStyle[K comparable](
	diffs iter.Seq2[IDDiff[K], error],
	candidateIndex int,
	collectionName string,
) (string, error) {
	var body strings.Builder
	emitted := false

	for d, err := range diffs {
		if err != nil {
			return "", err
		}
		if candidateIndex < 0 || candidateIndex >= len(d.Candidates) {
			continue
		}
		c := d.Candidates[candidateIndex]
		switch c.Status {
		case Matched:
			continue
		case Missing:
			fmt.Fprintf(&body, "- %v\n", d.ID)
			emitted = true
		case Extra:
			fmt.Fprintf(&body, "+ %v:\n", d.ID)
			fields := append([]FieldValue(nil), c.Fields...)
			sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
			for _, f := range fields {
				fmt.Fprintf(&body, "    %s: %v\n", f.Name, f.Value)
			}
			emitted = true
		case Changed:
			fmt.Fprintf(&body, "%v:\n", d.ID)
			fields := append([]FieldValue(nil), c.Fields...)
			sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
			baseLookup := baselineFieldLookup(d.Baseline)
			for _, f := range fields {
				oldVal, hadOld := baseLookup[f.Name]
				if hadOld {
					fmt.Fprintf(&body, "-   %s: %v\n", f.Name, oldVal)
				}
				if !f.Absent {
					fmt.Fprintf(&body, "+   %s: %v\n", f.Name, f.Value)
				}
				// f.Absent == true → suppress the + line entirely (git-diff
				// semantics: the field was removed, only the - line is emitted).
			}
			emitted = true
		}
	}

	if !emitted {
		return fmt.Sprintf("%s: {}\n", collectionName), nil
	}
	var out strings.Builder
	out.WriteString(collectionName)
	out.WriteString(":\n")
	out.WriteString(body.String())
	return out.String(), nil
}

func baselineFieldLookup(b *RecordSnapshot) map[string]any {
	m := make(map[string]any)
	if b == nil {
		return m
	}
	for _, f := range b.Fields {
		m[f.Name] = f.Value
	}
	return m
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./recordops/ -run 'TestRenderYAMLGitStyle' -v
```

Expected: all tests pass, including the byte-equal golden.

- [ ] **Step 4: Commit**

```bash
git add recordops/render_yaml_gitstyle.go recordops/render_yaml_gitstyle_test.go
git commit -m "feat(recordops): RenderYAMLGitStyle per-candidate renderer

Per-candidate git-diff-style YAML emission. Missing renders as '- id',
Extra as '+ id:' block, Changed as id-block with paired '-/+' lines.
When a Changed candidate's FieldValue.Absent == true, only the '-' line
is emitted (git-diff semantics: a removed line is just a deletion). A
real Go-nil candidate Value renders normally as '<nil>'. Empty
collection name produces a malformed but non-panicking first line.

Spec: REQ render-yaml-git-style, absent-field-rendering.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 7: RenderYAMLByID (cross-candidate divergence view)

**Files:**
- Create: `recordops/render_yaml_byid.go` — open with `// specscore: feat-recordops/diff` above the `package recordops` declaration.
- Create: `recordops/render_yaml_byid_test.go` — same specscore annotation.

- [ ] **Step 1: Write tests covering validity, includes-all-candidates, determinism, and absent-flag rendering**

The absent-flag rendering test (REQ `absent-field-rendering` AC-2) requires: given a Changed candidate with `FieldValue{Name:"nickname", Absent:true}`, the YAML output for that field MUST be a nested map containing `absent: true` (e.g., `nickname:\n  absent: true`) — NOT YAML null. A real nil value (Absent:false, Value:nil) MUST render as plain YAML null (`nickname: null` or `nickname: ~`), structurally distinct.

- [ ] **Step 2: Implement using `gopkg.in/yaml.v3` to marshal a deterministic intermediate structure**

The renderer builds an ordered intermediate per ID — baseline (if present) + per-candidate sub-blocks keyed `"0"`, `"1"`, ... — then marshals. For a Changed candidate field, the per-field shape is either:
- `{new: <value>}` for a normal value change (Absent:false), or
- `{absent: true}` for an absent field (Absent:true).

Build the intermediate as `map[string]any` ordered via the yaml.v3 Node API (or via stable key ordering) for determinism. Add `omitempty` / explicit emission to avoid serializing zero-valued sub-fields.

- [ ] **Step 3: Run tests**

- [ ] **Step 4: Commit**

```bash
git add recordops/render_yaml_byid.go recordops/render_yaml_byid_test.go
git commit -m "feat(recordops): RenderYAMLByID cross-candidate divergence view

One YAML block per emitted IDDiff, showing baseline and every
candidate side-by-side keyed by candidate index. Absent-from-candidate
fields render as a nested map { absent: true }, structurally distinct
from a real Go-nil value (which renders as YAML null).

Spec: REQ render-yaml-by-id, absent-field-rendering AC-2.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 8: RenderYAML + RenderJSON (structured renderers)

**Files:**
- Create: `recordops/render_yaml.go` — open with `// specscore: feat-recordops/diff`.
- Create: `recordops/render_json.go` — same specscore annotation.
- Create: `recordops/render_test.go` — same specscore annotation.
- Modify: `go.mod` (add `gopkg.in/yaml.v3`)

- [ ] **Step 1: Add the yaml.v3 dependency**

```bash
go get gopkg.in/yaml.v3
```

- [ ] **Step 2: Write tests: round-trip JSON, parses-as-valid-YAML, determinism for both**

- [ ] **Step 3: Implement both renderers**

Both marshal an intermediate structure `{collectionName: {<id>: <idDiff serialized>}}` using `encoding/json` and `yaml.v3`. Nil values render as native null. Output is deterministic by maintaining ID-sorted order in the intermediate.

- [ ] **Step 4: Run tests**

- [ ] **Step 5: Commit**

```bash
git add recordops/render_yaml.go recordops/render_json.go recordops/render_test.go go.mod go.sum
git commit -m "feat(recordops): RenderYAML and RenderJSON structured renderers

Full structured serialization of the IDDiff stream via gopkg.in/yaml.v3
and encoding/json. Both deterministic; both emit Value=nil as the
language-native null (the ambiguity between nil and field-absent is
documented in package godoc and acceptable for structured outputs).

Spec: REQ render-yaml-and-json.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 9: Package README and example test

**Files:**
- Create: `recordops/README.md` — open with an HTML-comment specscore annotation: `<!-- specscore: feat-recordops/diff -->`.
- Create: `recordops/diff_example_test.go` (package `recordops_test`) — open with `// specscore: feat-recordops/diff`.

- [ ] **Step 1: Write `ExampleDiffFunc` demonstrating UUID-keyed usage with `bytes.Compare`**

```go
// specscore: feat-recordops/diff
package recordops_test

import (
	"bytes"
	"fmt"

	"github.com/dal-go/dalgo/record"
	"github.com/dal-go/dalgo/recordops"
)

func ExampleDiffFunc() {
	type uuid = [16]byte
	a := uuid{0x01}
	b := uuid{0x02}

	baseline := recordops.SliceToSeq([]record.WithID[uuid]{
		record.NewWithID(a, nil, map[string]any{"name": "Alice"}),
	})
	cand := recordops.SliceToSeq([]record.WithID[uuid]{
		record.NewWithID(b, nil, map[string]any{"name": "Bob"}),
	})

	less := func(x, y uuid) bool { return bytes.Compare(x[:], y[:]) < 0 }
	for d, err := range recordops.DiffFunc(baseline, []recordops.RecordSeq[uuid]{cand}, less) {
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		fmt.Printf("id=%x status=%d\n", d.ID, d.Candidates[0].Status)
	}
	// Output:
	// id=01000000000000000000000000000000 status=0
	// id=02000000000000000000000000000000 status=1
}
```

- [ ] **Step 2: Write `recordops/README.md` with one end-to-end example covering both `RenderYAMLGitStyle` and `RenderYAMLByID`**

- [ ] **Step 3: Verify godoc presence**

```bash
go doc github.com/dal-go/dalgo/recordops
go doc github.com/dal-go/dalgo/recordops.Diff
go doc github.com/dal-go/dalgo/recordops.DiffFunc
go doc github.com/dal-go/dalgo/recordops.IDDiff
# ...etc for each exported symbol per REQ godoc
```

Expected: every command returns a non-empty doc block.

- [ ] **Step 4: Run the example test**

```bash
go test ./recordops/ -run Example
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add recordops/README.md recordops/diff_example_test.go
git commit -m "docs(recordops): package README + ExampleDiffFunc

README demonstrates end-to-end usage with both per-candidate and
cross-candidate renderers. ExampleDiffFunc lives in the external
recordops_test package so production recordops doesn't import bytes.

Spec: REQ godoc.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 10: Mark Feature and Plan as Implemented

**Files:**
- Modify: `spec/features/recordops/README.md` (umbrella status)
- Modify: `spec/features/recordops/diff/README.md` (child status)
- Modify: `spec/features/README.md` (index entry)
- Modify: `spec/plans/README.md` (index entry)

- [ ] **Step 1: Run the full test suite once more**

```bash
go test ./recordops/...
```

Expected: all tests pass.

- [ ] **Step 2: Run `specscore spec lint`**

```bash
specscore spec lint
```

Expected: `0 violations found`.

- [ ] **Step 3: Update statuses**

- `spec/features/recordops/README.md`: `**Status:** Approved` → `**Status:** Implemented`
- `spec/features/recordops/diff/README.md`: `**Status:** Approved` → `**Status:** Implemented`
- `spec/features/README.md`: index row's status column: `Approved` → `Implemented`
- `spec/plans/README.md`: this plan's status: `Ready` → `Done` (and add a row if not already present)

- [ ] **Step 4: Re-lint**

```bash
specscore spec lint
```

Expected: `0 violations found`.

- [ ] **Step 5: Commit and push**

```bash
git add spec/
git commit -m "docs(spec): mark recordops/diff Feature as Implemented

All thirteen REQs satisfied with passing tests:
- input-streams, id-diff-shape, emission-rules
- diff-entrypoints, field-comparison, options
- bridge-helpers, renderer-stream-consumption
- absent-field-rendering
- render-yaml-git-style, render-yaml-by-id, render-yaml-and-json
- godoc

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
git push
```

---

## Verification Checklist (after all tasks complete)

- [ ] `go build ./...` succeeds with no output
- [ ] `go test ./recordops/...` succeeds, all tests passing
- [ ] `go test ./...` succeeds (no regressions elsewhere)
- [ ] `go vet ./recordops/` returns empty output
- [ ] `specscore spec lint` returns `0 violations found`
- [ ] `go doc github.com/dal-go/dalgo/recordops` returns substantive package doc
- [ ] Every exported symbol listed in REQ `godoc` has a non-empty doc block
- [ ] `git log --oneline -- recordops/` shows ~9 atomic commits (one per task that produced code)
- [ ] Feature READMEs show `Status: Implemented`
- [ ] Plan index shows `Status: Done`

## Out of Scope (will not be done here)

- Migrating `dal.RecordsReader` itself to `iter.Seq2` — separate Idea (`spec/ideas/dal-records-reader-iter-seq.md`).
- `DiffMap[K, V]` entrypoint — deferred.
- Schema-aware diffing — belongs with `dalgo-schema-modification`.
- CLI tool / patch application / streaming-renderer to `io.Writer` / per-index slice-field diffing.
- Surfacing `dal.Reader.Cursor()` through `ReaderToSeq`.

## CHANGELOG Entry (draft for the maintainer)

```markdown
### New package: recordops

- New sub-package `github.com/dal-go/dalgo/recordops` provides streaming
  record-level diff between one baseline recordset and N candidates,
  via K-way merge over ID-sorted `iter.Seq2` inputs.
- Two entrypoints (`Diff` for `cmp.Ordered` keys; `DiffFunc` for
  `comparable` with explicit `less`) — mirrors `slices.Sort` /
  `slices.SortFunc`.
- Four orthogonal options: `WithIgnoreFields`, `WithIncludeMatched`,
  `WithOnlyChangedFields`, `WithAbsentEqualsNil`.
- Four renderers: per-candidate git-diff style, cross-candidate
  divergence view, plain YAML, JSON.
- Two bridge helpers: `SliceToSeq` (in-memory slice → stream),
  `ReaderToSeq` (`dal.RecordsReader` → stream).
- One new dependency: `gopkg.in/yaml.v3`.
- No changes to any existing dalgo package.
```
