// specscore: feat-recordops/diff
package recordops

import (
	"encoding/json"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// recStructured is a local helper for building a record.WithID[string]
// carrying a dal.Record whose Data() returns the supplied map. The
// per-test-file name avoids collisions with helpers in sibling test files.
func recStructured(id string, data map[string]any) record.WithID[string] {
	key := dal.NewKeyWithID("Users", id)
	r := dal.NewRecordWithData(key, data)
	r.SetError(nil)
	return record.WithID[string]{ID: id, Record: r}
}

// renderFixtureStream builds a fresh diff stream covering one of each
// non-Matched status plus (with WithIncludeMatched) one Matched.
//   - u1: Matched (both have {name:"Al"})
//   - u2: Changed (baseline {name:"Bea"} vs candidate {name:"Beatrice"})
//   - u3: Missing (only in baseline)
//   - u4: Extra   (only in candidate)
func renderFixtureStream() (RecordSeq[string], []RecordSeq[string]) {
	baseline := SliceToSeq([]record.WithID[string]{
		recStructured("u1", map[string]any{"name": "Al"}),
		recStructured("u2", map[string]any{"name": "Bea"}),
		recStructured("u3", map[string]any{"name": "Cy"}),
	})
	candidate := SliceToSeq([]record.WithID[string]{
		recStructured("u1", map[string]any{"name": "Al"}),
		recStructured("u2", map[string]any{"name": "Beatrice"}),
		recStructured("u4", map[string]any{"name": "Dee"}),
	})
	return baseline, []RecordSeq[string]{candidate}
}

func TestRenderJSON_RoundTrips(t *testing.T) {
	base, cands := renderFixtureStream()
	stream := Diff[string](base, cands, WithIncludeMatched())

	out, err := RenderJSON[string](stream, "users")
	require.NoError(t, err)
	require.NotEmpty(t, out)

	var parsed map[string][]IDDiff[string]
	require.NoError(t, json.Unmarshal([]byte(out), &parsed))

	require.Contains(t, parsed, "users")
	got := parsed["users"]
	require.Len(t, got, 4)

	ids := make([]string, len(got))
	for i, d := range got {
		ids[i] = d.ID
	}
	assert.Equal(t, []string{"u1", "u2", "u3", "u4"}, ids)
}

func TestRenderYAML_ParsesAsValidYAML(t *testing.T) {
	base, cands := renderFixtureStream()
	stream := Diff[string](base, cands, WithIncludeMatched())

	out, err := RenderYAML[string](stream, "users")
	require.NoError(t, err)
	require.NotEmpty(t, out)

	var parsed map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(out), &parsed))
	require.Contains(t, parsed, "users")

	arr, ok := parsed["users"].([]any)
	require.True(t, ok, "users key must be a sequence")
	assert.Len(t, arr, 4)
}

func TestRender_Determinism(t *testing.T) {
	// JSON determinism.
	baseA, candsA := renderFixtureStream()
	jsonA, err := RenderJSON[string](Diff[string](baseA, candsA, WithIncludeMatched()), "users")
	require.NoError(t, err)
	baseB, candsB := renderFixtureStream()
	jsonB, err := RenderJSON[string](Diff[string](baseB, candsB, WithIncludeMatched()), "users")
	require.NoError(t, err)
	assert.Equal(t, jsonA, jsonB, "JSON output must be byte-equal across runs")

	// YAML determinism.
	baseC, candsC := renderFixtureStream()
	yamlA, err := RenderYAML[string](Diff[string](baseC, candsC, WithIncludeMatched()), "users")
	require.NoError(t, err)
	baseD, candsD := renderFixtureStream()
	yamlB, err := RenderYAML[string](Diff[string](baseD, candsD, WithIncludeMatched()), "users")
	require.NoError(t, err)
	assert.Equal(t, yamlA, yamlB, "YAML output must be byte-equal across runs")
}

// absentFlagFixture: baseline has {name:"Al", nickname:"Big Al"}; candidate
// has {name:"Al"} — `nickname` is absent. With default options, the diff
// stream emits a Changed candidate state with a FieldValue{Name:"nickname",
// Absent:true} delta.
func absentFlagFixture() (RecordSeq[string], []RecordSeq[string]) {
	baseline := SliceToSeq([]record.WithID[string]{
		recStructured("u1", map[string]any{"name": "Al", "nickname": "Big Al"}),
	})
	candidate := SliceToSeq([]record.WithID[string]{
		recStructured("u1", map[string]any{"name": "Al"}),
	})
	return baseline, []RecordSeq[string]{candidate}
}

func TestRender_PreservesAbsentFlag_JSON(t *testing.T) {
	base, cands := absentFlagFixture()
	out, err := RenderJSON[string](Diff[string](base, cands), "users")
	require.NoError(t, err)

	var parsed map[string][]IDDiff[string]
	require.NoError(t, json.Unmarshal([]byte(out), &parsed))

	require.Len(t, parsed["users"], 1)
	got := parsed["users"][0]
	require.Equal(t, "u1", got.ID)
	require.Len(t, got.Candidates, 1)
	cand := got.Candidates[0]
	assert.Equal(t, Changed, cand.Status)

	var nickField *FieldValue
	for i := range cand.Fields {
		if cand.Fields[i].Name == "nickname" {
			nickField = &cand.Fields[i]
			break
		}
	}
	require.NotNil(t, nickField, "nickname delta must round-trip")
	assert.True(t, nickField.Absent, "absent flag must survive JSON round-trip")
}

func TestRender_PreservesAbsentFlag_YAML(t *testing.T) {
	base, cands := absentFlagFixture()
	out, err := RenderYAML[string](Diff[string](base, cands), "users")
	require.NoError(t, err)

	var parsed map[string][]IDDiff[string]
	require.NoError(t, yaml.Unmarshal([]byte(out), &parsed))

	require.Len(t, parsed["users"], 1)
	got := parsed["users"][0]
	require.Equal(t, "u1", got.ID)
	require.Len(t, got.Candidates, 1)
	cand := got.Candidates[0]
	assert.Equal(t, Changed, cand.Status)

	var nickField *FieldValue
	for i := range cand.Fields {
		if cand.Fields[i].Name == "nickname" {
			nickField = &cand.Fields[i]
			break
		}
	}
	require.NotNil(t, nickField, "nickname delta must round-trip")
	assert.True(t, nickField.Absent, "absent flag must survive YAML round-trip")
}
