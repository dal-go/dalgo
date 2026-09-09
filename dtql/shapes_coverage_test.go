package dtql

import (
	"math"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPortableScalarBoundaries(t *testing.T) {
	for _, value := range []any{nil, "x", true, float32(1.5), float64(1.5), int8(1), uint8(1)} {
		if !portableScalar(value) {
			t.Fatalf("portable scalar rejected: %T", value)
		}
	}
	for _, value := range []any{float32(math.NaN()), math.Inf(1), uint64(1 << 54), make(chan int)} {
		if portableScalar(value) {
			t.Fatalf("non-portable scalar accepted: %T", value)
		}
	}
	if portableScalarArray(nil) || portableScalarArray("x") || portableScalarArray([]any{make(chan int)}) {
		t.Fatal("non-portable array accepted")
	}
	if !portableScalarArray([2]int{1, 2}) {
		t.Fatal("portable array rejected")
	}
}

func TestExpressionNodeHelpersRejectInvalidShapes(t *testing.T) {
	bad := any(make(chan int))
	if _, err := encodeExpressionNode(exprYAML{Value: &bad}, nil); err == nil {
		t.Fatal("unencodable value accepted")
	}
	var expression exprYAML
	if err := decodeExpressionNode(&yaml.Node{Kind: yaml.ScalarNode}, &expression, nil); err == nil {
		t.Fatal("scalar expression accepted")
	}
	unknown := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Value: "unknown"}, {Kind: yaml.ScalarNode, Value: "x"}}}
	if err := decodeExpressionNode(unknown, &expression, map[string]func(*yaml.Node) error{}); err == nil {
		t.Fatal("unknown expression key accepted")
	}
	wrongField := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Value: "field"}, {Kind: yaml.SequenceNode}}}
	if err := decodeExpressionNode(wrongField, &expression, nil); err == nil {
		t.Fatal("non-scalar field accepted")
	}
}
