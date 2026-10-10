package dtql

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTugQLPublicYAMLNodeHooksRejectMalformedEmitterInput(t *testing.T) {
	text := func(value string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value} }
	mapping := func(values ...*yaml.Node) *yaml.Node {
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: values}
	}
	for _, name := range []string{"invalid query node kind", "invalid query scalar UTF8", "invalid mapping expression node kind", "invalid mapping expression scalar UTF8"} {
		t.Run(name, func(t *testing.T) {
			invalid := &yaml.Node{Kind: yaml.Kind(42)}
			if name == "invalid query scalar UTF8" || name == "invalid mapping expression scalar UTF8" {
				invalid = text(string([]byte{0xff}))
			}
			query := mapping(text("from"), mapping(text("name"), text("T")))
			var definitions *yaml.Node
			if name == "invalid mapping expression node kind" || name == "invalid mapping expression scalar UTF8" {
				definitions = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{
					mapping(text("kind"), text("import"), text("name"), text("Saved"), text("path"), text("./saved.tql"), text("using"),
						&yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{mapping(text("name"), text("P"), text("expression"), mapping(text("value"), invalid))}}),
				}}
			} else {
				query = mapping(text("from"), mapping(text("name"), invalid))
			}
			treeNode := mapping(text("format"), text("tugqtree"), text("version"), &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "1"}, text("query"), query)
			bodyNode := mapping(text("query"), query)
			if definitions != nil {
				treeNode.Content = append(treeNode.Content, text("definitions"), definitions)
				bodyNode.Content = append(bodyNode.Content, text("definitions"), definitions)
			}
			before := TugQLTree{Format: "tugqtree", Version: 1, Query: map[string]any{"from": map[string]any{"name": "Before"}}}
			tree := before
			if err := tree.UnmarshalYAML(treeNode); err == nil {
				t.Fatal("malformed caller YAML node accepted by tree hook")
			}
			if !reflect.DeepEqual(tree, before) {
				t.Fatal("failed YAML tree hook changed receiver")
			}
			bodyBefore := TugQLBody{Query: before.Query}
			body := bodyBefore
			if err := body.UnmarshalYAML(bodyNode); err == nil {
				t.Fatal("malformed caller YAML node accepted by body hook")
			}
			if !reflect.DeepEqual(body, bodyBefore) {
				t.Fatal("failed YAML body hook changed receiver")
			}
		})
	}
}

func TestTugQLBodyUnmarshalYAMLRejectsSequenceMappingKey(t *testing.T) {
	text := func(value string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value} }
	mapping := func(values ...*yaml.Node) *yaml.Node {
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: values}
	}
	query := mapping(text("from"), mapping(text("name"), text("Invoice")))
	definitions := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	sequenceKey := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Value: "definitions"}
	root := mapping(sequenceKey, definitions, text("query"), query)

	before := TugQLBody{Query: map[string]any{"from": map[string]any{"name": "Before"}}}
	body := before
	if err := body.UnmarshalYAML(root); err == nil {
		t.Fatal("sequence mapping key unexpectedly decoded into TugQLBody")
	}
	if !reflect.DeepEqual(body, before) {
		t.Fatal("failed YAML body hook changed receiver")
	}
}
