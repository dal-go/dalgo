// specscore: feat-recordops/diff
package recordops

import (
	"errors"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Smoke tests: every exported type/sentinel exists and has the expected zero value.
// Semantic tests for options live below.

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

// mkRecData builds a record.WithID[string] with arbitrary Data() value. Used
// by option semantic tests that need to vary Data() between baseline and
// candidate (mkRec from diff_test.go derives Data() deterministically from id).
func mkRecData(id string, data any) record.WithID[string] {
	key := dal.NewKeyWithID("Things", id)
	rec := dal.NewRecordWithData(key, data)
	rec.SetError(nil)
	return record.WithID[string]{ID: id, Record: rec}
}

// runDiff is a small helper that drives Diff[string] and collects emissions.
func runDiff(t *testing.T, baseline []record.WithID[string], cands [][]record.WithID[string], opts ...Option) []IDDiff[string] {
	t.Helper()
	bSeq := SliceToSeq(baseline)
	cSeqs := make([]RecordSeq[string], len(cands))
	for i, c := range cands {
		cSeqs[i] = SliceToSeq(c)
	}
	var out []IDDiff[string]
	for d, err := range Diff[string](bSeq, cSeqs, opts...) {
		require.NoError(t, err)
		out = append(out, d)
	}
	return out
}

// AC-1 (with-ignore-fields-skips-comparison): baseline and candidate differ
// only on UpdatedAt; WithIgnoreFields("UpdatedAt") collapses the diff to no
// emissions in default (skip-matched) mode.
func TestOptions_WithIgnoreFields_SkipsComparison(t *testing.T) {
	base := []record.WithID[string]{
		mkRecData("u1", map[string]any{"FirstName": "Alex", "UpdatedAt": "t1"}),
	}
	cand := []record.WithID[string]{
		mkRecData("u1", map[string]any{"FirstName": "Alex", "UpdatedAt": "t2"}),
	}
	got := runDiff(t, base, [][]record.WithID[string]{cand}, WithIgnoreFields("UpdatedAt"))
	assert.Empty(t, got, "no IDDiff expected when only differing field is ignored")
}

// AC-2 (with-include-matched-emits-matched): identical baseline and candidate,
// with WithIncludeMatched — every baseline ID emits an IDDiff with
// Candidates[0].Status == Matched, Candidates[0].Fields == nil, Baseline.Fields populated.
func TestOptions_WithIncludeMatched_EmitsMatched(t *testing.T) {
	base := []record.WithID[string]{
		mkRecData("u1", map[string]any{"FirstName": "Alex"}),
		mkRecData("u2", map[string]any{"FirstName": "Sam"}),
	}
	cand := []record.WithID[string]{
		mkRecData("u1", map[string]any{"FirstName": "Alex"}),
		mkRecData("u2", map[string]any{"FirstName": "Sam"}),
	}
	got := runDiff(t, base, [][]record.WithID[string]{cand}, WithIncludeMatched())
	require.Len(t, got, 2)
	for _, d := range got {
		require.Len(t, d.Candidates, 1)
		assert.Equal(t, Matched, d.Candidates[0].Status)
		assert.Nil(t, d.Candidates[0].Fields)
		require.NotNil(t, d.Baseline)
		assert.NotEmpty(t, d.Baseline.Fields)
	}
}

// AC-3 (with-only-changed-fields-trims-baseline): baseline {A:1,B:2,C:3} vs
// candidate {A:1,B:9,C:3}; WithOnlyChangedFields trims Baseline.Fields to only B.
// Candidate Fields still contains only the delta {B:9}.
func TestOptions_WithOnlyChangedFields_TrimsBaseline(t *testing.T) {
	base := []record.WithID[string]{
		mkRecData("u1", map[string]any{"A": 1, "B": 2, "C": 3}),
	}
	cand := []record.WithID[string]{
		mkRecData("u1", map[string]any{"A": 1, "B": 9, "C": 3}),
	}
	got := runDiff(t, base, [][]record.WithID[string]{cand}, WithOnlyChangedFields())
	require.Len(t, got, 1)
	d := got[0]
	require.NotNil(t, d.Baseline)
	require.Len(t, d.Baseline.Fields, 1, "Baseline.Fields trimmed to changed-only")
	assert.Equal(t, "B", d.Baseline.Fields[0].Name)
	assert.Equal(t, 2, d.Baseline.Fields[0].Value)
	require.Len(t, d.Candidates, 1)
	require.Len(t, d.Candidates[0].Fields, 1)
	assert.Equal(t, "B", d.Candidates[0].Fields[0].Name)
	assert.Equal(t, 9, d.Candidates[0].Fields[0].Value)
}

// AC-4 (full-baseline-by-default): same inputs as AC-3 with no option — Baseline.Fields
// contains all three fields in stable name-sorted order; Candidates[0].Fields contains
// only the B delta.
func TestOptions_FullBaselineByDefault(t *testing.T) {
	base := []record.WithID[string]{
		mkRecData("u1", map[string]any{"A": 1, "B": 2, "C": 3}),
	}
	cand := []record.WithID[string]{
		mkRecData("u1", map[string]any{"A": 1, "B": 9, "C": 3}),
	}
	got := runDiff(t, base, [][]record.WithID[string]{cand})
	require.Len(t, got, 1)
	d := got[0]
	require.NotNil(t, d.Baseline)
	require.Len(t, d.Baseline.Fields, 3)
	names := []string{d.Baseline.Fields[0].Name, d.Baseline.Fields[1].Name, d.Baseline.Fields[2].Name}
	assert.Equal(t, []string{"A", "B", "C"}, names, "full baseline must be name-sorted ascending")
	require.Len(t, d.Candidates, 1)
	require.Len(t, d.Candidates[0].Fields, 1)
	assert.Equal(t, "B", d.Candidates[0].Fields[0].Name)
	assert.Equal(t, 9, d.Candidates[0].Fields[0].Value)
}

// AC-5 (options-compose): baseline + candidate differ only on UpdatedAt. With
// WithIgnoreFields("UpdatedAt") + WithIncludeMatched + WithOnlyChangedFields combined,
// every ID emits with Status==Matched, Baseline.Fields nil (no changes after ignore),
// Candidates[0].Fields nil.
func TestOptions_Compose(t *testing.T) {
	base := []record.WithID[string]{
		mkRecData("u1", map[string]any{"FirstName": "Alex", "UpdatedAt": "t1"}),
		mkRecData("u2", map[string]any{"FirstName": "Sam", "UpdatedAt": "t1"}),
	}
	cand := []record.WithID[string]{
		mkRecData("u1", map[string]any{"FirstName": "Alex", "UpdatedAt": "t2"}),
		mkRecData("u2", map[string]any{"FirstName": "Sam", "UpdatedAt": "t9"}),
	}
	got := runDiff(t, base, [][]record.WithID[string]{cand},
		WithIgnoreFields("UpdatedAt"),
		WithIncludeMatched(),
		WithOnlyChangedFields(),
	)
	require.Len(t, got, 2)
	for _, d := range got {
		require.Len(t, d.Candidates, 1)
		assert.Equal(t, Matched, d.Candidates[0].Status)
		assert.Nil(t, d.Candidates[0].Fields)
		require.NotNil(t, d.Baseline)
		assert.Nil(t, d.Baseline.Fields, "OnlyChanged + nothing changed (after ignore) -> nil")
	}
}

// AC-6 (with-absent-equals-nil-collapses-absent-to-match): baseline {nickname:nil}
// (value IS nil) and candidate {} (key absent) — default mode skips (fully matched).
// With WithIncludeMatched + WithAbsentEqualsNil, the emit shows Status == Matched.
func TestOptions_WithAbsentEqualsNil_BaselineNilCandidateAbsent(t *testing.T) {
	base := []record.WithID[string]{
		mkRecData("u1", map[string]any{"nickname": nil}),
	}
	cand := []record.WithID[string]{
		mkRecData("u1", map[string]any{}),
	}
	got := runDiff(t, base, [][]record.WithID[string]{cand},
		WithIncludeMatched(),
		WithAbsentEqualsNil(),
	)
	require.Len(t, got, 1)
	d := got[0]
	require.Len(t, d.Candidates, 1)
	assert.Equal(t, Matched, d.Candidates[0].Status)
	assert.Nil(t, d.Candidates[0].Fields)
}

// Symmetric case: baseline lacks the key, candidate has nil for it. Should also collapse.
func TestOptions_WithAbsentEqualsNil_BaselineAbsentCandidateNil(t *testing.T) {
	base := []record.WithID[string]{
		mkRecData("u1", map[string]any{}),
	}
	cand := []record.WithID[string]{
		mkRecData("u1", map[string]any{"nickname": nil}),
	}
	got := runDiff(t, base, [][]record.WithID[string]{cand},
		WithIncludeMatched(),
		WithAbsentEqualsNil(),
	)
	require.Len(t, got, 1)
	d := got[0]
	require.Len(t, d.Candidates, 1)
	assert.Equal(t, Matched, d.Candidates[0].Status)
	assert.Nil(t, d.Candidates[0].Fields)
}

// AC-7 (with-absent-equals-nil-only-collapses-when-baseline-is-nil): baseline
// {nickname:"Al"} (non-nil value) and candidate {} — even with WithAbsentEqualsNil,
// this is still a Changed delta with Candidates[0].Fields == [{Name:"nickname", Absent:true}].
func TestOptions_WithAbsentEqualsNil_NonNilBaselineStaysChanged(t *testing.T) {
	base := []record.WithID[string]{
		mkRecData("u1", map[string]any{"nickname": "Al"}),
	}
	cand := []record.WithID[string]{
		mkRecData("u1", map[string]any{}),
	}
	got := runDiff(t, base, [][]record.WithID[string]{cand}, WithAbsentEqualsNil())
	require.Len(t, got, 1)
	d := got[0]
	require.Len(t, d.Candidates, 1)
	assert.Equal(t, Changed, d.Candidates[0].Status)
	require.Len(t, d.Candidates[0].Fields, 1)
	assert.Equal(t, "nickname", d.Candidates[0].Fields[0].Name)
	assert.True(t, d.Candidates[0].Fields[0].Absent)
}

// Symmetric: baseline lacks key, candidate has non-nil value. Even with AbsentEqualsNil,
// this is a Changed delta (the candidate's value is real, not nil-like).
func TestOptions_WithAbsentEqualsNil_NonNilCandidateStaysChanged(t *testing.T) {
	base := []record.WithID[string]{
		mkRecData("u1", map[string]any{}),
	}
	cand := []record.WithID[string]{
		mkRecData("u1", map[string]any{"nickname": "Al"}),
	}
	got := runDiff(t, base, [][]record.WithID[string]{cand}, WithAbsentEqualsNil())
	require.Len(t, got, 1)
	d := got[0]
	require.Len(t, d.Candidates, 1)
	assert.Equal(t, Changed, d.Candidates[0].Status)
	require.Len(t, d.Candidates[0].Fields, 1)
	assert.Equal(t, "nickname", d.Candidates[0].Fields[0].Name)
	assert.False(t, d.Candidates[0].Fields[0].Absent)
	assert.Equal(t, "Al", d.Candidates[0].Fields[0].Value)
}

