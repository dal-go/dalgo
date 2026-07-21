// specscore: feat-recordops/diff
package recordops

import (
	"iter"

	"github.com/dal-go/record"
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
