package access

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

type mutatingJSONValue struct{ mutate func() }

func (v mutatingJSONValue) MarshalJSON() ([]byte, error) { v.mutate(); return []byte(`"x"`), nil }

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

func TestNormalizePortablePolicyRecursiveFailures(t *testing.T) {
	base, err := ParseDTQLPolicy([]byte(portablePolicy("p", "public", validPortableScopes)))
	if err != nil {
		t.Fatal(err)
	}
	mask := Mask{Stages: []MaskStage{{Include: []string{"*"}}}}
	candidate := base
	candidate.Scopes[0].Scopes = []DTQLScope{{Path: "/child", Rules: []DTQLRule{{ID: "r", Effect: "allow", Operations: []string{"get"}, Fields: []string{}, FieldMask: &mask}}}}
	if _, err := NormalizeDTQLPolicy(candidate); err == nil {
		t.Fatal("nested duplicate selector accepted")
	}
	base, _ = ParseDTQLPolicy([]byte(portablePolicy("p", "public", validPortableScopes)))
	candidate = base
	candidate.Scopes[0].Rules[0].Where = &DocumentCondition{Op: "==", Left: &DocumentExpression{Field: "id"}, Right: &DocumentExpression{Value: make(chan int)}}
	if _, err := NormalizeDTQLPolicy(candidate); err == nil {
		t.Fatal("unmarshalable constant accepted")
	}
	base, _ = ParseDTQLPolicy([]byte(portablePolicy("p", "public", validPortableScopes)))
	candidate = base
	candidate.Scopes[0].Rules[0].Where = &DocumentCondition{Op: "==", Left: &DocumentExpression{Field: "id"}, Right: &DocumentExpression{Value: uint64(math.MaxUint64)}}
	if _, err := NormalizeDTQLPolicy(candidate); err != nil {
		t.Fatalf("uint64 restore: %v", err)
	}
	tooLarge := json.Number("18446744073709551616")
	failing := &DocumentCondition{And: []DocumentCondition{{Or: []DocumentCondition{{Op: "==", Left: &DocumentExpression{Field: "id"}, Right: &DocumentExpression{Values: []any{map[string]any{"n": tooLarge}}}}}}}}
	for name, mutate := range map[string]func(*DTQLDocument){
		"where": func(d *DTQLDocument) { d.Scopes[0].Rules[0].Where = failing },
		"check": func(d *DTQLDocument) {
			d.Scopes[0].Rules[0].Operations = []string{"set"}
			d.Scopes[0].Rules[0].Check = failing
		},
		"nested": func(d *DTQLDocument) {
			d.Scopes[0].Scopes = []DTQLScope{{Path: "/child", Rules: []DTQLRule{{ID: "r", Effect: "allow", Operations: []string{"get"}, Where: failing}}}}
		},
		"ruleset": func(d *DTQLDocument) {
			d.RuleSets = map[string][]DTQLScope{"r": {{Path: "/x", Rules: []DTQLRule{{ID: "r", Effect: "allow", Operations: []string{"get"}, Where: failing}}}}}
			d.Scopes = nil
			d.Bindings = &DocumentBindings{Everyone: []string{"r"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			d, _ := ParseDTQLPolicy([]byte(portablePolicy("p", "public", validPortableScopes)))
			mutate(&d)
			if _, err := NormalizeDTQLPolicy(d); err == nil {
				t.Fatal("oversized nested number accepted")
			}
		})
	}
}

func TestNormalizePortablePolicyRejectsDecoderDepthOverflow(t *testing.T) {
	doc, err := ParseDTQLPolicy([]byte(portablePolicy("p", "public", validPortableScopes)))
	if err != nil {
		t.Fatal(err)
	}
	var value any = "leaf"
	for range 10001 {
		value = []any{value}
	}
	doc.Scopes[0].Rules[0].Where = &DocumentCondition{Op: "==", Left: &DocumentExpression{Field: "id"}, Right: &DocumentExpression{Value: value}}
	if _, err := NormalizeDTQLPolicy(doc); err == nil || !strings.Contains(err.Error(), "depth") {
		t.Fatalf("depth overflow err=%v", err)
	}
}

func TestNormalizePortablePolicyRechecksSelectorsAfterMarshal(t *testing.T) {
	doc, err := ParseDTQLPolicy([]byte(portablePolicy("p", "public", validPortableScopes)))
	if err != nil {
		t.Fatal(err)
	}
	mask := Mask{Stages: []MaskStage{{Include: []string{"*"}}}}
	doc.Scopes[0].Rules[0].FieldMask = &mask
	doc.Scopes[0].Rules[0].Where = &DocumentCondition{Op: "==", Left: &DocumentExpression{Field: "id"}}
	doc.Scopes[0].Rules[0].Where.Right = &DocumentExpression{Value: mutatingJSONValue{mutate: func() { doc.Scopes[0].Rules[0].Fields = []string{"name"} }}}
	if _, err := NormalizeDTQLPolicy(doc); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("post-marshal mutation err=%v", err)
	}
}
