// specscore: feat-recordops/diff
package recordops

import (
	"errors"
	"io"
	"iter"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failMarshal implements yaml.Marshaler returning an error so yaml.Marshal
// returns an error (rather than panicking, which yaml.v3 only recovers for
// its own yamlError type).
type failMarshal struct{}

func (failMarshal) MarshalYAML() (any, error) {
	return nil, errors.New("yaml marshal boom")
}

// errIDDiffStream yields one (zero, err) pair — used to exercise renderer
// stream-error branches without depending on the diff pipeline.
func errIDDiffStream(err error) iter.Seq2[IDDiff[string], error] {
	return func(yield func(IDDiff[string], error) bool) {
		yield(IDDiff[string]{}, err)
	}
}

// singleIDDiffStream yields a single (d, nil) pair — used to inject
// manually-constructed IDDiff values into renderer tests.
func singleIDDiffStream(d IDDiff[string]) iter.Seq2[IDDiff[string], error] {
	return func(yield func(IDDiff[string], error) bool) {
		yield(d, nil)
	}
}

// TestIsNilLike covers every branch of isNilLike. Exercised directly because
// the public API only reaches a few of these cases organically.
func TestIsNilLike(t *testing.T) {
	var nilPtr *int
	var nilMap map[string]int
	var nilSlice []int
	var nilChan chan int
	var nilFunc func()
	x := 7

	tests := []struct {
		name string
		v    reflect.Value
		want bool
	}{
		{"invalid", reflect.Value{}, true},
		{"untyped-nil", reflect.ValueOf(any(nil)), true},
		{"typed-nil-ptr", reflect.ValueOf(nilPtr), true},
		{"typed-nil-map", reflect.ValueOf(nilMap), true},
		{"typed-nil-slice", reflect.ValueOf(nilSlice), true},
		{"typed-nil-chan", reflect.ValueOf(nilChan), true},
		{"typed-nil-func", reflect.ValueOf(nilFunc), true},
		{"zero-int", reflect.ValueOf(0), false},
		{"empty-string", reflect.ValueOf(""), false},
		{"false", reflect.ValueOf(false), false},
		{"non-nil-ptr", reflect.ValueOf(&x), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isNilLike(tc.v))
		})
	}
}

// TestBucket_InvalidValue covers bucket()'s invalid-Value early return. A
// record whose Data() returns untyped nil produces an invalid reflect.Value
// after deref.
func TestBucket_InvalidValue(t *testing.T) {
	assert.Equal(t, bucketOther, bucket(reflect.Value{}))
}

// structWithHidden has both an exported and an unexported field. Exercises
// extractAllFields' and compareStruct's "skip unexported" branches.
type structWithHidden struct {
	Name   string
	secret string //nolint:unused // intentional unexported field for coverage
}

// TestExtractAllFields_SkipsUnexported reaches compare.go:89-90 — the
// unexported-field skip in extractAllFields' struct branch.
func TestExtractAllFields_SkipsUnexported(t *testing.T) {
	base := mkDataRec(t, structWithHidden{Name: "Alex", secret: "x"})
	got := baselineFields(base, nil, options{})
	require.Len(t, got, 1, "unexported field must be skipped")
	assert.Equal(t, "Name", got[0].Name)
}

// TestCompareStruct_SkipsUnexported reaches compare.go:147-148 — the
// unexported-field skip in compareStruct.
func TestCompareStruct_SkipsUnexported(t *testing.T) {
	base := mkDataRec(t, structWithHidden{Name: "Alex", secret: "x"})
	cand := mkDataRec(t, structWithHidden{Name: "Alex", secret: "y"})
	deltas, err := compareRecords("u1", base, cand, options{})
	require.NoError(t, err)
	assert.Empty(t, deltas, "differing unexported field must not produce a delta")
}

// idOfErrReader is a fakeReader variant where idOf will fail downstream.
// (It only needs to yield a record; idOf is supplied by the caller.)

// TestReaderToSeq_IDOfError reaches bridge.go:49-52 — idOf returns an error.
func TestReaderToSeq_IDOfError(t *testing.T) {
	rec1 := dal.NewRecordWithData(dal.NewKeyWithID("Things", "a"), &struct{}{})
	r := &fakeReader{records: []dal.Record{rec1}}
	wantErr := errors.New("idof boom")
	idOf := func(_ dal.Record) (string, error) { return "", wantErr }

	var firstErr error
	for _, err := range ReaderToSeq[string](r, idOf) {
		if err != nil {
			firstErr = err
			break
		}
	}
	require.Error(t, firstErr)
	assert.True(t, errors.Is(firstErr, wantErr), "got %v", firstErr)
	assert.Equal(t, 1, r.closeCnt, "Close() must run even on idOf error")
}

// errSeq builds a RecordSeq that immediately yields (zero, err).
func errSeq[K comparable](err error) RecordSeq[K] {
	return func(yield func(record.WithID[K], error) bool) {
		var zero record.WithID[K]
		yield(zero, err)
	}
}

// TestDiffFunc_BaselinePrimingError reaches diff.go:76-78 — baseline's first
// advance returns an error.
func TestDiffFunc_BaselinePrimingError(t *testing.T) {
	wantErr := errors.New("prime baseline boom")
	baseline := errSeq[string](wantErr)
	cand := SliceToSeq([]record.WithID[string]{})

	var lastErr error
	for _, err := range Diff[string](baseline, []RecordSeq[string]{cand}) {
		if err != nil {
			lastErr = err
			break
		}
	}
	require.Error(t, lastErr)
	assert.True(t, errors.Is(lastErr, wantErr))
}

// TestDiffFunc_CandidatePrimingError reaches diff.go:81-84 — a candidate's
// first advance returns an error.
func TestDiffFunc_CandidatePrimingError(t *testing.T) {
	wantErr := errors.New("prime candidate boom")
	baseline := SliceToSeq([]record.WithID[string]{})
	cand := errSeq[string](wantErr)

	var lastErr error
	for _, err := range Diff[string](baseline, []RecordSeq[string]{cand}) {
		if err != nil {
			lastErr = err
			break
		}
	}
	require.Error(t, lastErr)
	assert.True(t, errors.Is(lastErr, wantErr))
}

// panicRecRec wraps panicRecord into a record.WithID[string] so it can flow
// through Diff. classify will call compareRecords which recovers the panic and
// returns ErrIncomparableField — exercising diff.go:95-98 and 213-215 and
// 240-242 (classify's compareRecords-error branch).
func TestDiffFunc_AssembleErrorFromCompareRecover(t *testing.T) {
	baseline := SliceToSeq([]record.WithID[string]{mkRec("u1")})
	cand := func(yield func(record.WithID[string], error) bool) {
		yield(record.WithID[string]{ID: "u1", Record: panicRecord{}}, nil)
	}

	var lastErr error
	for _, err := range Diff[string](baseline, []RecordSeq[string]{cand}) {
		if err != nil {
			lastErr = err
			break
		}
	}
	require.Error(t, lastErr)
	assert.True(t, errors.Is(lastErr, ErrIncomparableField))
}

// TestDiffFunc_CandidateAdvanceErrorAfterFirstID reaches diff.go:109-112 —
// a candidate advances past its first record and then errors.
func TestDiffFunc_CandidateAdvanceErrorAfterFirstID(t *testing.T) {
	wantErr := errors.New("cand advance boom")
	baseline := SliceToSeq([]record.WithID[string]{mkRec("u1"), mkRec("u2")})
	cand := func(yield func(record.WithID[string], error) bool) {
		if !yield(mkRec("u1"), nil) {
			return
		}
		yield(record.WithID[string]{}, wantErr)
	}

	var lastErr error
	for _, err := range Diff[string](baseline, []RecordSeq[string]{cand}) {
		if err != nil {
			lastErr = err
			break
		}
	}
	require.Error(t, lastErr)
	assert.True(t, errors.Is(lastErr, wantErr))
}

// TestDiff_ConsumerBreaksEarly reaches diff.go:118-120 — the yield returns
// false because the consumer broke out of the range loop.
func TestDiff_ConsumerBreaksEarly(t *testing.T) {
	baseline := SliceToSeq([]record.WithID[string]{mkRec("u1"), mkRec("u2"), mkRec("u3")})
	cand := SliceToSeq([]record.WithID[string]{})

	count := 0
	for range Diff[string](baseline, []RecordSeq[string]{cand}) {
		count++
		break
	}
	assert.Equal(t, 1, count, "consumer-break must terminate the stream after the first emission")
}

// TestRenderJSON_StreamError reaches render_json.go:32-34 — stream yields an
// error, RenderJSON returns ("", err) verbatim.
func TestRenderJSON_StreamError(t *testing.T) {
	out, err := RenderJSON[string](errIDDiffStream(io.ErrUnexpectedEOF), "users")
	require.Error(t, err)
	assert.True(t, errors.Is(err, io.ErrUnexpectedEOF))
	assert.Empty(t, out)
}

// TestRenderJSON_MarshalError reaches render_json.go:39-41 — json.MarshalIndent
// fails because a FieldValue.Value carries a chan (json: unsupported type).
func TestRenderJSON_MarshalError(t *testing.T) {
	bad := IDDiff[string]{
		ID: "u1",
		Candidates: []CandidateState{
			{Status: Changed, Fields: []FieldValue{{Name: "ch", Value: make(chan int)}}},
		},
	}
	out, err := RenderJSON[string](singleIDDiffStream(bad), "users")
	require.Error(t, err)
	assert.Empty(t, out)
}

// TestRenderYAML_StreamError reaches render_yaml.go:33-35.
func TestRenderYAML_StreamError(t *testing.T) {
	out, err := RenderYAML[string](errIDDiffStream(io.ErrUnexpectedEOF), "users")
	require.Error(t, err)
	assert.True(t, errors.Is(err, io.ErrUnexpectedEOF))
	assert.Empty(t, out)
}

// TestRenderYAML_MarshalError reaches render_yaml.go:40-42 — yaml.Marshal
// returns an error because a FieldValue.Value's MarshalYAML method fails.
func TestRenderYAML_MarshalError(t *testing.T) {
	bad := IDDiff[string]{
		ID: "u1",
		Candidates: []CandidateState{
			{Status: Changed, Fields: []FieldValue{{Name: "bad", Value: failMarshal{}}}},
		},
	}
	out, err := RenderYAML[string](singleIDDiffStream(bad), "users")
	require.Error(t, err)
	assert.Empty(t, out)
}

// TestRenderYAMLByID_StreamError reaches render_yaml_byid.go:39-41.
func TestRenderYAMLByID_StreamError(t *testing.T) {
	out, err := RenderYAMLByID[string](errIDDiffStream(io.ErrUnexpectedEOF), "users")
	require.Error(t, err)
	assert.True(t, errors.Is(err, io.ErrUnexpectedEOF))
	assert.Empty(t, out)
}

// TestRenderYAMLByID_EmptyStream reaches render_yaml_byid.go:47-51 — the
// empty-body flow-style branch. Output is "<name>: {}\n".
func TestRenderYAMLByID_EmptyStream(t *testing.T) {
	empty := func(yield func(IDDiff[string], error) bool) {}
	out, err := RenderYAMLByID[string](empty, "users")
	require.NoError(t, err)
	assert.Equal(t, "users: {}\n", out)
}

// TestRenderYAMLByID_EmptyBaselineFields reaches render_yaml_byid.go:103-106 —
// buildBaselineNode with an empty Fields slice renders as a flow-style {}.
func TestRenderYAMLByID_EmptyBaselineFields(t *testing.T) {
	d := IDDiff[string]{
		ID:         "u1",
		Baseline:   &RecordSnapshot{Fields: nil},
		Candidates: []CandidateState{{Status: Missing}},
	}
	out, err := RenderYAMLByID[string](singleIDDiffStream(d), "users")
	require.NoError(t, err)
	assert.Contains(t, out, "baseline: {}")
}

// TestRenderYAMLByID_EmptyCandidates reaches render_yaml_byid.go:119-122 —
// buildCandidatesNode with an empty slice renders as a flow-style {}.
func TestRenderYAMLByID_EmptyCandidates(t *testing.T) {
	d := IDDiff[string]{
		ID:         "u1",
		Candidates: nil,
	}
	out, err := RenderYAMLByID[string](singleIDDiffStream(d), "users")
	require.NoError(t, err)
	assert.Contains(t, out, "candidates: {}")
}

// TestValueNode_EncodeError reaches render_yaml_byid.go:189-191 — valueNode
// falls back to a null scalar when n.Encode returns an error.
func TestValueNode_EncodeError(t *testing.T) {
	n := valueNode(failMarshal{})
	require.NotNil(t, n)
	assert.Equal(t, "null", n.Value)
	assert.Equal(t, "!!null", n.Tag)
}

// TestStatusName_DefaultBranch reaches render_yaml_byid.go:206-207 — the
// default case for an unknown RecordStatus value.
func TestStatusName_DefaultBranch(t *testing.T) {
	assert.Equal(t, "unknown", statusName(RecordStatus(99)))
}

// TestRenderYAMLGitStyle_CandidateIndexOutOfRange reaches
// render_yaml_gitstyle.go:32-33 — the continue path when candidateIndex is
// outside the [0, len(Candidates)) range.
func TestRenderYAMLGitStyle_CandidateIndexOutOfRange(t *testing.T) {
	// Index out of range -> renderer silently skips and emits the empty form.
	baseline := SliceToSeq([]record.WithID[string]{mkRec("u1")})
	cand := SliceToSeq([]record.WithID[string]{})
	diffs := Diff[string](baseline, []RecordSeq[string]{cand})
	out, err := RenderYAMLGitStyle(diffs, 5, "users")
	require.NoError(t, err)
	assert.Equal(t, "users: {}\n", out)
}
