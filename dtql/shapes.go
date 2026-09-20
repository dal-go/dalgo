package dtql

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// document is the YAML representation of an in-scope dal.StructuredQuery.
// Field order here defines the canonical key order of a DTQL-YAML document.
type document struct {
	From    fromYAML    `yaml:"from"`
	Where   *condYAML   `yaml:"where,omitempty"`
	GroupBy []exprYAML  `yaml:"groupBy,omitempty"`
	Having  *condYAML   `yaml:"having,omitempty"`
	OrderBy []orderYAML `yaml:"orderBy,omitempty"`
	Limit   int         `yaml:"limit,omitempty"`
	Offset  int         `yaml:"offset,omitempty"`
	// Columns is deliberately last: DTQL's SELECT/projection stage remains at
	// the end of the pipeline rather than inheriting SQL's textual order.
	Columns []columnYAML `yaml:"columns,omitempty"`
}

// fromYAML is the YAML representation of the root dal.CollectionRef source.
type fromYAML struct {
	Schema *string `yaml:"schema,omitempty"`
	Name   string  `yaml:"name"`
	Alias  string  `yaml:"alias,omitempty"`
}

func (from *fromYAML) UnmarshalYAML(node *yaml.Node) error {
	if err := validateFromYAMLNode(node); err != nil {
		return err
	}
	type plainFromYAML fromYAML
	var decoded plainFromYAML
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	*from = fromYAML(decoded)
	return nil
}

func validateFromYAMLNode(node *yaml.Node) error {
	return validateFromYAMLNodeWithState(node, map[*yaml.Node]bool{}, map[*yaml.Node]bool{})
}

func validateFromYAMLNodeWithState(node *yaml.Node, visiting, validated map[*yaml.Node]bool) error {
	node = resolvedYAMLAlias(node)
	if node.Kind != yaml.MappingNode {
		return &yaml.TypeError{Errors: []string{"from must be a mapping"}}
	}
	if validated[node] {
		return nil
	}
	if visiting[node] {
		return &yaml.TypeError{Errors: []string{"from merge contains a recursive alias"}}
	}
	visiting[node] = true
	defer delete(visiting, node)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i].Value, node.Content[i+1]
		switch key {
		case "<<":
			if node.Content[i].Tag != "!!merge" {
				return &yaml.TypeError{Errors: []string{"field << not found in from"}}
			}
			if err := validateFromYAMLMerge(value, visiting, validated); err != nil {
				return err
			}
		case "schema":
			value = resolvedYAMLAlias(value)
			if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
				return &yaml.TypeError{Errors: []string{"from.schema must be a string"}}
			}
		case "name", "alias":
		default:
			return &yaml.TypeError{Errors: []string{"field " + key + " not found in from"}}
		}
	}
	validated[node] = true
	return nil
}

func validateFromYAMLMerge(node *yaml.Node, visiting, validated map[*yaml.Node]bool) error {
	node = resolvedYAMLAlias(node)
	switch node.Kind {
	case yaml.MappingNode:
		return validateFromYAMLNodeWithState(node, visiting, validated)
	case yaml.SequenceNode:
		if visiting[node] {
			return &yaml.TypeError{Errors: []string{"from merge contains a recursive alias"}}
		}
		visiting[node] = true
		defer delete(visiting, node)
		for _, merged := range node.Content {
			if err := validateFromYAMLNodeWithState(merged, visiting, validated); err != nil {
				return err
			}
		}
		return nil
	default:
		return &yaml.TypeError{Errors: []string{"from merge value must be a mapping or sequence of mappings"}}
	}
}

func resolvedYAMLAlias(node *yaml.Node) *yaml.Node {
	for node.Kind == yaml.AliasNode && node.Alias != nil && node.Alias != node {
		node = node.Alias
	}
	return node
}

// exprYAML is the YAML representation of an in-scope dal.Expression.
// Exactly one discriminator is set.
type exprYAML struct {
	Field         string         `yaml:"field,omitempty"`
	Source        string         `yaml:"source,omitempty"`
	Value         *any           `yaml:"value,omitempty"`
	Values        any            `yaml:"values,omitempty"`
	Param         string         `yaml:"param,omitempty"`
	Star          bool           `yaml:"star,omitempty"`
	Aggregate     *aggregateYAML `yaml:"aggregate,omitempty"`
	Binary        *binaryYAML    `yaml:"binary,omitempty"`
	sourcePresent bool
}

type aggregateYAML struct {
	Function string     `yaml:"function"`
	Distinct bool       `yaml:"distinct,omitempty"`
	Args     []exprYAML `yaml:"args"`
}

type binaryYAML struct {
	Op    string    `yaml:"op"`
	Left  *exprYAML `yaml:"left"`
	Right *exprYAML `yaml:"right"`
}

func (expression *exprYAML) UnmarshalYAML(node *yaml.Node) error {
	return decodeExpressionNode(node, expression, nil)
}

func (expression exprYAML) MarshalYAML() (any, error) {
	return encodeExpressionNode(expression, nil)
}

// columnYAML is the YAML representation of a dal.Column.
type columnYAML struct {
	exprYAML           `yaml:",inline"`
	As                 string        `yaml:"as,omitempty"` // dal.Column.Alias
	Wildcard           *wildcardYAML `yaml:"wildcard,omitempty"`
	asPresent          bool
	expressionKeyCount int
}

// wildcardYAML is a column-set projection: all columns from the optional
// source except the explicitly named columns.
type wildcardYAML struct {
	Source  string   `yaml:"source,omitempty"`
	Exclude []string `yaml:"exclude"`
}

func (column *columnYAML) UnmarshalYAML(node *yaml.Node) error {
	extra := map[string]func(*yaml.Node) error{
		"as": func(value *yaml.Node) error {
			column.asPresent = true
			return value.Decode(&column.As)
		},
		"wildcard": func(value *yaml.Node) error {
			if value.Kind != yaml.MappingNode {
				return &yaml.TypeError{Errors: []string{"column wildcard must be a mapping"}}
			}
			for i := 0; i+1 < len(value.Content); i += 2 {
				switch value.Content[i].Value {
				case "source":
					if value.Content[i+1].Kind != yaml.ScalarNode || value.Content[i+1].Tag != "!!str" || value.Content[i+1].Value == "" {
						return &yaml.TypeError{Errors: []string{"column wildcard source must be a non-empty string"}}
					}
				case "exclude":
				default:
					return &yaml.TypeError{Errors: []string{"field " + value.Content[i].Value + " not found in column wildcard"}}
				}
			}
			var wildcard wildcardYAML
			if err := value.Decode(&wildcard); err != nil {
				return err
			}
			column.Wildcard = &wildcard
			return nil
		},
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		switch node.Content[i].Value {
		case "field", "value", "values", "param", "star", "aggregate", "binary":
			column.expressionKeyCount++
		}
	}
	return decodeExpressionNode(node, &column.exprYAML, extra)
}

func (column columnYAML) MarshalYAML() (any, error) {
	extra := []yaml.Node{}
	if column.Wildcard != nil {
		var encoded yaml.Node
		// wildcardYAML contains only strings and string slices, so it is always
		// representable by yaml.Node.
		_ = encoded.Encode(column.Wildcard)
		extra = append(extra, yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "wildcard"}, encoded)
	}
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
	case expression.Star:
		key, value = "star", true
	case expression.Aggregate != nil:
		key, value = "aggregate", expression.Aggregate
	case expression.Binary != nil:
		key, value = "binary", expression.Binary
	}
	if key != "" {
		var encoded yaml.Node
		if err := encodeYAMLNode(&encoded, value); err != nil {
			return nil, err
		}
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &encoded)
		if key == "field" && expression.Source != "" {
			node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "source"}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: expression.Source})
		}
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
		case "source":
			expression.sourcePresent = true
			if value.Kind != yaml.ScalarNode || value.Tag != "!!str" || value.Value == "" {
				return &yaml.TypeError{Errors: []string{"expression source must be a non-empty string"}}
			}
			err = value.Decode(&expression.Source)
		case "value":
			var decoded any
			err = value.Decode(&decoded)
			expression.Value = &decoded
		case "values":
			err = value.Decode(&expression.Values)
		case "param":
			err = value.Decode(&expression.Param)
		case "star":
			err = value.Decode(&expression.Star)
		case "aggregate":
			if err = validateExpressionObjectKeys(value, "aggregate", map[string]bool{"function": true, "distinct": true, "args": true}); err != nil {
				break
			}
			var decoded aggregateYAML
			err = value.Decode(&decoded)
			expression.Aggregate = &decoded
		case "binary":
			if err = validateExpressionObjectKeys(value, "binary", map[string]bool{"op": true, "left": true, "right": true}); err != nil {
				break
			}
			var decoded binaryYAML
			err = value.Decode(&decoded)
			expression.Binary = &decoded
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

func validateExpressionObjectKeys(node *yaml.Node, label string, allowed map[string]bool) error {
	if node.Kind != yaml.MappingNode {
		return &yaml.TypeError{Errors: []string{label + " must be a mapping"}}
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if !allowed[node.Content[i].Value] {
			return &yaml.TypeError{Errors: []string{"field " + node.Content[i].Value + " not found in " + label}}
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
