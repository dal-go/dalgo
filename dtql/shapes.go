package dtql

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// document is the YAML representation of an in-scope dal.StructuredQuery.
// Field order here defines the canonical key order of a DTQL-YAML document.
type document struct {
	From    fromYAML     `yaml:"from"`
	Columns []columnYAML `yaml:"columns,omitempty"`
	Where   *condYAML    `yaml:"where,omitempty"`
	OrderBy []orderYAML  `yaml:"orderBy,omitempty"`
	Limit   int          `yaml:"limit,omitempty"`
	Offset  int          `yaml:"offset,omitempty"`
}

// fromYAML is the YAML representation of the root dal.CollectionRef source.
type fromYAML struct {
	Name  string `yaml:"name"`
	Alias string `yaml:"alias,omitempty"`
}

// exprYAML is the YAML representation of an in-scope dal.Expression.
// Exactly one of Field / Value / Values / Param is set, which discriminates a
// FieldRef, a Constant, an Array or a Param respectively.
type exprYAML struct {
	Field  string `yaml:"field,omitempty"`  // dal.FieldRef
	Value  *any   `yaml:"value,omitempty"`  // dal.Constant (inline scalar, including null)
	Values any    `yaml:"values,omitempty"` // dal.Array (inline sequence)
	Param  string `yaml:"param,omitempty"`  // dal.Param (runtime parameter, "$name")
}

func (expression *exprYAML) UnmarshalYAML(node *yaml.Node) error {
	return decodeExpressionNode(node, expression, nil)
}

func (expression exprYAML) MarshalYAML() (any, error) {
	return encodeExpressionNode(expression, nil)
}

// columnYAML is the YAML representation of a dal.Column.
type columnYAML struct {
	exprYAML `yaml:",inline"`
	As       string `yaml:"as,omitempty"` // dal.Column.Alias
}

func (column *columnYAML) UnmarshalYAML(node *yaml.Node) error {
	extra := map[string]func(*yaml.Node) error{
		"as": func(value *yaml.Node) error { return value.Decode(&column.As) },
	}
	return decodeExpressionNode(node, &column.exprYAML, extra)
}

func (column columnYAML) MarshalYAML() (any, error) {
	extra := []yaml.Node{}
	if column.As != "" {
		extra = append(extra, yaml.Node{Kind: yaml.ScalarNode, Value: "as"}, yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: column.As})
	}
	return encodeExpressionNode(column.exprYAML, extra)
}

// orderYAML is the YAML representation of a dal.OrderExpression.
type orderYAML struct {
	exprYAML `yaml:",inline"`
	Desc     bool `yaml:"desc,omitempty"` // descending order
}

func (order *orderYAML) UnmarshalYAML(node *yaml.Node) error {
	extra := map[string]func(*yaml.Node) error{
		"desc": func(value *yaml.Node) error { return value.Decode(&order.Desc) },
	}
	return decodeExpressionNode(node, &order.exprYAML, extra)
}

func (order orderYAML) MarshalYAML() (any, error) {
	extra := []yaml.Node{}
	if order.Desc {
		extra = append(extra, yaml.Node{Kind: yaml.ScalarNode, Value: "desc"}, yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
	}
	return encodeExpressionNode(order.exprYAML, extra)
}

func encodeExpressionNode(expression exprYAML, extra []yaml.Node) (*yaml.Node, error) {
	node := &yaml.Node{Kind: yaml.MappingNode}
	var key string
	var value any
	switch {
	case expression.Field != "":
		key, value = "field", expression.Field
	case expression.Value != nil:
		key, value = "value", *expression.Value
	case expression.Values != nil:
		key, value = "values", expression.Values
	case expression.Param != "":
		key, value = "param", expression.Param
	}
	if key != "" {
		var encoded yaml.Node
		if err := encodeYAMLNode(&encoded, value); err != nil {
			return nil, err
		}
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &encoded)
	}
	for i := range extra {
		node.Content = append(node.Content, &extra[i])
	}
	return node, nil
}

func encodeYAMLNode(node *yaml.Node, value any) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("encode expression: %v", recovered)
		}
	}()
	return node.Encode(value)
}

func decodeExpressionNode(node *yaml.Node, expression *exprYAML, extra map[string]func(*yaml.Node) error) error {
	if node.Kind != yaml.MappingNode {
		return &yaml.TypeError{Errors: []string{"expression must be a mapping"}}
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i].Value, node.Content[i+1]
		var err error
		switch key {
		case "field":
			err = value.Decode(&expression.Field)
		case "value":
			var decoded any
			err = value.Decode(&decoded)
			expression.Value = &decoded
		case "values":
			err = value.Decode(&expression.Values)
		case "param":
			err = value.Decode(&expression.Param)
		default:
			if decode := extra[key]; decode != nil {
				err = decode(value)
			} else {
				return &yaml.TypeError{Errors: []string{"field " + key + " not found in expression"}}
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// condYAML is the YAML representation of a dal.Condition.
// A Comparison sets Op/Left/Right; a GroupCondition sets And or Or.
type condYAML struct {
	Op    string     `yaml:"op,omitempty"`    // dal.Comparison.Operator
	Left  *exprYAML  `yaml:"left,omitempty"`  // dal.Comparison.Left
	Right *exprYAML  `yaml:"right,omitempty"` // dal.Comparison.Right
	And   []condYAML `yaml:"and,omitempty"`   // dal.GroupCondition (And)
	Or    []condYAML `yaml:"or,omitempty"`    // dal.GroupCondition (Or)
}
