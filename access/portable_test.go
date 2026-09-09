package access

import (
	"github.com/stretchr/testify/require"
	"os"
	"strings"
	"testing"
)

func TestPortablePolicyRoundTrip(t *testing.T) {
	input := portablePolicy("p", "", validPortableScopes) + "collectionMask: {stages: [{include: ['b*', 'a**']}, {include: ['a*']}, {exclude: ['*c*']}, {include: ['**d']}] }\nexecution: {allow: [{class: dtql}]}\n"
	doc, err := ParseDTQLPolicy([]byte(input))
	require.NoError(t, err)
	require.Equal(t, "public", doc.Metadata.Visibility)
	require.Equal(t, []string{"a*", "b*"}, doc.CollectionMask.Stages[0].Include)
	for _, marshal := range []func(DTQLDocument) ([]byte, error){MarshalDTQLPolicyJSON, MarshalDTQLPolicyYAML} {
		text, err := marshal(doc)
		require.NoError(t, err)
		parsed, err := ParseDTQLPolicy(text)
		require.NoError(t, err)
		require.Equal(t, doc, parsed)
	}
	doc.Scopes[0].Rules[0].Where = &DocumentCondition{Op: "==", Left: &DocumentExpression{Field: "id"}, Right: &DocumentExpression{Value: int64(9007199254740993)}}
	normalized, err := NormalizeDTQLPolicy(doc)
	require.NoError(t, err)
	require.Equal(t, int64(9007199254740993), normalized.Scopes[0].Rules[0].Where.Right.Value)
	normalized.CollectionMask.Stages[0].Include[0] = "changed"
	require.Equal(t, "a*", doc.CollectionMask.Stages[0].Include[0])
	require.Error(t, writeDTQLPolicyYAML(badWriter{}, doc))
}

func TestPortablePolicyInvalidExtensions(t *testing.T) {
	for _, extra := range []string{
		"collectionMask: null",
		"collectionMask: {stages: [{include: ['*'], exclude: null}]}",
		"collectionMask: {stages: [{exclude: ['*']}]}",
		"execution: null",
		"execution: {allow: null}",
		"execution: {allow: [{class: something}]}",
		"execution: {allow: [{class: dtql}, {class: dtql}]}",
		"execution: {allow: [{class: stored_procedure, namespace: public}]}",
	} {
		_, err := ParseDTQLPolicy([]byte(portablePolicy("p", "public", validPortableScopes) + extra + "\n"))
		require.Error(t, err, extra)
	}
	_, err := ParseDTQLPolicy([]byte(strings.Replace(portablePolicy("p", "public", validPortableScopes), "operations: [get]", "operations: [get]\n        fieldMask: null", 1)))
	require.Error(t, err)
}

func TestPortableApprovedCanonicalFixtures(t *testing.T) {
	for _, name := range []string{"mask-policy.canonical.yaml", "three-stage-procedure-policy.json"} {
		data, err := os.ReadFile("testdata/scoped-masks/" + name)
		require.NoError(t, err)
		doc, err := ParseDTQLPolicy(data)
		require.NoError(t, err, name)
		for _, marshal := range []func(DTQLDocument) ([]byte, error){MarshalDTQLPolicyJSON, MarshalDTQLPolicyYAML} {
			encoded, err := marshal(doc)
			require.NoError(t, err)
			got, err := ParseDTQLPolicy(encoded)
			require.NoError(t, err)
			require.Equal(t, doc, got)
		}
	}
}

func TestPortableNormalizationPreservesFieldPresence(t *testing.T) {
	doc, err := ParseDTQLPolicy([]byte(portablePolicy("p", "public", validPortableScopes)))
	require.NoError(t, err)
	mask := restoredAddressMask()
	doc.Scopes[0].Rules[0].FieldMask = &mask
	doc.Scopes[0].Rules[0].Fields = []string{}
	_, err = NormalizeDTQLPolicy(doc)
	require.Error(t, err)
}

func TestPortableNormalizationTraversesNestedRuleSetsAndConstants(t *testing.T) {
	doc, err := ParseDTQLPolicy([]byte(portablePolicy("p", "public", validPortableScopes)))
	require.NoError(t, err)
	rule := doc.Scopes[0].Rules[0]
	rule.Where = &DocumentCondition{And: []DocumentCondition{{Op: "In", Left: &DocumentExpression{Field: "id"}, Right: &DocumentExpression{Values: []any{uint64(1), map[string]any{"n": float64(1.5)}}}}}}
	doc.Scopes = nil
	doc.RuleSets = map[string][]DTQLScope{"reader": {{Path: "/users", Rules: []DTQLRule{rule}, Scopes: []DTQLScope{{Path: "/*", Rules: []DTQLRule{{ID: "nested", Effect: "allow", Operations: []string{"get"}}}}}}}}
	doc.Bindings = &DocumentBindings{Everyone: []string{"reader"}}
	_, err = NormalizeDTQLPolicy(doc)
	require.Error(t, err, "non-scalar nested constant must be rejected after recursive normalization")

	doc, err = ParseDTQLPolicy([]byte(portablePolicy("p", "public", validPortableScopes)))
	require.NoError(t, err)
	bad := Mask{}
	doc.Scopes[0].Rules[0].FieldMask = &bad
	_, err = NormalizeDTQLPolicy(doc)
	require.Error(t, err)
}

func TestPortableNormalizationRestoresNestedNumericArrays(t *testing.T) {
	doc, err := ParseDTQLPolicy([]byte(portablePolicy("p", "public", validPortableScopes)))
	require.NoError(t, err)
	doc.Scopes[0].Rules[0].Where = &DocumentCondition{Or: []DocumentCondition{{Op: "In", Left: &DocumentExpression{Field: "id"}, Right: &DocumentExpression{Values: []any{int64(9007199254740993), uint64(9007199254740994)}}}}}
	doc.Scopes[0].Scopes = []DTQLScope{{Path: "/children/*", Rules: []DTQLRule{{ID: "child", Effect: "allow", Operations: []string{"set"}, Check: &DocumentCondition{Op: "==", Left: &DocumentExpression{Field: "score"}, Right: &DocumentExpression{Value: float64(1.5)}}}}}}
	normalized, err := NormalizeDTQLPolicy(doc)
	require.NoError(t, err)
	values := normalized.Scopes[0].Rules[0].Where.Or[0].Right.Values.([]any)
	require.IsType(t, int64(0), values[0])
	require.IsType(t, int64(0), values[1])
}

func TestPortableNormalizationRejectsUnsafeExtensions(t *testing.T) {
	base, err := ParseDTQLPolicy([]byte(portablePolicy("p", "public", validPortableScopes)))
	require.NoError(t, err)
	mask := Mask{Stages: []MaskStage{{Include: []string{"*"}}}}
	tests := map[string]func(*DTQLDocument){
		"target":                func(d *DTQLDocument) { d.Target.Database = " " },
		"scope collection mask": func(d *DTQLDocument) { d.Scopes[0].CollectionMask = &mask },
		"deny field mask":       func(d *DTQLDocument) { d.Scopes[0].Rules[0].Effect = "deny"; d.Scopes[0].Rules[0].FieldMask = &mask },
		"empty execution":       func(d *DTQLDocument) { d.Execution = &ExecutionGate{} },
		"too many execution entries": func(d *DTQLDocument) {
			d.Execution = &ExecutionGate{Allow: make([]ExecutionEntry, 33)}
		},
		"duplicate execution": func(d *DTQLDocument) {
			d.Execution = &ExecutionGate{Allow: []ExecutionEntry{{Class: ExecutionDTQL}, {Class: ExecutionDTQL}}}
		},
		"dtql namespace": func(d *DTQLDocument) {
			d.Execution = &ExecutionGate{Allow: []ExecutionEntry{{Class: ExecutionDTQL, Namespace: "x"}}}
		},
		"procedure namespace": func(d *DTQLDocument) {
			d.Execution = &ExecutionGate{Allow: []ExecutionEntry{{Class: ExecutionStoredProcedure, Namespace: "bad.*", Mask: &mask}}}
		},
		"procedure no mask": func(d *DTQLDocument) {
			d.Execution = &ExecutionGate{Allow: []ExecutionEntry{{Class: ExecutionStoredProcedure, Namespace: "public"}}}
		},
		"unknown execution": func(d *DTQLDocument) { d.Execution = &ExecutionGate{Allow: []ExecutionEntry{{Class: "future"}}} },
		"raw mask limit": func(d *DTQLDocument) {
			patterns := make([]string, MaxMaskPatterns+1)
			for i := range patterns {
				patterns[i] = "x"
			}
			d.CollectionMask = &Mask{Stages: []MaskStage{{Include: patterns}}}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := base
			candidate.Scopes = append([]DTQLScope(nil), base.Scopes...)
			candidate.Scopes[0].Rules = append([]DTQLRule(nil), base.Scopes[0].Rules...)
			mutate(&candidate)
			if _, err := NormalizeDTQLPolicy(candidate); err == nil {
				t.Fatal("unsafe extension accepted")
			}
			if _, err := MarshalDTQLPolicyJSON(candidate); err == nil {
				t.Fatal("JSON marshal accepted unsafe extension")
			}
			if _, err := MarshalDTQLPolicyYAML(candidate); err == nil {
				t.Fatal("YAML marshal accepted unsafe extension")
			}
		})
	}
	if _, err := ParseDTQLPolicy(make([]byte, maxPolicyFileBytes+1)); err == nil {
		t.Fatal("oversize policy parsed")
	}
}
