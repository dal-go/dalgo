// specscore: feat-recordops/diff
package recordops

import (
	"errors"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testUser struct {
	FirstName string
	LastName  string
	Age       int
}

// mkDataRec wraps an arbitrary Data() value in a dal.Record. Data() requires
// SetError(nil) to be invoked so the underlying record doesn't panic.
func mkDataRec(t *testing.T, data any) dal.Record {
	t.Helper()
	key := dal.NewKeyWithID("Things", "k")
	rec := dal.NewRecordWithData(key, data)
	rec.SetError(nil)
	return rec
}

// TestCompare_DataExtractedViaRecordData pins REQ field-comparison AC-1: the
// comparator reads via Data(), not via the dal.Record interface header. Two
// records with the same Data() compare Matched even though the wrappers are
// distinct *record values.
func TestCompare_DataExtractedViaRecordData(t *testing.T) {
	a := mkDataRec(t, testUser{FirstName: "Alex", LastName: "T", Age: 30})
	b := mkDataRec(t, testUser{FirstName: "Alex", LastName: "T", Age: 30})
	deltas, err := compareRecords("u1", a, b, options{})
	require.NoError(t, err)
	assert.Empty(t, deltas)
}

// TestCompare_StructFieldDelta — REQ id-diff-shape AC-4: changed fields carry
// only deltas; the candidate Fields contains exactly the differing field.
func TestCompare_StructFieldDelta(t *testing.T) {
	base := mkDataRec(t, testUser{FirstName: "Alex", LastName: "T", Age: 30})
	cand := mkDataRec(t, testUser{FirstName: "Alex", LastName: "T", Age: 31})
	deltas, err := compareRecords("u1", base, cand, options{})
	require.NoError(t, err)
	require.Len(t, deltas, 1)
	assert.Equal(t, "Age", deltas[0].Name)
	assert.Equal(t, 31, deltas[0].Value)
	assert.False(t, deltas[0].Absent)
}

// TestCompare_StructSortedByName — deltas sorted by Name ascending.
func TestCompare_StructSortedByName(t *testing.T) {
	base := mkDataRec(t, testUser{FirstName: "Alex", LastName: "T", Age: 30})
	cand := mkDataRec(t, testUser{FirstName: "Sam", LastName: "K", Age: 31})
	deltas, err := compareRecords("u1", base, cand, options{})
	require.NoError(t, err)
	require.Len(t, deltas, 3)
	assert.Equal(t, "Age", deltas[0].Name)
	assert.Equal(t, "FirstName", deltas[1].Name)
	assert.Equal(t, "LastName", deltas[2].Name)
}

// TestCompare_StructIgnoreFields — REQ options: WithIgnoreFields drops named
// fields from struct comparison.
func TestCompare_StructIgnoreFields(t *testing.T) {
	base := mkDataRec(t, testUser{FirstName: "Alex", LastName: "T", Age: 30})
	cand := mkDataRec(t, testUser{FirstName: "Alex", LastName: "T", Age: 31})
	cfg := resolveOptions(WithIgnoreFields("Age"))
	deltas, err := compareRecords("u1", base, cand, cfg)
	require.NoError(t, err)
	assert.Empty(t, deltas)
}

// TestCompare_MapFieldDelta — both sides map[string]any with one differing key.
func TestCompare_MapFieldDelta(t *testing.T) {
	base := mkDataRec(t, map[string]any{"first_name": "Alex", "last_name": "T"})
	cand := mkDataRec(t, map[string]any{"first_name": "Sam", "last_name": "T"})
	deltas, err := compareRecords("u1", base, cand, options{})
	require.NoError(t, err)
	require.Len(t, deltas, 1)
	assert.Equal(t, "first_name", deltas[0].Name)
	assert.Equal(t, "Sam", deltas[0].Value)
	assert.False(t, deltas[0].Absent)
}

// TestCompare_FieldAbsentInCandidate — REQ id-diff-shape AC-5: a field present
// in baseline but absent from the candidate's map encodes as
// FieldValue{Name, Absent: true} with zero Value.
func TestCompare_FieldAbsentInCandidate(t *testing.T) {
	base := mkDataRec(t, map[string]any{"first_name": "Alex", "nickname": "Al"})
	cand := mkDataRec(t, map[string]any{"first_name": "Alex"})
	deltas, err := compareRecords("u1", base, cand, options{})
	require.NoError(t, err)
	require.Len(t, deltas, 1)
	assert.Equal(t, "nickname", deltas[0].Name)
	assert.True(t, deltas[0].Absent)
	assert.Nil(t, deltas[0].Value)
}

// TestCompare_NilValuedFieldDistinctFromAbsent — REQ id-diff-shape AC-5b: a
// real Go-nil value held by the candidate encodes as Value:nil, Absent:false —
// distinct from the absent-from-record case above.
func TestCompare_NilValuedFieldDistinctFromAbsent(t *testing.T) {
	base := mkDataRec(t, map[string]any{"first_name": "Alex"})
	cand := mkDataRec(t, map[string]any{"first_name": nil})
	deltas, err := compareRecords("u1", base, cand, options{})
	require.NoError(t, err)
	require.Len(t, deltas, 1)
	assert.Equal(t, "first_name", deltas[0].Name)
	assert.False(t, deltas[0].Absent, "explicit nil value must NOT be encoded as Absent")
	assert.Nil(t, deltas[0].Value)
}

// TestCompare_FieldOnlyInCandidate — candidate has a key baseline lacks.
func TestCompare_FieldOnlyInCandidate(t *testing.T) {
	base := mkDataRec(t, map[string]any{"first_name": "Alex"})
	cand := mkDataRec(t, map[string]any{"first_name": "Alex", "nickname": "Al"})
	deltas, err := compareRecords("u1", base, cand, options{})
	require.NoError(t, err)
	require.Len(t, deltas, 1)
	assert.Equal(t, "nickname", deltas[0].Name)
	assert.Equal(t, "Al", deltas[0].Value)
	assert.False(t, deltas[0].Absent)
}

// TestCompare_MapIgnoreFields — WithIgnoreFields applies to map keys.
func TestCompare_MapIgnoreFields(t *testing.T) {
	base := mkDataRec(t, map[string]any{"first_name": "Alex", "updated_at": "t0"})
	cand := mkDataRec(t, map[string]any{"first_name": "Alex", "updated_at": "t1"})
	cfg := resolveOptions(WithIgnoreFields("updated_at"))
	deltas, err := compareRecords("u1", base, cand, cfg)
	require.NoError(t, err)
	assert.Empty(t, deltas)
}

// TestCompare_KindMismatch — REQ field-comparison AC-2: struct vs map emits
// exactly one _value delta carrying the candidate's full Data().
func TestCompare_KindMismatch(t *testing.T) {
	base := mkDataRec(t, testUser{FirstName: "Alex"})
	cand := mkDataRec(t, map[string]any{"first_name": "Alex"})
	deltas, err := compareRecords("u1", base, cand, options{})
	require.NoError(t, err)
	require.Len(t, deltas, 1)
	assert.Equal(t, "_value", deltas[0].Name)
	assert.Equal(t, map[string]any{"first_name": "Alex"}, deltas[0].Value)
}

// TestCompare_IncomparablePanicRecovered — REQ field-comparison AC-3: panics
// during comparison are recovered and surfaced as ErrIncomparableField.
func TestCompare_IncomparablePanicRecovered(t *testing.T) {
	base := mkDataRec(t, testUser{FirstName: "Alex"})
	cand := panicRecord{}
	deltas, err := compareRecords("u1", base, cand, options{})
	require.Error(t, err)
	assert.Nil(t, deltas)
	assert.True(t, errors.Is(err, ErrIncomparableField), "want ErrIncomparableField, got %v", err)
}

// panicRecord.Data() panics — exercises the compareRecords recover path.
type panicRecord struct{}

func (panicRecord) Key() *dal.Key             { return nil }
func (panicRecord) Data() any                 { panic("boom") }
func (panicRecord) Error() error              { return nil }
func (panicRecord) SetError(error) dal.Record { return panicRecord{} }
func (panicRecord) Exists() bool              { return true }
func (panicRecord) HasChanged() bool          { return false }
func (panicRecord) MarkAsChanged()            {}

// TestCompare_SliceDataEmitsValue — REQ field-comparison AC-4: slice Data()
// emits one _value delta when the slices differ.
func TestCompare_SliceDataEmitsValue(t *testing.T) {
	base := mkDataRec(t, []int{1, 2, 3})
	cand := mkDataRec(t, []int{1, 2, 4})
	deltas, err := compareRecords("u1", base, cand, options{})
	require.NoError(t, err)
	require.Len(t, deltas, 1)
	assert.Equal(t, "_value", deltas[0].Name)
	assert.Equal(t, []int{1, 2, 4}, deltas[0].Value)
}

// TestCompare_EqualSlicesNoDelta — equal "other"-bucket values produce no delta.
func TestCompare_EqualSlicesNoDelta(t *testing.T) {
	base := mkDataRec(t, []int{1, 2, 3})
	cand := mkDataRec(t, []int{1, 2, 3})
	deltas, err := compareRecords("u1", base, cand, options{})
	require.NoError(t, err)
	assert.Empty(t, deltas)
}

// TestCompare_PointerToStructDereferenced — REQ id-diff-shape AC-6/REQ field-
// comparison: a *struct is dereferenced before kind dispatch.
func TestCompare_PointerToStructDereferenced(t *testing.T) {
	base := mkDataRec(t, &testUser{FirstName: "Alex", LastName: "T", Age: 30})
	cand := mkDataRec(t, &testUser{FirstName: "Alex", LastName: "T", Age: 31})
	deltas, err := compareRecords("u1", base, cand, options{})
	require.NoError(t, err)
	require.Len(t, deltas, 1)
	assert.Equal(t, "Age", deltas[0].Name)
	assert.Equal(t, 31, deltas[0].Value)
}

// TestBaselineFields_FullByDefault — buildBaselineSnapshot populates the
// full field list by default.
func TestBaselineFields_FullByDefault(t *testing.T) {
	base := mkDataRec(t, testUser{FirstName: "Alex", LastName: "T", Age: 30})
	got := baselineFields(base, nil, options{})
	require.Len(t, got, 3)
	names := []string{got[0].Name, got[1].Name, got[2].Name}
	assert.Equal(t, []string{"FirstName", "LastName", "Age"}, names)
}

// TestBaselineFields_OnlyChangedTrim — REQ options WithOnlyChangedFields:
// trim baseline snapshot to only fields with a delta on at least one candidate.
func TestBaselineFields_OnlyChangedTrim(t *testing.T) {
	base := mkDataRec(t, testUser{FirstName: "Alex", LastName: "T", Age: 30})
	deltas := [][]FieldValue{{{Name: "Age", Value: 31}}}
	cfg := resolveOptions(WithOnlyChangedFields())
	got := baselineFields(base, deltas, cfg)
	require.Len(t, got, 1)
	assert.Equal(t, "Age", got[0].Name)
	assert.Equal(t, 30, got[0].Value)
}

// TestBaselineFields_OnlyChangedAllMatched — REQ options: if no candidate has
// any delta, OnlyChangedFields collapses the baseline to nil.
func TestBaselineFields_OnlyChangedAllMatched(t *testing.T) {
	base := mkDataRec(t, testUser{FirstName: "Alex"})
	cfg := resolveOptions(WithOnlyChangedFields())
	got := baselineFields(base, nil, cfg)
	assert.Nil(t, got)
}

// TestBaselineFields_OtherBucketValue — non-struct/non-map baseline collapses
// to a single _value FieldValue (REQ id-diff-shape AC-6).
func TestBaselineFields_OtherBucketValue(t *testing.T) {
	base := mkDataRec(t, []int{1, 2, 3})
	got := baselineFields(base, nil, options{})
	require.Len(t, got, 1)
	assert.Equal(t, "_value", got[0].Name)
	assert.Equal(t, []int{1, 2, 3}, got[0].Value)
}
