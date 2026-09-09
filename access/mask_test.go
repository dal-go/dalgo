package access

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"os"
	"strings"
	"testing"
)

func TestScopedMaskApprovedVectors(t *testing.T) {
	data, err := os.ReadFile("testdata/scoped-masks/scoped-mask-vectors.json")
	require.NoError(t, err)
	var vectors struct {
		Mask               Mask
		CanonicalMask      Mask
		ExpectedMembership map[string]bool
		AdditionalCases    []struct {
			Mask                 Mask
			Name                 string
			Expected             any
			CanonicalMask        *Mask
			ExpectedMembership   map[string]bool
			ExpectedEffective    *bool
			AnotherOwnerIncludes []string
			AnotherOwnerExcludes []string
		}
	}
	require.NoError(t, json.Unmarshal(data, &vectors))
	compiled, err := CompileMask(vectors.Mask, NameMask)
	require.NoError(t, err)
	require.Equal(t, vectors.CanonicalMask, compiled.Canonical())
	for name, expected := range vectors.ExpectedMembership {
		require.Equal(t, expected, compiled.Allows(name), name)
	}
	for _, v := range vectors.AdditionalCases {
		c, e := CompileMask(v.Mask, FieldMask)
		require.NoError(t, e)
		if expected, ok := v.Expected.(bool); ok {
			require.Equal(t, expected, c.Allows(v.Name), v.Name)
		}
		if v.CanonicalMask != nil {
			require.Equal(t, *v.CanonicalMask, c.Canonical())
		}
		for name, expected := range v.ExpectedMembership {
			require.Equal(t, expected, c.Allows(name), name)
		}
		if v.ExpectedEffective != nil {
			stages := []MaskStage{{Include: []string{"*"}}}
			if v.AnotherOwnerIncludes != nil {
				stages[0].Include = v.AnotherOwnerIncludes
			}
			if v.AnotherOwnerExcludes != nil {
				stages = append(stages, MaskStage{Exclude: v.AnotherOwnerExcludes})
			}
			other, e := CompileMask(Mask{Stages: stages}, NameMask)
			require.NoError(t, e)
			require.Equal(t, *v.ExpectedEffective, c.Allows(v.Name) && other.Allows(v.Name))
		}
		// Projection and mutation cases are exercised by the enforcement task.
	}
	canonical := compiled.Canonical()
	canonical.Stages[0].Include[0] = "z*"
	require.False(t, compiled.Allows("zcd"), "returned canonical document must not mutate evaluator")
}

func TestScopedMaskValidationAndFields(t *testing.T) {
	for _, m := range []Mask{
		{}, {Stages: []MaskStage{{Exclude: []string{"*"}}}},
		{Stages: []MaskStage{{Include: []string{}}}},
		{Stages: []MaskStage{{Include: []string{"*"}, Exclude: []string{"x"}}}},
		{Stages: []MaskStage{{Include: []string{strings.Repeat("a", 129)}}}},
		{Stages: []MaskStage{{Include: []string{"a/b"}}}},
	} {
		_, err := CompileMask(m, NameMask)
		require.Error(t, err)
	}
	m := Mask{Stages: []MaskStage{{Include: []string{"address.**"}}, {Exclude: []string{"address.secret"}}, {Include: []string{"address.secret.public"}}}}
	c, err := CompileMask(m, FieldMask)
	require.NoError(t, err)
	require.True(t, c.Allows("address.city"))
	require.False(t, c.Allows("address.secret.value"))
	require.True(t, c.Allows("address.secret.public"))
	require.False(t, c.Allows("other.secret.public"))
	require.False(t, c.CompleteSubtree("address"))
}

func TestScopedMaskBudgetsAndUnicode(t *testing.T) {
	stages := make([]MaskStage, MaxMaskStages+1)
	for i := range stages {
		stages[i] = MaskStage{Include: []string{"*"}}
	}
	_, err := CompileMask(Mask{Stages: stages}, NameMask)
	require.Error(t, err)
	patterns := make([]string, MaxMaskPatterns+1)
	for i := range patterns {
		patterns[i] = "*"
	}
	_, err = CompileMask(Mask{Stages: []MaskStage{{Include: patterns}}}, NameMask)
	require.Error(t, err)
	c, err := CompileMask(Mask{Stages: []MaskStage{{Include: []string{"É*界", "*"}}}}, NameMask)
	require.NoError(t, err)
	require.False(t, c.Allows("a/b"))
	require.False(t, c.Allows("a.b"))
	c, err = CompileMask(Mask{Stages: []MaskStage{{Include: []string{"É*界"}}}}, NameMask)
	require.NoError(t, err)
	require.True(t, c.Allows("É世界"))
	require.False(t, c.Allows("é世界"))
}
