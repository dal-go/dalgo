// specscore: feat-recordops/diff
package recordops

import (
	"testing"

	"github.com/dal-go/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// recByID constructs a record.WithID whose record.Record carries the given data
// payload. Used to build inputs for the RenderYAMLByID tests.
func recByID(id string, data map[string]any) record.WithID[string] {
	key := record.NewKeyWithID("Users", id)
	rec := record.NewRecordWithData(key, data)
	rec.SetError(nil)
	return record.WithID[string]{ID: id, Record: rec}
}

func TestRenderYAMLByID_ProducesValidYAML(t *testing.T) {
	baseline := SliceToSeq([]record.WithID[string]{
		recByID("u1", map[string]any{"first_name": "Alex", "nickname": "Al"}),
		recByID("u3", map[string]any{"first_name": "Charlie"}),
	})
	cand := SliceToSeq([]record.WithID[string]{
		recByID("u1", map[string]any{"first_name": "Alex"}),
		recByID("u2", map[string]any{"first_name": "Jack"}),
	})

	out, err := RenderYAMLByID(Diff[string](baseline, []RecordSeq[string]{cand}), "users")
	require.NoError(t, err)
	require.NotEmpty(t, out)

	var parsed any
	require.NoError(t, yaml.Unmarshal([]byte(out), &parsed),
		"output must parse as valid YAML; got:\n%s", out)
}

func TestRenderYAMLByID_IncludesAllCandidates(t *testing.T) {
	baseline := SliceToSeq([]record.WithID[string]{
		recByID("u1", map[string]any{"first_name": "Alex"}),
	})
	// Three candidates: c0 matches, c1 changed, c2 missing.
	c0 := SliceToSeq([]record.WithID[string]{
		recByID("u1", map[string]any{"first_name": "Alex"}),
	})
	c1 := SliceToSeq([]record.WithID[string]{
		recByID("u1", map[string]any{"first_name": "Alexander"}),
	})
	c2 := SliceToSeq([]record.WithID[string]{})

	out, err := RenderYAMLByID(
		Diff[string](baseline, []RecordSeq[string]{c0, c1, c2}),
		"users",
	)
	require.NoError(t, err)

	// Parse and inspect the candidates section for u1.
	var parsed map[string]map[string]map[string]map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(out), &parsed), "output:\n%s", out)

	users, ok := parsed["users"]
	require.True(t, ok, "top-level 'users' key missing; output:\n%s", out)
	u1, ok := users["u1"]
	require.True(t, ok, "expected 'u1' block; output:\n%s", out)
	candidates, ok := u1["candidates"]
	require.True(t, ok, "expected 'candidates' section under u1; output:\n%s", out)

	assert.Contains(t, candidates, "0", "candidate index 0 missing")
	assert.Contains(t, candidates, "1", "candidate index 1 missing")
	assert.Contains(t, candidates, "2", "candidate index 2 missing")
}

func TestRenderYAMLByID_Deterministic(t *testing.T) {
	// Materialize a single IDDiff slice; build two independent streams.
	baseline1 := SliceToSeq([]record.WithID[string]{
		recByID("u1", map[string]any{"first_name": "Alex", "nickname": "Al"}),
		recByID("u3", map[string]any{"first_name": "Charlie"}),
	})
	cand1 := SliceToSeq([]record.WithID[string]{
		recByID("u1", map[string]any{"first_name": "Alex"}),
		recByID("u2", map[string]any{"first_name": "Jack"}),
	})
	diffs := collect(t, Diff[string](baseline1, []RecordSeq[string]{cand1}))
	require.NotEmpty(t, diffs)

	stream1 := func(yield func(IDDiff[string], error) bool) {
		for _, d := range diffs {
			if !yield(d, nil) {
				return
			}
		}
	}
	stream2 := func(yield func(IDDiff[string], error) bool) {
		for _, d := range diffs {
			if !yield(d, nil) {
				return
			}
		}
	}

	out1, err := RenderYAMLByID[string](stream1, "users")
	require.NoError(t, err)
	out2, err := RenderYAMLByID[string](stream2, "users")
	require.NoError(t, err)

	assert.Equal(t, out1, out2, "renderer must be deterministic for the same input")
}

func TestRenderYAMLByID_AbsentFieldStructured(t *testing.T) {
	baseline := SliceToSeq([]record.WithID[string]{
		recByID("u1", map[string]any{"first_name": "Alex", "nickname": "Al"}),
	})
	cand := SliceToSeq([]record.WithID[string]{
		recByID("u1", map[string]any{"first_name": "Alex"}),
	})

	out, err := RenderYAMLByID(Diff[string](baseline, []RecordSeq[string]{cand}), "users")
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(out), &parsed), "output:\n%s", out)

	// Navigate: users.u1.candidates."0".fields.nickname
	users, _ := parsed["users"].(map[string]any)
	require.NotNil(t, users)
	u1, _ := users["u1"].(map[string]any)
	require.NotNil(t, u1, "expected u1 block; output:\n%s", out)
	candidates, _ := u1["candidates"].(map[string]any)
	require.NotNil(t, candidates)
	c0, _ := candidates["0"].(map[string]any)
	require.NotNil(t, c0, "expected candidate '0' block; output:\n%s", out)
	fields, _ := c0["fields"].(map[string]any)
	require.NotNil(t, fields, "expected 'fields' under candidate 0; output:\n%s", out)
	nick, _ := fields["nickname"].(map[string]any)
	require.NotNil(t, nick, "expected nickname entry; output:\n%s", out)

	absentVal, hasAbsent := nick["absent"]
	require.True(t, hasAbsent, "nickname must contain 'absent' key; got %#v", nick)
	assert.Equal(t, true, absentVal, "absent must be boolean true; got %#v", absentVal)

	// And must NOT carry a 'new' key — the absence is the sole signal.
	_, hasNew := nick["new"]
	assert.False(t, hasNew, "absent field must not also carry 'new'; got %#v", nick)
}

func TestRenderYAMLByID_NilValueDistinctFromAbsent(t *testing.T) {
	baseline := SliceToSeq([]record.WithID[string]{
		recByID("u1", map[string]any{"first_name": "Alex"}),
	})
	cand := SliceToSeq([]record.WithID[string]{
		recByID("u1", map[string]any{"first_name": nil}),
	})

	out, err := RenderYAMLByID(Diff[string](baseline, []RecordSeq[string]{cand}), "users")
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(out), &parsed), "output:\n%s", out)

	users, _ := parsed["users"].(map[string]any)
	require.NotNil(t, users)
	u1, _ := users["u1"].(map[string]any)
	require.NotNil(t, u1, "expected u1 block; output:\n%s", out)
	candidates, _ := u1["candidates"].(map[string]any)
	require.NotNil(t, candidates)
	c0, _ := candidates["0"].(map[string]any)
	require.NotNil(t, c0)
	fields, _ := c0["fields"].(map[string]any)
	require.NotNil(t, fields, "expected 'fields' under candidate 0; output:\n%s", out)
	fn, _ := fields["first_name"].(map[string]any)
	require.NotNil(t, fn, "expected first_name entry; output:\n%s", out)

	// Must have 'new' key with nil value, NOT 'absent: true'.
	newVal, hasNew := fn["new"]
	require.True(t, hasNew, "first_name must contain 'new' key for a real nil value; got %#v", fn)
	assert.Nil(t, newVal, "new value must be YAML null; got %#v", newVal)

	_, hasAbsent := fn["absent"]
	assert.False(t, hasAbsent, "real nil value must NOT render as absent:true; got %#v", fn)
}
