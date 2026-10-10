package dtql

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTugQLFinalBoundaryYAMLDecodeFailureKeepsPriorValues(t *testing.T) {
	originalTree := TugQLTree{
		Format: "tugqtree", Version: 1,
		Query: map[string]any{"from": map[string]any{"name": "Before"}},
	}
	for name, wire := range map[string]string{
		"typed envelope decode":  "format: []\nversion: 1\nquery: {from: {name: After}}\n",
		"nested body validation": "definitions: []\nquery: {from: {name: After}, unknown: true}\n",
	} {
		t.Run(name, func(t *testing.T) {
			tree := originalTree
			tree.Query = map[string]any{"from": map[string]any{"name": "Before"}}
			if name == "nested body validation" {
				body := TugQLBody{
					Definitions: []TugQLDefinition{{Kind: "cte", Name: "Prior", Query: &TugQLBody{Query: map[string]any{"from": map[string]any{"name": "Prior"}}}}},
					Query:       map[string]any{"from": map[string]any{"name": "Before"}},
				}
				if err := yaml.Unmarshal([]byte(wire), &body); err == nil {
					t.Fatalf("malformed query body accepted: %s", wire)
				}
				want := TugQLBody{
					Definitions: []TugQLDefinition{{Kind: "cte", Name: "Prior", Query: &TugQLBody{Query: map[string]any{"from": map[string]any{"name": "Prior"}}}}},
					Query:       map[string]any{"from": map[string]any{"name": "Before"}},
				}
				if !reflect.DeepEqual(body, want) {
					t.Fatalf("failed body decode changed receiver: got %#v want %#v", body, want)
				}
				return
			}
			if err := yaml.Unmarshal([]byte(wire), &tree); err == nil {
				t.Fatalf("malformed tree envelope accepted: %s", wire)
			}
			if !reflect.DeepEqual(tree, originalTree) {
				t.Fatalf("failed tree decode changed receiver: got %#v want %#v", tree, originalTree)
			}
		})
	}
}

func TestTugQLYAMLNodePreflightAllowsCanonicalNearLimit(t *testing.T) {
	columns := make([]string, 1665)
	for i := range columns {
		columns[i] = fmt.Sprintf("    - field: Id\n      as: Column%d", i)
	}
	wire := "format: tugqtree\nversion: 1\nquery:\n  from: {name: T}\n  columns:\n" + strings.Join(columns, "\n") + "\n"
	var tree TugQLTree
	if err := yaml.Unmarshal([]byte(wire), &tree); err != nil {
		t.Fatalf("raw YAML keys and values should fit the staging budget when the semantic tree fits: %v", err)
	}
	if len(tree.Query["columns"].([]any)) != len(columns) {
		t.Fatalf("decoded %d projection items, want %d", len(tree.Query["columns"].([]any)), len(columns))
	}
}

func TestTugQLYAMLNodePreflightBoundsPublicHooks(t *testing.T) {
	tree := TugQLTree{Format: "tugqtree", Version: 1, Query: map[string]any{"from": map[string]any{"name": "Before"}}}
	if err := tree.UnmarshalYAML(nil); err == nil {
		t.Fatal("tree hook accepted a nil YAML node")
	}
	body := TugQLBody{Query: map[string]any{"from": map[string]any{"name": "Before"}}}
	if err := body.UnmarshalYAML(nil); err == nil {
		t.Fatal("body hook accepted a nil YAML node")
	}

	cycle := &yaml.Node{Kind: yaml.MappingNode}
	cycle.Content = []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: "format"}, cycle}
	if err := tree.UnmarshalYAML(cycle); err == nil || !strings.Contains(err.Error(), "cyclic TugQL YAML node") {
		t.Fatalf("tree hook cycle error = %v", err)
	}

	aliasRoot := &yaml.Node{Kind: yaml.SequenceNode}
	alias := &yaml.Node{Kind: yaml.AliasNode, Alias: aliasRoot}
	aliasRoot.Content = []*yaml.Node{alias}
	if err := body.UnmarshalYAML(aliasRoot); err == nil || !strings.Contains(err.Error(), "cyclic TugQL YAML node") {
		t.Fatalf("body hook alias cycle error = %v", err)
	}

	deep := &yaml.Node{Kind: yaml.ScalarNode}
	for i := 0; i < 3*maxTugQLStructuralDepth+1; i++ {
		deep = &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{deep}}
	}
	if err := tree.UnmarshalYAML(deep); err == nil || !strings.Contains(err.Error(), "depth exceeds") {
		t.Fatalf("tree hook depth error = %v", err)
	}

	wide := &yaml.Node{Kind: yaml.SequenceNode, Content: make([]*yaml.Node, 4*maxTugQLSemanticNodes)}
	for i := range wide.Content {
		wide.Content[i] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "x"}
	}
	if err := body.UnmarshalYAML(wide); err == nil || !strings.Contains(err.Error(), "nodes") {
		t.Fatalf("body hook node-count error = %v", err)
	}
}
