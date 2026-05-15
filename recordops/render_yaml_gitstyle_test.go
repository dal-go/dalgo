// specscore: feat-recordops/diff
package recordops

import (
	"errors"
	"io"
	"iter"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rec(id string, data map[string]any) record.WithID[string] {
	key := dal.NewKeyWithID("Users", id)
	r := dal.NewRecordWithData(key, data)
	r.SetError(nil)
	return record.WithID[string]{ID: id, Record: r}
}

func TestRenderYAMLGitStyle_GoldenIdeaExample(t *testing.T) {
	baseline := SliceToSeq([]record.WithID[string]{
		rec("u1", map[string]any{}),
		rec("u3", map[string]any{"first_name": "Alex"}),
	})
	cand := SliceToSeq([]record.WithID[string]{
		rec("u2", map[string]any{"first_name": "Jack", "gender": "male"}),
		rec("u3", map[string]any{"first_name": "Alexander"}),
	})

	diffs := Diff[string](baseline, []RecordSeq[string]{cand})
	out, err := RenderYAMLGitStyle(diffs, 0, "users")
	require.NoError(t, err)

	want := "users:\n" +
		"- u1\n" +
		"+ u2:\n" +
		"    first_name: Jack\n" +
		"    gender: male\n" +
		"u3:\n" +
		"-   first_name: Alex\n" +
		"+   first_name: Alexander\n"

	assert.Equal(t, want, out)
}

func TestRenderYAMLGitStyle_EmptyDiff(t *testing.T) {
	baseline := SliceToSeq([]record.WithID[string]{
		rec("u1", map[string]any{"first_name": "Alex"}),
	})
	cand := SliceToSeq([]record.WithID[string]{
		rec("u1", map[string]any{"first_name": "Alex"}),
	})
	diffs := Diff[string](baseline, []RecordSeq[string]{cand})
	out, err := RenderYAMLGitStyle(diffs, 0, "users")
	require.NoError(t, err)
	assert.Equal(t, "users: {}\n", out)
}

func TestRenderYAMLGitStyle_MatchedCandidateSkipped(t *testing.T) {
	baseline := SliceToSeq([]record.WithID[string]{
		rec("u1", map[string]any{"first_name": "Alex"}),
	})
	cand := SliceToSeq([]record.WithID[string]{
		rec("u1", map[string]any{"first_name": "Alex"}),
	})
	diffs := Diff[string](baseline, []RecordSeq[string]{cand}, WithIncludeMatched())
	out, err := RenderYAMLGitStyle(diffs, 0, "users")
	require.NoError(t, err)
	assert.Equal(t, "users: {}\n", out)
}

func TestRenderYAMLGitStyle_StreamErrorPropagated(t *testing.T) {
	wantErr := io.ErrUnexpectedEOF
	stream := func(yield func(IDDiff[string], error) bool) {
		// First, a valid diff so we exercise the consumption-then-error path.
		if !yield(IDDiff[string]{
			ID: "u1",
			Candidates: []CandidateState{
				{Status: Missing},
			},
		}, nil) {
			return
		}
		var zero IDDiff[string]
		yield(zero, wantErr)
	}
	var seq iter.Seq2[IDDiff[string], error] = stream
	out, err := RenderYAMLGitStyle(seq, 0, "users")
	require.Error(t, err)
	assert.True(t, errors.Is(err, io.ErrUnexpectedEOF), "expected ErrUnexpectedEOF, got %v", err)
	assert.Equal(t, "", out)
}

func TestRenderYAMLGitStyle_AbsentFieldOmitsPlusLine(t *testing.T) {
	baseline := SliceToSeq([]record.WithID[string]{
		rec("u1", map[string]any{"first_name": "Alex", "nickname": "Al"}),
	})
	cand := SliceToSeq([]record.WithID[string]{
		rec("u1", map[string]any{"first_name": "Alex"}),
	})
	diffs := Diff[string](baseline, []RecordSeq[string]{cand})
	out, err := RenderYAMLGitStyle(diffs, 0, "users")
	require.NoError(t, err)

	assert.Contains(t, out, "-   nickname: Al\n")
	assert.NotContains(t, out, "+   nickname:")
	// Sanity-check the surrounding structure.
	assert.Contains(t, out, "u1:\n")
}
