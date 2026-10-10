package dtql

import (
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTugQLMoreRejectsMalformedUsingWireAndDuplicateNestedKeys(t *testing.T) {
	prefix := "format: tugqtree\nversion: 1\ndefinitions: [{kind: import, name: Saved, path: saved, using: ["
	suffix := "]}]\nquery: {from: {name: Saved}}\n"
	for _, tc := range []struct{ name, mapping string }{
		{"using name empty", "{name: '', expression: {field: Id}}"},
		{"using name wrong type", "{name: 7, expression: {field: Id}}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var tree TugQLTree
			if err := yaml.Unmarshal([]byte(prefix+tc.mapping+suffix), &tree); err == nil {
				t.Fatalf("malformed using mapping was accepted: %s", tc.mapping)
			}
		})
	}

	for _, tc := range []struct{ name, query string }{
		{"duplicate query key", "{from: {name: T}, from: {name: U}}"},
		{"duplicate call field", "{from: {name: T}, columns: [{call: {function: F, function: G, args: []}}]}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var tree TugQLTree
			wire := "format: tugqtree\nversion: 1\nquery: " + tc.query + "\n"
			if err := yaml.Unmarshal([]byte(wire), &tree); err == nil {
				t.Fatalf("duplicate nested key was accepted: %s", tc.query)
			}
		})
	}
}

func TestTugQLMoreRejectsMalformedNestedConditionAndSourceShapes(t *testing.T) {
	prefix := "format: tugqtree\nversion: 1\nquery: "
	for _, tc := range []struct{ name, query string }{
		{"empty condition", "{from: {name: T}, where: {}}"},
		{"comparison has malformed left", "{from: {name: T}, where: {op: '=', left: {field: A, value: 1}, right: {value: 2}}}"},
		{"isNull has malformed expression", "{from: {name: T}, where: {isNull: {source: s, value: 1}}}"},
		{"isNotNull has malformed expression", "{from: {name: T}, where: {isNotNull: {field: A, value: 1}}}"},
		{"groupBy has malformed expression", "{from: {name: T}, groupBy: [{binary: {op: '+', left: {field: A}}}]}"},
		{"nested from query missing source", "{from: {query: {where: {isNull: {field: A}}}}}"},
		{"nested join query missing source", "{from: {name: T, joins: [{from: {query: {where: {isNull: {field: A}}}}}]}}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var tree TugQLTree
			if err := yaml.Unmarshal([]byte(prefix+tc.query+"\n"), &tree); err == nil {
				t.Fatalf("malformed query condition/source was accepted: %s", tc.query)
			}
		})
	}
}

func TestTugQLMoreAcceptsNestedFromJoinAndConditionForms(t *testing.T) {
	wire := `format: tugqtree
version: 1
query:
  from:
    query: {from: {name: T}}
    joins:
      - from:
          query: {from: {name: U}}
        on:
          - {op: "=", left: {field: Id}, right: {field: Id}}
  where:
    and:
      - {isNull: {field: A}}
      - {notExists: {query: {from: {name: Hidden}}}}
  having: {isNotNull: {field: A}}
  groupBy: [{field: A}]
  orderBy: [{field: A, desc: true}]
`
	var tree TugQLTree
	if err := yaml.Unmarshal([]byte(wire), &tree); err != nil {
		t.Fatalf("valid nested from/join/condition forms rejected: %v", err)
	}
	if _, err := yaml.Marshal(tree); err != nil {
		t.Fatalf("valid nested from/join/condition forms failed marshal: %v", err)
	}
}

func TestTugQLMoreValidatesBodyYAMLMethodsAndNilReceiver(t *testing.T) {
	var validNode yaml.Node
	if err := yaml.Unmarshal([]byte("query: {from: {name: T}}\n"), &validNode); err != nil {
		t.Fatal(err)
	}
	var nilBody *TugQLBody
	if err := nilBody.UnmarshalYAML(validNode.Content[0]); err == nil {
		t.Fatal("nil TugQLBody receiver was accepted")
	}

	cases := []struct {
		name string
		body TugQLBody
	}{
		{"missing query", TugQLBody{}},
		{"unknown query key", TugQLBody{Query: map[string]any{"from": map[string]any{"name": "T"}, "mystery": true}}},
		{"invalid nested definition", TugQLBody{Query: map[string]any{"from": map[string]any{"name": "T"}}, Definitions: []TugQLDefinition{{Kind: "import", Name: "Saved"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := yaml.Marshal(tc.body); err == nil {
				t.Fatalf("invalid query body was marshaled: %+v", tc.body)
			}
		})
	}
	for _, badBody := range []string{
		"unknown: true\nquery: {from: {name: T}}\n",
		"definitions: {}\nquery: {from: {name: T}}\n",
		"definitions: [nope]\nquery: {from: {name: T}}\n",
		"query: {where: {isNull: {field: A}}}\n",
	} {
		t.Run("unmarshal rejects "+strings.Split(badBody, "\n")[0], func(t *testing.T) {
			var body TugQLBody
			if err := yaml.Unmarshal([]byte(badBody), &body); err == nil {
				t.Fatalf("malformed query body was accepted: %s", badBody)
			}
		})
	}
}

func TestTugQLMoreRejectsWrongTypedCallerParametersAndDefinitions(t *testing.T) {
	base := func() TugQLTree {
		return TugQLTree{Format: "tugqtree", Version: 1, Query: map[string]any{"from": map[string]any{"name": "T"}}}
	}
	cases := []struct {
		name   string
		mutate func(*TugQLTree)
	}{
		{"unsupported parameter type", func(tree *TugQLTree) { tree.Parameters = []TugQLParameter{{Name: "P", Type: "geography"}} }},
		{"integer default has string value", func(tree *TugQLTree) {
			value := any("12")
			tree.Parameters = []TugQLParameter{{Name: "P", Type: "integer", Default: &value}}
		}},
		{"decimal default has boolean value", func(tree *TugQLTree) {
			value := any(true)
			tree.Parameters = []TugQLParameter{{Name: "P", Type: "decimal", Default: &value}}
		}},
		{"string default has integer value", func(tree *TugQLTree) {
			value := any(12)
			tree.Parameters = []TugQLParameter{{Name: "P", Type: "string", Default: &value}}
		}},
		{"definition name missing", func(tree *TugQLTree) { tree.Definitions = []TugQLDefinition{{Kind: "import", Path: "i"}} }},
		{"cte query body missing query map", func(tree *TugQLTree) {
			tree.Definitions = []TugQLDefinition{{Kind: "cte", Name: "C", Query: &TugQLBody{}}}
		}},
		{"cte cannot have path", func(tree *TugQLTree) {
			tree.Definitions = []TugQLDefinition{{Kind: "cte", Name: "C", Path: "c", Query: &TugQLBody{Query: map[string]any{"from": map[string]any{"name": "T"}}}}}
		}},
		{"cte cannot have using", func(tree *TugQLTree) {
			tree.Definitions = []TugQLDefinition{{Kind: "cte", Name: "C", Using: []TugQLMapping{{Name: "P", Expression: map[string]any{"field": "Id"}}}, Query: &TugQLBody{Query: map[string]any{"from": map[string]any{"name": "T"}}}}}
		}},
		{"import cannot have query", func(tree *TugQLTree) {
			tree.Definitions = []TugQLDefinition{{Kind: "import", Name: "I", Path: "i", Query: &TugQLBody{Query: map[string]any{"from": map[string]any{"name": "T"}}}}}
		}},
		{"import using name empty", func(tree *TugQLTree) {
			tree.Definitions = []TugQLDefinition{{Kind: "import", Name: "I", Path: "i", Using: []TugQLMapping{{Expression: map[string]any{"field": "Id"}}}}}
		}},
		{"cte body query contains unknown shape", func(tree *TugQLTree) {
			tree.Definitions = []TugQLDefinition{{Kind: "cte", Name: "C", Query: &TugQLBody{Query: map[string]any{"from": map[string]any{"name": "T"}, "mystery": true}}}}
		}},
		{"cte body has incomplete source shape", func(tree *TugQLTree) {
			tree.Definitions = []TugQLDefinition{{Kind: "cte", Name: "C", Query: &TugQLBody{Query: map[string]any{"from": map[string]any{}}}}}
		}},
		{"cte contains invalid nested definition", func(tree *TugQLTree) {
			tree.Definitions = []TugQLDefinition{{Kind: "cte", Name: "C", Query: &TugQLBody{Query: map[string]any{"from": map[string]any{"name": "T"}}, Definitions: []TugQLDefinition{{Kind: "unknown", Name: "Nested"}}}}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree := base()
			tc.mutate(&tree)
			if _, err := yaml.Marshal(tree); err == nil {
				t.Fatalf("malformed caller value was marshaled: %+v", tree)
			}
		})
	}
}

func TestTugQLMoreAcceptsNestedScalarQueryDefinitions(t *testing.T) {
	wire := `format: tugqtree
version: 1
definitions:
  - kind: import
    name: External
    path: saved.tugql
query:
  from: {name: Invoice}
  columns:
    - query:
        definitions:
          - kind: cte
            name: Inner
            query:
              query: {from: {name: Customer}}
        query: {from: {name: Inner}}
`
	var tree TugQLTree
	if err := yaml.Unmarshal([]byte(wire), &tree); err != nil {
		t.Fatalf("valid scalar query body rejected: %v", err)
	}
	data, err := yaml.Marshal(tree)
	if err != nil {
		t.Fatalf("marshal scalar query body: %v", err)
	}
	var roundTrip TugQLTree
	if err := yaml.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("round-trip scalar query body: %v\n%s", err, data)
	}
	if !strings.Contains(string(data), "name: Inner") || !strings.Contains(string(data), "name: Customer") {
		t.Fatalf("scalar query body definitions were lost: %s", data)
	}
	if len(roundTrip.Definitions) != 1 || roundTrip.Definitions[0].Name != "External" {
		t.Fatalf("import without using mappings was lost: %+v", roundTrip.Definitions)
	}
}

func TestTugQLMoreDirectCallValidatorBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, wire string }{
		{"not a mapping", "Fn"},
		{"unknown call key", "{function: Fn, args: [], extra: 1}"},
		{"duplicate call key", "{function: Fn, function: Other, args: []}"},
		{"missing function", "{args: []}"},
		{"missing arguments", "{function: Fn}"},
		{"empty function", "{function: '', args: []}"},
		{"non string function", "{function: 7, args: []}"},
		{"arguments are not a sequence", "{function: Fn, args: Id}"},
		{"malformed argument", "{function: Fn, args: [{field: Id, value: 1}]}"},
		{"valid call", "{function: Fn, args: [{field: Id}]}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := yamlNode(t, tc.wire)
			err := validateTugQLCallNode(node)
			if tc.name == "valid call" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatalf("invalid call node was accepted: %s", tc.wire)
			}
		})
	}
}

func TestTugQLMoreDirectExpressionValidatorBoundaries(t *testing.T) {
	if err := validateTugQLExpressionNode(nil); err == nil {
		t.Fatal("nil expression node was accepted")
	}
	for _, tc := range []struct{ name, wire string }{
		{"not a mapping", "17"},
		{"empty expression", "{}"},
		{"multiple forms", "{field: Id, value: 1}"},
		{"unknown form", "{mystery: true}"},
		{"source without field", "{source: i}"},
		{"binary missing member", "{binary: {op: '+', left: {field: A}}}"},
		{"binary invalid left", "{binary: {op: '+', left: {field: A, value: 1}, right: {value: 2}}}"},
		{"binary invalid right", "{binary: {op: '+', left: {field: A}, right: {field: B, value: 2}}}"},
		{"aggregate missing function", "{aggregate: {args: []}}"},
		{"aggregate invalid argument", "{aggregate: {function: SUM, args: [{field: A, value: 1}]}}"},
		{"invalid nested call during lowering", "{call: {function: Fn, args: [{field: A, value: 1}]}}"},
		{"query has malformed body", "{query: {where: {isNull: {field: A}}}}"},
		{"valid field", "{field: Id}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTugQLExpressionNode(yamlNode(t, tc.wire))
			if tc.name == "valid field" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatalf("invalid expression node was accepted: %s", tc.wire)
			}
		})
	}

	for _, expression := range []exprYAML{
		{tugqlCall: &tugqlCallYAML{}},
		{tugqlCall: &tugqlCallYAML{Function: "Fn", Args: []exprYAML{{Field: "Id", Value: valuePointer(any(1))}}}},
	} {
		if err := validateTugQLExpressionShape(expression); err == nil {
			t.Fatalf("malformed internal call expression was accepted: %+v", expression)
		}
	}
}

func TestTugQLMoreCallLoweringClonesInput(t *testing.T) {
	original := yamlNode(t, "{call: {function: Fn, args: [{field: Id}]}}")
	copy := cloneTugQLYAMLNode(original)
	if err := replaceTugQLCalls(copy); err != nil {
		t.Fatal(err)
	}
	originalText, err := yaml.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	copyText, err := yaml.Marshal(copy)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(originalText), "call:") || !strings.Contains(string(copyText), "value: null") {
		t.Fatalf("lowering changed original or missed copy: original=%s copy=%s", originalText, copyText)
	}
	if cloneTugQLYAMLNode(nil) != nil {
		t.Fatal("nil YAML node clone should remain nil")
	}
}

func yamlNode(t *testing.T, wire string) *yaml.Node {
	t.Helper()
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(wire), &root); err != nil {
		t.Fatalf("decode test node %q: %v", wire, err)
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) != 1 {
		t.Fatalf("unexpected test node root: %#v", root)
	}
	return root.Content[0]
}

func valuePointer(value any) *any { return &value }

func TestTugQLMoreTreeNilReceiverAndNestedScalarShape(t *testing.T) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("format: tugqtree\nversion: 1\nquery: {from: {name: T}}\n"), &node); err != nil {
		t.Fatal(err)
	}
	var nilTree *TugQLTree
	if err := nilTree.UnmarshalYAML(node.Content[0]); err == nil {
		t.Fatal("nil TugQLTree receiver was accepted")
	}

	for _, query := range []string{
		"{from: {name: T}, columns: [{query: {where: {isNull: {field: A}}}}]}",
		"{from: {name: T}, columns: [{aggregate: {args: []}}]}",
		"{from: {name: T}, columns: [{binary: {op: '+', left: {field: A}, right: {binary: {op: '*', left: {field: B}}}}}]}",
	} {
		var tree TugQLTree
		if err := yaml.Unmarshal([]byte("format: tugqtree\nversion: 1\nquery: "+query+"\n"), &tree); err == nil {
			t.Fatalf("malformed scalar/expression shape was accepted: %s", query)
		}
	}
}

func TestTugQLMoreDecodeKnownYAMLRejectsMultipleDocuments(t *testing.T) {
	var decoded map[string]any
	if err := decodeKnownYAML([]byte("a: 1\n---\nb: 2\n"), &decoded); err == nil || !strings.Contains(err.Error(), "multiple YAML documents") {
		t.Fatalf("multiple YAML documents error=%v", err)
	}
}

func TestTugQLMoreDirectMalformedNodeInvariants(t *testing.T) {
	if err := replaceTugQLCalls(nil); err != nil {
		t.Fatalf("nil optional YAML node should be a no-op: %v", err)
	}
	if err := replaceTugQLCalls(&yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: "key"}}}); err == nil {
		t.Fatal("odd-length mapping content was accepted")
	}
	duplicate := yamlNode(t, "{outer: {item: one, item: two}}")
	if err := replaceTugQLCalls(duplicate); err == nil || !strings.Contains(err.Error(), "duplicate TugQTree key") {
		t.Fatalf("duplicate nested key error=%v", err)
	}

	if err := validateTugQLQueryNode(yamlNode(t, "not-a-query")); err == nil {
		t.Fatal("scalar query node was accepted")
	}
	if tugQLMappingValue(nil, "query") != nil || tugQLMappingValue(yamlNode(t, "scalar"), "query") != nil {
		t.Fatal("mapping lookup should return nil for absent and non-mapping nodes")
	}
}

func TestTugQLMoreDirectInternalDefinitionInvariants(t *testing.T) {
	validDoc := document{From: fromYAML{Name: "T"}}
	validCTE := tugqlDefinition{Kind: "cte", Name: "C", Query: &tugqlBody{Query: validDoc}}
	validImport := tugqlDefinition{Kind: "import", Name: "I", Path: "i.tugql"}
	cases := []struct {
		name       string
		definition tugqlDefinition
		wantErr    bool
	}{
		{"missing name", tugqlDefinition{Kind: "import", Path: "i.tugql"}, true},
		{"cte missing query", tugqlDefinition{Kind: "cte", Name: "C"}, true},
		{"cte with path", tugqlDefinition{Kind: "cte", Name: "C", Path: "c", Query: &tugqlBody{Query: validDoc}}, true},
		{"cte invalid query shape", tugqlDefinition{Kind: "cte", Name: "C", Query: &tugqlBody{}}, true},
		{"cte invalid nested definition", tugqlDefinition{Kind: "cte", Name: "C", Query: &tugqlBody{Query: validDoc, Definitions: []tugqlDefinition{{Kind: "import", Name: "Nested"}}}}, true},
		{"import missing path", tugqlDefinition{Kind: "import", Name: "I"}, true},
		{"import with query", tugqlDefinition{Kind: "import", Name: "I", Path: "i.tugql", Query: &tugqlBody{Query: validDoc}}, true},
		{"unknown kind", tugqlDefinition{Kind: "other", Name: "X"}, true},
		{"valid cte", validCTE, false},
		{"valid import", validImport, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTugQLInternalDefinition(tc.definition)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateTugQLInternalDefinition(%+v) error=%v, wantErr=%v", tc.definition, err, tc.wantErr)
			}
		})
	}

	queryWithBadDefinition := exprYAML{
		Query:          &validDoc,
		tugqlQueryBody: &tugqlBody{Query: validDoc, Definitions: []tugqlDefinition{{Kind: "import", Name: "Nested"}}},
	}
	if err := validateTugQLExpressionShape(queryWithBadDefinition); err == nil || !strings.Contains(err.Error(), "query definitions") {
		t.Fatalf("nested query definition error=%v", err)
	}
}

func TestTugQLMoreRejectsCustomYAMLMarshalersAtCallerValueBoundaries(t *testing.T) {
	wireMarshalCalls = 0
	query := map[string]any{"from": map[string]any{"name": wireMarshalError{}}}
	cases := []struct {
		name string
		run  func() error
	}{
		{"body marshal", func() error { _, err := (TugQLBody{Query: query}).MarshalYAML(); return err }},
		{"tree yaml marshal", func() error {
			_, err := (TugQLTree{Format: "tugqtree", Version: 1, Query: query}).MarshalYAML()
			return err
		}},
		{"tree import", func() error {
			_, err := importTugQLTree(TugQLTree{Format: "tugqtree", Version: 1, Query: query})
			return err
		}},
		{"body import", func() error {
			_, err := importTugQLBody(TugQLBody{Query: query})
			return err
		}},
		{"expression import", func() error {
			_, err := importExpression(map[string]any{"value": wireMarshalError{}})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil || !strings.Contains(err.Error(), "JSON-compatible profile") {
				t.Fatalf("custom YAML marshaler was not rejected by preflight: %v", err)
			}
		})
	}
	if wireMarshalCalls != 0 {
		t.Fatalf("custom YAML marshaler was called %d times during validation", wireMarshalCalls)
	}

	if err := validateTugQLQueryYAML([]byte{0xff}); err == nil {
		t.Fatal("invalid UTF-8 YAML query bytes were accepted")
	}
}

type wireMarshalError struct{}

type wireMarshalKey string

var wireMarshalCalls int

func (wireMarshalError) MarshalYAML() (any, error) {
	wireMarshalCalls++
	return nil, fmt.Errorf("test YAML marshal failure")
}

func (wireMarshalKey) MarshalYAML() (any, error) {
	wireMarshalCalls++
	return nil, fmt.Errorf("test YAML map-key marshal failure")
}
