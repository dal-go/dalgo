package dtql

import (
	"fmt"

	"github.com/dal-go/dalgo/dal"
	"gopkg.in/yaml.v3"
)

func algorithmsFromYAML(node *yaml.Node, joinPath string) ([]dal.JoinAlgorithm, error) {
	if node == nil {
		return nil, nil
	}
	path := joinPath + ".hints.algorithms"
	var err error
	node, err = resolveJoinHintAlias(node, path)
	if err != nil {
		return nil, err
	}
	if node.Kind != yaml.MappingNode {
		return nil, &dal.JoinValidationError{Category: "join_algorithm", Path: path, Message: "hints must be a mapping"}
	}
	var list *yaml.Node
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value != "algorithms" || list != nil {
			return nil, &dal.JoinValidationError{Category: "join_algorithm", Path: path, Message: "hints must contain exactly one algorithms list"}
		}
		list, err = resolveJoinHintAlias(node.Content[i+1], path)
		if err != nil {
			return nil, err
		}
	}
	if list == nil || list.Kind != yaml.SequenceNode || len(list.Content) == 0 {
		return nil, &dal.JoinValidationError{Category: "join_algorithm", Path: path, Message: "algorithms must be a nonempty sequence"}
	}
	algorithms := make([]dal.JoinAlgorithm, len(list.Content))
	for i, item := range list.Content {
		item, err = resolveJoinHintAlias(item, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			return nil, err
		}
		if item.Kind != yaml.ScalarNode || item.Tag != "!!str" || item.Value == "" {
			return nil, &dal.JoinValidationError{Category: "join_algorithm", Path: fmt.Sprintf("%s[%d]", path, i), Message: "algorithm must be a nonempty string"}
		}
		algorithms[i] = dal.JoinAlgorithm(item.Value)
	}
	return algorithms, nil
}

func resolveJoinHintAlias(node *yaml.Node, path string) (*yaml.Node, error) {
	seen := map[*yaml.Node]bool{}
	for node.Kind == yaml.AliasNode {
		if seen[node] || node.Alias == nil {
			return nil, &dal.JoinValidationError{Category: "join_algorithm", Path: path, Message: "recursive or unresolved hint alias"}
		}
		seen[node] = true
		node = node.Alias
	}
	return node, nil
}

func algorithmsToYAML(algorithms []dal.JoinAlgorithm) *yaml.Node {
	if algorithms == nil {
		return nil
	}
	items := make([]*yaml.Node, len(algorithms))
	for i, algorithm := range algorithms {
		items[i] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: string(algorithm)}
	}
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "algorithms"},
		{Kind: yaml.SequenceNode, Tag: "!!seq", Content: items},
	}}
}
