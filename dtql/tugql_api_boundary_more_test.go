package dtql

import (
	"math"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTugQLOverylayNestedExpressionWireBoundaries(t *testing.T) {
	parseNode := func(text string) *yaml.Node {
		t.Helper()
		var document yaml.Node
		if err := yaml.Unmarshal([]byte(text), &document); err != nil {
			t.Fatal(err)
		}
		return document.Content[0]
	}
	if isTugQLBodyYAML(nil) || isTugQLBodyYAML(parseNode(`null`)) || isTugQLBodyYAML(parseNode("from: {name: T}\n")) {
		t.Fatal("legacy scalar query document was misclassified as a TugQL body")
	}
	if !isTugQLBodyYAML(parseNode("query: {from: {name: T}}\n")) {
		t.Fatal("TugQL nested body was not detected")
	}
	for _, input := range []struct {
		name string
		node *yaml.Node
		want string
	}{
		{name: "body is scalar", node: parseNode(`null`), want: "scalar query body must be a mapping"},
		{name: "unknown body key", node: parseNode("query: {from: {name: T}}\nextra: true\n"), want: "unknown scalar query body field"},
		{name: "duplicate body key", node: parseNode("query: {from: {name: T}}\nquery: {from: {name: U}}\n"), want: "duplicate scalar query body field"},
		{name: "missing body query", node: parseNode("definitions: []\n"), want: "scalar query body requires query"},
	} {
		t.Run(input.name, func(t *testing.T) {
			if err := validateTugQLBodyYAMLKeys(input.node); err == nil || !strings.Contains(err.Error(), input.want) {
				t.Fatalf("body validation error = %v, want substring %q", err, input.want)
			}
		})
	}

	var malformedType exprYAML
	if err := yaml.Unmarshal([]byte("field: []\n"), &malformedType); err == nil {
		t.Fatal("expression with non-string field unexpectedly decoded")
	}
	var legacyScalar exprYAML
	if err := yaml.Unmarshal([]byte("query:\n  from: {name: T}\n"), &legacyScalar); err != nil || legacyScalar.Query == nil || legacyScalar.tugqlQueryBody != nil {
		t.Fatalf("legacy scalar query decode = %+v, error=%v", legacyScalar, err)
	}
	var missingQuery exprYAML
	if err := yaml.Unmarshal([]byte("query:\n  query: null\n"), &missingQuery); err == nil || !strings.Contains(err.Error(), "scalar query body requires query") {
		t.Fatalf("nested body without query error = %v", err)
	}
	var unexpanded exprYAML
	if err := yaml.Unmarshal([]byte("query:\n  definitions:\n    - kind: cte\n      name: Local\n      query:\n        query:\n          from: {name: T}\n  query:\n    from: {name: T}\n"), &unexpanded); err != nil {
		t.Fatal(err)
	}
	if _, err := exprFromYAMLAt(unexpanded, "test.expression"); err == nil || !strings.Contains(err.Error(), "unexpanded TugQL body in scalar query") {
		t.Fatalf("unexpanded scalar definitions error = %v", err)
	}
}

func TestTugQLPublicBoundariesRejectUnsupportedNativeValues(t *testing.T) {
	tree := TugQLTree{
		Format:  "tugqtree",
		Version: 1,
		Query: map[string]any{
			"from":    map[string]any{"name": "T"},
			"columns": []any{map[string]any{"field": "Id"}},
			"runtime": func() {},
		},
	}
	document := TugQLDocument{
		SourceMetadata: TugQLSourceMetadata{Format: "tugql", Version: 1},
		Tree:           &tree,
	}
	if resolved, diagnostics := ResolveTugQL(document, TugQLResolveContext{}); resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "invalid_tree" {
		t.Fatalf("ResolveTugQL accepted unsupported native value: resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
	if _, err := yaml.Marshal(tree); err == nil {
		t.Fatal("yaml.Marshal(TugQLTree) accepted an unsupported native function value")
	}
	wireMarshalCalls = 0
	query := map[string]any{
		"from":    map[string]any{"name": "T"},
		"columns": []any{map[string]any{"field": "Id"}},
		"extra":   map[wireMarshalKey]any{"custom": true},
	}
	tree.Query = query
	if _, err := yaml.Marshal(tree); err == nil || !strings.Contains(err.Error(), "JSON-compatible profile") {
		t.Fatalf("yaml.Marshal(TugQLTree) accepted a custom-marshal map key: %v", err)
	}
	if wireMarshalCalls != 0 {
		t.Fatalf("custom map-key marshaler was called %d times during preflight", wireMarshalCalls)
	}
}

func TestTugQLPublicBoundariesRejectNonFiniteNumbers(t *testing.T) {
	for name, value := range map[string]float64{
		"nan":          math.NaN(),
		"positive_inf": math.Inf(1),
		"negative_inf": math.Inf(-1),
	} {
		t.Run(name, func(t *testing.T) {
			tree := TugQLTree{
				Format:  "tugqtree",
				Version: 1,
				Query: map[string]any{
					"from":    map[string]any{"name": "T"},
					"columns": []any{map[string]any{"field": "Id"}},
					"opaque":  value,
				},
			}
			document := TugQLDocument{
				SourceMetadata: TugQLSourceMetadata{Format: "tugql", Version: 1},
				Tree:           &tree,
			}
			if resolved, diagnostics := ResolveTugQL(document, TugQLResolveContext{}); resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "invalid_tree" {
				t.Fatalf("ResolveTugQL accepted non-finite native number: resolved=%+v diagnostics=%+v", resolved, diagnostics)
			}
			if _, err := yaml.Marshal(tree); err == nil {
				t.Fatal("yaml.Marshal(TugQLTree) accepted a non-finite native number")
			}
		})
	}
}

func TestTugQLPublicSerializersRejectInvalidUTF8Values(t *testing.T) {
	invalidText := string([]byte{0xff})
	query := map[string]any{"from": map[string]any{"name": invalidText}}
	if _, err := (TugQLBody{Query: query}).MarshalYAML(); err == nil {
		t.Fatal("TugQLBody.MarshalYAML accepted invalid UTF-8 query data")
	}
	if _, err := importTugQLTree(TugQLTree{Format: "tugqtree", Version: 1, Query: query}); err == nil {
		t.Fatal("importTugQLTree accepted invalid UTF-8 query data")
	}

	cte := TugQLDefinition{Kind: "cte", Name: "Broken", Query: &TugQLBody{Query: query}}
	tree := TugQLTree{Format: "tugqtree", Version: 1, Query: map[string]any{"from": map[string]any{"name": "T"}}, Definitions: []TugQLDefinition{cte}}
	if _, err := importTugQLTree(tree); err == nil {
		t.Fatal("importTugQLTree accepted invalid UTF-8 inside a CTE")
	}
}

func TestTugQLPublicTreeMarshalBoundsCyclicNativeMaps(t *testing.T) {
	query := map[string]any{
		"from":    map[string]any{"name": "T"},
		"columns": []any{map[string]any{"field": "Id"}},
	}
	query["cycle"] = query
	tree := TugQLTree{Format: "tugqtree", Version: 1, Query: query}
	if _, err := yaml.Marshal(tree); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("cyclic tree marshal error = %v, want bounded depth or node-limit error", err)
	}
	document := TugQLDocument{SourceMetadata: TugQLSourceMetadata{Format: "tugql", Version: 1}, Tree: &tree}
	if resolved, diagnostics := ResolveTugQL(document, TugQLResolveContext{}); resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "document_depth_exceeded" {
		t.Fatalf("ResolveTugQL cycle diagnostic = %+v, resolved=%+v; want bounded depth rejection", diagnostics, resolved.Query)
	}
}

func TestTugQLTreeBudgetTraversesSharedNativeSubtreesByOccurrence(t *testing.T) {
	shared := map[string]any{"leaf": "value"}
	tree := TugQLTree{Format: "tugqtree", Version: 1, Query: map[string]any{
		"from":    map[string]any{"name": "T"},
		"columns": []any{map[string]any{"field": "Id"}},
		"extra":   []any{shared, shared},
	}}
	if err := validateTugQLTreeValues(tree, &tugqlResolveBudget{}); err != nil {
		t.Fatalf("shared DAG must be counted by occurrence without cycle identity rejection: %v", err)
	}
}
