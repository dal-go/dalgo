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
