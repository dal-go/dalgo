// specscore: feat-recordops/diff
package recordops

import (
	"bytes"
	"errors"
	"fmt"
	"iter"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mkRec constructs a record.WithID carrying a real dal.Record with a stable
// Data() value. Task 4's classify path calls Data() on both sides, so a nil
// Record would panic. The data value is deterministic per ID so two records
// with the same ID compare Matched.
func mkRec[K comparable](id K, _ any) record.WithID[K] {
	key := dal.NewKeyWithID("Things", fmt.Sprintf("%v", id))
	rec := dal.NewRecordWithData(key, map[string]any{"id": fmt.Sprintf("%v", id)})
	rec.SetError(nil)
	return record.WithID[K]{ID: id, Record: rec}
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
		mkRec("u2", nil),
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
	// baseline lacks u3, c0 lacks u3 → Missing from baseline-absent perspective
	// (classify returns Missing when cand == nil but base is also nil → falls
	// through to Missing branch only if base==nil && cand==nil yields Matched;
	// for u3 baseRec is nil so cand==nil branch is reached, returning Missing).
	assert.Equal(t, Missing, got[2].Candidates[0].Status)
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
		if !yield(mkRec("a", nil), nil) {
			return
		}
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
