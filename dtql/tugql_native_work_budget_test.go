package dtql

import (
	"fmt"
	"testing"
)

func TestTugQLPublicNativeTreeWorkBudget(t *testing.T) {
	base := func() *TugQLTree {
		return &TugQLTree{Format: "tugqtree", Version: 1, Query: map[string]any{"from": map[string]any{"name": "T"}}}
	}
	for _, name := range []string{"parameters", "parameter defaults", "definitions", "definition bodies", "body queries", "import mappings", "typed map", "typed slice", "pointer scalar"} {
		t.Run(name, func(t *testing.T) {
			tree := base()
			want := "document_node_limit"
			switch name {
			case "parameters", "parameter defaults":
				count := 5000
				if name == "parameter defaults" {
					count = 4999
				}
				for i := 0; i < count; i++ {
					parameter := TugQLParameter{Name: fmt.Sprintf("P%d", i), Type: "integer"}
					if name == "parameter defaults" {
						var value any = 0
						parameter.Default = &value
					}
					tree.Parameters = append(tree.Parameters, parameter)
				}
			case "definitions", "definition bodies", "body queries":
				count := 5000
				if name == "definition bodies" {
					count = 4999
				}
				if name == "body queries" {
					count = 4998
				}
				for i := 0; i < count; i++ {
					tree.Definitions = append(tree.Definitions, TugQLDefinition{
						Kind: "cte", Name: fmt.Sprintf("Q%d", i), Query: &TugQLBody{Query: map[string]any{"from": map[string]any{"name": "T"}}},
					})
				}
			case "import mappings":
				definition := TugQLDefinition{Kind: "import", Name: "Saved", Path: "./saved.tql"}
				for i := 0; i < 5000; i++ {
					definition.Using = append(definition.Using, TugQLMapping{Name: fmt.Sprintf("P%d", i), Expression: map[string]any{"value": i}})
				}
				tree.Definitions = []TugQLDefinition{definition}
			case "typed map":
				values := map[string]int{}
				for i := 0; i < 5000; i++ {
					values[fmt.Sprintf("F%d", i)] = i
				}
				tree.Query["unknown"] = values
			case "typed slice":
				tree.Query["unknown"] = make([]int, 5000)
			case "pointer scalar":
				value := 7
				tree.Query["columns"] = []any{map[string]any{"value": &value, "as": "Value"}}
				want = "invalid_tree"
			}
			resolved, diagnostics := ResolveTugQL(tugqlTestDocument(tree), TugQLResolveContext{})
			if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != want {
				t.Fatalf("native input escaped the early work budget: query=%v diagnostics=%+v want=%s", resolved.Query != nil, diagnostics, want)
			}
		})
	}
}
