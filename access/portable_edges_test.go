package access

import (
	"strings"
	"testing"
)

func TestPortablePolicyRejectsMalformedSecurityShapes(t *testing.T) {
	if _, err := ParseDTQLPolicy(make([]byte, maxPolicyFileBytes+1)); err == nil {
		t.Fatal("oversized policy accepted")
	}
	base := "apiVersion: dtql.org/access/v1\nkind: AccessPolicy\nmetadata: {name: p}\ntarget: {database: db}\ndefault: deny\n"
	for name, body := range map[string]string{
		"mask scalar":           "collectionMask: x\nscopes: []\n",
		"mask stages scalar":    "collectionMask: {stages: x}\nscopes: []\n",
		"stage extra keys":      "collectionMask: {stages: [{include: ['*'], exclude: ['x']}]}\nscopes: []\n",
		"stage bad action":      "collectionMask: {stages: [{other: []}]}\nscopes: []\n",
		"scope collection mask": "scopes: [{path: '/', collectionMask: {stages: [{include: ['*']}]}}]\n",
		"fields and mask":       "scopes: [{path: '/', rules: [{id: r, effect: allow, operations: [get], fields: [], fieldMask: {stages: [{include: ['*']}]}}]}]\n",
		"nested bad mask":       "scopes: [{path: '/', scopes: [{path: /x, rules: [{id: r, effect: allow, operations: [get], fieldMask: x}]}]}]\n",
		"ruleset bad mask":      "ruleSets: {r: [{path: '/', rules: [{id: r, effect: allow, operations: [get], fieldMask: x}]}]}\nscopes: []\n",
		"execution scalar":      "execution: x\nscopes: []\n",
		"execution mask":        "execution: {allow: [{class: dtql, mask: x}]}\nscopes: []\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDTQLPolicy([]byte(base + body)); err == nil {
				t.Fatal("malformed policy accepted")
			}
		})
	}
	if _, err := ParseDTQLPolicy([]byte(base + "scopes: []\n---\n[")); err == nil {
		t.Fatal("malformed trailing document accepted")
	}
}

func TestNormalizePortablePolicyDefensiveErrors(t *testing.T) {
	mask := Mask{Stages: []MaskStage{{Include: []string{"*"}}}}
	rule := DTQLRule{ID: "r", Effect: "allow", Operations: []string{"get"}, Fields: []string{}, FieldMask: &mask}
	for _, document := range []DTQLDocument{
		{Target: DTQLTarget{Database: "db"}, Scopes: []DTQLScope{{Rules: []DTQLRule{rule}}}},
		{Target: DTQLTarget{Database: "db"}, RuleSets: map[string][]DTQLScope{"x": {{Rules: []DTQLRule{rule}}}}},
	} {
		if _, err := NormalizeDTQLPolicy(document); err == nil {
			t.Fatal("fields plus field mask accepted")
		}
	}
	if _, err := NormalizeDTQLPolicy(DTQLDocument{}); err == nil || !strings.Contains(err.Error(), "target.database") {
		t.Fatalf("missing target err=%v", err)
	}
}
