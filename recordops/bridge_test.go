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
	records  []dal.Record
	idx      int
	err      error // returned at position errAt (after records[errAt-1] is yielded)
	errAt    int
	closeCnt int
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
	rec1 := dal.NewRecordWithData(dal.NewKeyWithID("Things", "a"), &struct{}{})
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
		name    string
		setup   func() *fakeReader
		consume func(seq RecordSeq[string])
	}{
		{
			name: "exhaustion",
			setup: func() *fakeReader {
				rec := dal.NewRecordWithData(dal.NewKeyWithID("Things", "a"), &struct{}{})
				return &fakeReader{records: []dal.Record{rec}}
			},
			consume: func(seq RecordSeq[string]) {
				for range seq { /* drain */
				}
			},
		},
		{
			name: "early-break",
			setup: func() *fakeReader {
				rec1 := dal.NewRecordWithData(dal.NewKeyWithID("Things", "a"), &struct{}{})
				rec2 := dal.NewRecordWithData(dal.NewKeyWithID("Things", "b"), &struct{}{})
				return &fakeReader{records: []dal.Record{rec1, rec2}}
			},
			consume: func(seq RecordSeq[string]) {
				for range seq {
					break /* take first only */
				}
			},
		},
		{
			name: "upstream-error",
			setup: func() *fakeReader {
				return &fakeReader{
					records: []dal.Record{dal.NewRecordWithData(dal.NewKeyWithID("Things", "a"), &struct{}{})},
					err:     io.ErrUnexpectedEOF,
					errAt:   1,
				}
			},
			consume: func(seq RecordSeq[string]) {
				for range seq { /* drain — error terminates */
				}
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
