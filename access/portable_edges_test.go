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
	if _, err := ParseTugQLPolicy(make([]byte, maxPolicyFileBytes+1)); err == nil {
		t.Fatal("oversized policy accepted")
	}
	base := "apiVersion: tugql.org/access/v1\nkind: AccessPolicy\nmetadata: {name: p}\ntarget: {database: db}\ndefault: deny\n"
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
		"execution mask":        "execution: {allow: [{class: tugql, mask: x}]}\nscopes: []\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseTugQLPolicy([]byte(base + body)); err == nil {
				t.Fatal("malformed policy accepted")
			}
		})
	}
	if _, err := ParseTugQLPolicy([]byte(base + "scopes: []\n---\n[")); err == nil {
		t.Fatal("malformed trailing document accepted")
	}
}

func TestLegacyPortableSyntaxAcceptsOrdinaryPolicy(t *testing.T) {
	if err := validatePortablePolicyYAML([]byte("scopes: [{path: /x, rules: []}]")); err != nil {
		t.Fatal(err)
	}
	if scopedPolicyFeature(nil) != "" {
		t.Fatal("nil scopes reported feature")
	}
}

func TestNormalizePortablePolicyDefensiveErrors(t *testing.T) {
	mask := Mask{Stages: []MaskStage{{Include: []string{"*"}}}}
	rule := TugQLRule{ID: "r", Effect: "allow", Operations: []string{"get"}, Fields: []string{}, FieldMask: &mask}
	for _, document := range []TugQLDocument{
		{Target: TugQLTarget{Database: "db"}, Scopes: []TugQLScope{{Rules: []TugQLRule{rule}}}},
		{Target: TugQLTarget{Database: "db"}, RuleSets: map[string][]TugQLScope{"x": {{Rules: []TugQLRule{rule}}}}},
	} {
		if _, err := NormalizeTugQLPolicy(document); err == nil {
			t.Fatal("fields plus field mask accepted")
		}
	}
	if _, err := NormalizeTugQLPolicy(TugQLDocument{}); err == nil || !strings.Contains(err.Error(), "target.database") {
		t.Fatalf("missing target err=%v", err)
	}
}

func TestNormalizePortablePolicyRecursiveFailures(t *testing.T) {
	base, err := ParseTugQLPolicy([]byte(portablePolicy("p", "public", validPortableScopes)))
	if err != nil {
		t.Fatal(err)
	}
	mask := Mask{Stages: []MaskStage{{Include: []string{"*"}}}}
	candidate := base
	candidate.Scopes[0].Scopes = []TugQLScope{{Path: "/child", Rules: []TugQLRule{{ID: "r", Effect: "allow", Operations: []string{"get"}, Fields: []string{}, FieldMask: &mask}}}}
	if _, err := NormalizeTugQLPolicy(candidate); err == nil {
		t.Fatal("nested duplicate selector accepted")
	}
	base, _ = ParseTugQLPolicy([]byte(portablePolicy("p", "public", validPortableScopes)))
	candidate = base
	candidate.Scopes[0].Rules[0].Where = &DocumentCondition{Op: "==", Left: &DocumentExpression{Field: "id"}, Right: &DocumentExpression{Value: make(chan int)}}
	if _, err := NormalizeTugQLPolicy(candidate); err == nil {
		t.Fatal("unmarshalable constant accepted")
	}
	base, _ = ParseTugQLPolicy([]byte(portablePolicy("p", "public", validPortableScopes)))
	candidate = base
	candidate.Scopes[0].Rules[0].Where = &DocumentCondition{Op: "==", Left: &DocumentExpression{Field: "id"}, Right: &DocumentExpression{Value: uint64(math.MaxUint64)}}
	if _, err := NormalizeTugQLPolicy(candidate); err != nil {
		t.Fatalf("uint64 restore: %v", err)
	}
	tooLarge := json.Number("18446744073709551616")
	failing := &DocumentCondition{And: []DocumentCondition{{Or: []DocumentCondition{{Op: "==", Left: &DocumentExpression{Field: "id"}, Right: &DocumentExpression{Values: []any{map[string]any{"n": tooLarge}}}}}}}}
	for name, mutate := range map[string]func(*TugQLDocument){
		"where": func(d *TugQLDocument) { d.Scopes[0].Rules[0].Where = failing },
		"check": func(d *TugQLDocument) {
			d.Scopes[0].Rules[0].Operations = []string{"set"}
			d.Scopes[0].Rules[0].Check = failing
		},
		"nested": func(d *TugQLDocument) {
			d.Scopes[0].Scopes = []TugQLScope{{Path: "/child", Rules: []TugQLRule{{ID: "r", Effect: "allow", Operations: []string{"get"}, Where: failing}}}}
		},
		"ruleset": func(d *TugQLDocument) {
			d.RuleSets = map[string][]TugQLScope{"r": {{Path: "/x", Rules: []TugQLRule{{ID: "r", Effect: "allow", Operations: []string{"get"}, Where: failing}}}}}
			d.Scopes = nil
			d.Bindings = &DocumentBindings{Everyone: []string{"r"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			d, _ := ParseTugQLPolicy([]byte(portablePolicy("p", "public", validPortableScopes)))
			mutate(&d)
			if _, err := NormalizeTugQLPolicy(d); err == nil {
				t.Fatal("oversized nested number accepted")
			}
		})
	}
}

func TestNormalizePortablePolicyRejectsDeepConstant(t *testing.T) {
	doc, err := ParseTugQLPolicy([]byte(portablePolicy("p", "public", validPortableScopes)))
	if err != nil {
		t.Fatal(err)
	}
	var value any = "leaf"
	for range 10001 {
		value = []any{value}
	}
	doc.Scopes[0].Rules[0].Where = &DocumentCondition{Op: "==", Left: &DocumentExpression{Field: "id"}, Right: &DocumentExpression{Value: value}}
	if _, err := NormalizeTugQLPolicy(doc); err == nil || !strings.Contains(err.Error(), "depth") {
		t.Fatalf("depth overflow err=%v", err)
	}
}

func TestNormalizePortablePolicyRechecksSelectorsAfterMarshal(t *testing.T) {
	doc, err := ParseTugQLPolicy([]byte(portablePolicy("p", "public", validPortableScopes)))
	if err != nil {
		t.Fatal(err)
	}
	mask := Mask{Stages: []MaskStage{{Include: []string{"*"}}}}
	doc.Scopes[0].Rules[0].FieldMask = &mask
	doc.Scopes[0].Rules[0].Where = &DocumentCondition{Op: "==", Left: &DocumentExpression{Field: "id"}}
	doc.Scopes[0].Rules[0].Where.Right = &DocumentExpression{Value: mutatingJSONValue{mutate: func() { doc.Scopes[0].Rules[0].Fields = []string{"name"} }}}
	if _, err := NormalizeTugQLPolicy(doc); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("post-marshal mutation err=%v", err)
	}
}

func TestNormalizePortablePolicyPropagatesNestedExtensionErrors(t *testing.T) {
	mask := Mask{Stages: []MaskStage{{Include: []string{"*"}}}}
	for name, mutate := range map[string]func(*TugQLDocument){
		"nested": func(d *TugQLDocument) {
			d.Scopes[0].Scopes = []TugQLScope{{Path: "/child", Rules: []TugQLRule{{ID: "r", Effect: "deny", Operations: []string{"get"}, FieldMask: &mask}}}}
		},
		"ruleset": func(d *TugQLDocument) {
			d.RuleSets = map[string][]TugQLScope{"r": {{Path: "/x", Rules: []TugQLRule{{ID: "r", Effect: "deny", Operations: []string{"get"}, FieldMask: &mask}}}}}
		},
		"namespace": func(d *TugQLDocument) {
			d.Execution = &ExecutionGate{Allow: []ExecutionEntry{{Class: ExecutionStoredProcedure, Namespace: " public", Mask: &mask}}}
		},
		"procedure mask": func(d *TugQLDocument) {
			bad := Mask{Stages: []MaskStage{{Include: []string{"bad?name"}}}}
			d.Execution = &ExecutionGate{Allow: []ExecutionEntry{{Class: ExecutionStoredProcedure, Namespace: "public", Mask: &bad}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			d, _ := ParseTugQLPolicy([]byte(portablePolicy("p", "public", validPortableScopes)))
			mutate(&d)
			if _, err := NormalizeTugQLPolicy(d); err == nil {
				t.Fatal("invalid nested extension accepted")
			}
		})
	}
}
