package dtql

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTugQLWireRejectsMalformedEnvelopes(t *testing.T) {
	valid := "format: tugqtree\nversion: 1\nquery: {from: {name: Invoice}}\n"
	cases := []struct {
		name string
		data string
	}{
		{"root scalar", "hello\n"},
		{"unknown envelope field", valid + "surprise: true\n"},
		{"duplicate envelope field", valid + "version: 1\n"},
		{"missing format", "version: 1\nquery: {from: {name: Invoice}}\n"},
		{"missing version", "format: tugqtree\nquery: {from: {name: Invoice}}\n"},
		{"missing query", "format: tugqtree\nversion: 1\n"},
		{"parameters not sequence", "format: tugqtree\nversion: 1\nparameters: {}\nquery: {from: {name: Invoice}}\n"},
		{"definitions not sequence", "format: tugqtree\nversion: 1\ndefinitions: {}\nquery: {from: {name: Invoice}}\n"},
		{"query not mapping", "format: tugqtree\nversion: 1\nquery: []\n"},
		{"wrong version type", "format: tugqtree\nversion: one\nquery: {from: {name: Invoice}}\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var tree TugQLTree
			if err := yaml.Unmarshal([]byte(tc.data), &tree); err == nil {
				t.Fatalf("malformed envelope accepted: %s", tc.data)
			}
		})
	}
}

func TestTugQLWireRejectsMalformedParametersAndDefinitions(t *testing.T) {
	prefix := "format: tugqtree\nversion: 1\n"
	suffix := "query: {from: {name: Invoice}}\n"
	cases := []struct {
		name string
		data string
	}{
		{"parameter scalar", prefix + "parameters: [P]\n" + suffix},
		{"unknown parameter field", prefix + "parameters: [{name: P, type: string, extra: x}]\n" + suffix},
		{"missing parameter name", prefix + "parameters: [{type: string}]\n" + suffix},
		{"empty parameter name", prefix + "parameters: [{name: '', type: string}]\n" + suffix},
		{"numeric parameter name", prefix + "parameters: [{name: 7, type: string}]\n" + suffix},
		{"missing parameter type", prefix + "parameters: [{name: P}]\n" + suffix},
		{"numeric parameter type", prefix + "parameters: [{name: P, type: 7}]\n" + suffix},
		{"non boolean required", prefix + "parameters: [{name: P, type: string, required: yes}]\n" + suffix},
		{"required parameter default", prefix + "parameters: [{name: P, type: integer, required: true, default: 0}]\n" + suffix},
		{"null parameter default", prefix + "parameters: [{name: P, type: string, default: null}]\n" + suffix},
		{"definition scalar", prefix + "definitions: [Thing]\n" + suffix},
		{"unknown definition field", prefix + "definitions: [{kind: cte, name: X, extra: true}]\n" + suffix},
		{"missing definition kind", prefix + "definitions: [{name: X}]\n" + suffix},
		{"numeric definition name", prefix + "definitions: [{kind: cte, name: 3, query: {query: {from: {name: T}}}}]\n" + suffix},
		{"unknown definition kind", prefix + "definitions: [{kind: view, name: X}]\n" + suffix},
		{"cte missing query", prefix + "definitions: [{kind: cte, name: X}]\n" + suffix},
		{"cte with import path", prefix + "definitions: [{kind: cte, name: X, path: x, query: {query: {from: {name: T}}}}]\n" + suffix},
		{"cte body missing query", prefix + "definitions: [{kind: cte, name: X, query: {definitions: []}}]\n" + suffix},
		{"cte unknown body field", prefix + "definitions: [{kind: cte, name: X, query: {query: {from: {name: T}, extra: x}}}]\n" + suffix},
		{"import missing path", prefix + "definitions: [{kind: import, name: X}]\n" + suffix},
		{"import non string path", prefix + "definitions: [{kind: import, name: X, path: 9}]\n" + suffix},
		{"import with query", prefix + "definitions: [{kind: import, name: X, path: x, query: {query: {from: {name: T}}}}]\n" + suffix},
		{"import using not sequence", prefix + "definitions: [{kind: import, name: X, path: x, using: {P: 1}}]\n" + suffix},
		{"using unknown field", prefix + "definitions: [{kind: import, name: X, path: x, using: [{name: P, expression: {value: 1}, extra: true}]}]\n" + suffix},
		{"using missing expression", prefix + "definitions: [{kind: import, name: X, path: x, using: [{name: P}]}]\n" + suffix},
		{"using bad expression", prefix + "definitions: [{kind: import, name: X, path: x, using: [{name: P, expression: {field: F, value: 2}}]}]\n" + suffix},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var tree TugQLTree
			if err := yaml.Unmarshal([]byte(tc.data), &tree); err == nil {
				t.Fatalf("malformed parameter or definition accepted: %s", tc.data)
			}
		})
	}
}

func TestTugQLWireRejectsMalformedNestedQueryShapes(t *testing.T) {
	prefix := "format: tugqtree\nversion: 1\nquery: "
	cases := []struct {
		name  string
		query string
	}{
		{"unknown query field", "{from: {name: T}, typo: true}"},
		{"from scalar", "{from: T}"},
		{"from lacks source", "{from: {alias: t}}"},
		{"column unknown field", "{from: {name: T}, columns: [{field: X, typo: true}]}"},
		{"column multiple forms", "{from: {name: T}, columns: [{field: X, value: 1}]}"},
		{"column empty form", "{from: {name: T}, columns: [{field: ''}]}"},
		{"source without field", "{from: {name: T}, columns: [{source: t, value: 1}]}"},
		{"binary missing right", "{from: {name: T}, columns: [{binary: {op: '+', left: {field: X}}}]}"},
		{"aggregate nested invalid", "{from: {name: T}, columns: [{aggregate: {function: SUM, args: [{value: 1, field: X}]}}]}"},
		{"wildcard mixed with field", "{from: {name: T}, columns: [{wildcard: {exclude: [Secret]}, field: X}]}"},
		{"wildcard missing exclusions", "{from: {name: T}, columns: [{wildcard: {}}]}"},
		{"wildcard empty exclusion", "{from: {name: T}, columns: [{wildcard: {exclude: ['']}}]}"},
		{"condition multiple forms", "{from: {name: T}, where: {and: [{isNull: {field: X}}], isNull: {field: Y}}}"},
		{"condition comparison missing right", "{from: {name: T}, where: {op: '=', left: {field: X}}}"},
		{"empty condition group", "{from: {name: T}, where: {and: []}}"},
		{"nested condition invalid", "{from: {name: T}, where: {or: [{notExists: {query: {from: {name: U}}}}, {and: []}]}}"},
		{"exists missing query", "{from: {name: T}, where: {exists: {}}}"},
		{"exists query malformed", "{from: {name: T}, where: {exists: {query: {where: {isNull: {field: X}}}}}}"},
		{"join missing from", "{from: {name: T, joins: [{on: [{op: '=', left: {field: A}, right: {field: B}}]}]}}"},
		{"join invalid relationship form", "{from: {name: T, joins: [{from: {name: U}, on: [{op: relationship, left: {field: A}, right: {field: B}}]}]}}"},
		{"having malformed", "{from: {name: T}, having: {isNull: {field: X}, or: [{isNull: {field: Y}}]}}"},
		{"order expression invalid", "{from: {name: T}, orderBy: [{binary: {op: '+', left: {field: A}}}]}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var tree TugQLTree
			if err := yaml.Unmarshal([]byte(prefix+tc.query+"\n"), &tree); err == nil {
				t.Fatalf("malformed nested query accepted: %s", tc.query)
			}
		})
	}
}

func TestTugQLWireRejectsMalformedCallNodes(t *testing.T) {
	prefix := "format: tugqtree\nversion: 1\nquery: {from: {name: T}, columns: [{call: "
	suffix := "}]}\n"
	cases := []struct{ name, call string }{
		{"call scalar", "COALESCE"},
		{"call unknown field", "{function: COALESCE, args: [], extra: true}"},
		{"call missing function", "{args: []}"},
		{"call missing args", "{function: COALESCE}"},
		{"call function numeric", "{function: 7, args: []}"},
		{"call args scalar", "{function: COALESCE, args: X}"},
		{"call arg not expression", "{function: COALESCE, args: [hello]}"},
		{"call arg multiple forms", "{function: COALESCE, args: [{field: X, value: 1}]}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var tree TugQLTree
			if err := yaml.Unmarshal([]byte(prefix+tc.call+suffix), &tree); err == nil {
				t.Fatalf("malformed call accepted: %s", tc.call)
			}
		})
	}
}

func TestTugQLWireMarshalRejectsCallerMutations(t *testing.T) {
	base := func() TugQLTree {
		return TugQLTree{Format: "tugqtree", Version: 1, Query: map[string]any{"from": map[string]any{"name": "T"}}}
	}
	cases := []struct {
		name   string
		mutate func(*TugQLTree)
	}{
		{"unsupported envelope", func(tree *TugQLTree) { tree.Version = 2 }},
		{"missing query", func(tree *TugQLTree) { tree.Query = nil }},
		{"empty parameter", func(tree *TugQLTree) { tree.Parameters = []TugQLParameter{{Name: "", Type: "string"}} }},
		{"duplicate parameters", func(tree *TugQLTree) {
			tree.Parameters = []TugQLParameter{{Name: "P", Type: "string"}, {Name: "P", Type: "integer"}}
		}},
		{"required default", func(tree *TugQLTree) {
			value := any(1)
			tree.Parameters = []TugQLParameter{{Name: "P", Type: "integer", Required: true, Default: &value}}
		}},
		{"null default", func(tree *TugQLTree) {
			var value any
			tree.Parameters = []TugQLParameter{{Name: "P", Type: "string", Default: &value}}
		}},
		{"unknown definition kind", func(tree *TugQLTree) { tree.Definitions = []TugQLDefinition{{Kind: "view", Name: "V"}} }},
		{"cte missing body", func(tree *TugQLTree) { tree.Definitions = []TugQLDefinition{{Kind: "cte", Name: "V"}} }},
		{"import missing path", func(tree *TugQLTree) { tree.Definitions = []TugQLDefinition{{Kind: "import", Name: "V"}} }},
		{"import mapping missing expression", func(tree *TugQLTree) {
			tree.Definitions = []TugQLDefinition{{Kind: "import", Name: "V", Path: "x", Using: []TugQLMapping{{Name: "P", Expression: map[string]any{"extra": 1}}}}}
		}},
		{"import mapping multiple expression forms", func(tree *TugQLTree) {
			tree.Definitions = []TugQLDefinition{{Kind: "import", Name: "V", Path: "x", Using: []TugQLMapping{{Name: "P", Expression: map[string]any{"field": "X", "value": 1}}}}}
		}},
		{"malformed query value", func(tree *TugQLTree) { tree.Query["unknown"] = true }},
		{"malformed call value", func(tree *TugQLTree) {
			tree.Query["columns"] = []any{map[string]any{"call": map[string]any{"function": "F", "args": []any{map[string]any{"binary": map[string]any{"op": "+", "left": map[string]any{"field": "A"}}}}}}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree := base()
			tc.mutate(&tree)
			if _, err := yaml.Marshal(tree); err == nil {
				t.Fatal("caller-mutated tree was marshaled")
			}
		})
	}
}

func TestTugQLWireRoundTripsNestedDefinitionsConditionsCallsAndScalars(t *testing.T) {
	input := `format: tugqtree
version: 1
parameters:
  - name: Maximum
    type: integer
    default: 12
definitions:
  - kind: import
    name: Saved
    path: queries/saved.tugql
    using:
      - name: Limit
        expression: {param: Maximum}
  - kind: cte
    name: Customers
    query:
      definitions:
        - kind: cte
          name: Active
          query:
            query: {from: {name: Customer}}
      query: {from: {name: Active}}
query:
  from: {name: Invoice}
  where:
    or:
      - {op: "=", left: {field: InvoiceId}, right: {value: 1}}
      - {exists: {query: {from: {name: Payment}}}}
  columns:
    - {call: {function: COALESCE, args: [{field: Note}, {value: "quoted, value"}]}, as: Note}
    - {aggregate: {function: COUNT, args: []}, as: Count}
`
	var parsed TugQLTree
	if err := yaml.Unmarshal([]byte(input), &parsed); err != nil {
		t.Fatalf("valid nested wire tree rejected: %v", err)
	}
	bodyData, err := yaml.Marshal(*parsed.Definitions[1].Query)
	if err != nil {
		t.Fatalf("marshal query body: %v", err)
	}
	var decodedBody TugQLBody
	if err := yaml.Unmarshal(bodyData, &decodedBody); err != nil {
		t.Fatalf("unmarshal query body: %v\n%s", err, bodyData)
	}
	if decodedBody.Query == nil || len(decodedBody.Definitions) != 1 || decodedBody.Definitions[0].Name != "Active" {
		t.Fatalf("query body lost nested definition/query: %+v", decodedBody)
	}
	doc, parseDiagnostics := ParseTugQL("from Invoice\nwhere InvoiceId = 1\nselect COALESCE(Note, 'quoted, value') as Note")
	if len(parseDiagnostics) != 0 || doc.Tree == nil {
		t.Fatalf("parse diagnostics=%+v tree=%+v", parseDiagnostics, doc.Tree)
	}
	data, err := yaml.Marshal(parsed)
	if err != nil {
		t.Fatal(err)
	}
	var got TugQLTree
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("round-trip YAML: %v\n%s", err, data)
	}
	if got.Format != parsed.Format || got.Version != parsed.Version || len(got.Parameters) != 1 || len(got.Definitions) != 2 || got.Definitions[0].Kind != "import" || len(got.Definitions[0].Using) != 1 || len(got.Definitions[1].Query.Definitions) != 1 {
		t.Fatalf("round-trip lost envelope/import data: %+v", got)
	}
	if !strings.Contains(string(data), "COALESCE") || !strings.Contains(string(data), "quoted, value") || !strings.Contains(string(data), "value: 1") {
		t.Fatalf("round-trip lost scalar or call values: %s", data)
	}
	resolved, resolveDiagnostics := ResolveTugQL(tugqlTestDocument(&got), TugQLResolveContext{})
	if resolved.Query != nil || len(resolveDiagnostics) == 0 {
		t.Fatalf("unbound imported query unexpectedly resolved: %+v %+v", resolved, resolveDiagnostics)
	}
}

func TestTugQLWirePreservesNullAndNumericLiteralTags(t *testing.T) {
	input := `format: tugqtree
version: 1
query:
  from: {name: T}
  columns:
    - {value: 0}
    - {value: -12}
    - {value: 1.5}
    - {value: null}
    - {value: "null"}
    - {value: "001"}
`
	var tree TugQLTree
	if err := yaml.Unmarshal([]byte(input), &tree); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip TugQLTree
	if err := yaml.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("round-trip: %v\n%s", err, data)
	}
	columns := roundTrip.Query["columns"].([]any)
	if len(columns) != 6 {
		t.Fatalf("columns=%#v", columns)
	}
	values := make([]any, len(columns))
	for i, column := range columns {
		values[i] = column.(map[string]any)["value"]
	}
	if values[0] != 0 || values[1] != -12 || values[2] != 1.5 || values[3] != nil || values[4] != "null" || values[5] != "001" {
		t.Fatalf("literal values changed across wire round-trip: %#v", values)
	}
}

func TestTugQLWireRoundTripRemainsResolvable(t *testing.T) {
	doc, diagnostics := ParseTugQL("from Invoice as i\nwhere i.InvoiceId = 0\nselect i.InvoiceId as Id")
	if len(diagnostics) != 0 || doc.Tree == nil {
		t.Fatalf("parse diagnostics=%+v tree=%+v", diagnostics, doc.Tree)
	}
	data, err := yaml.Marshal(*doc.Tree)
	if err != nil {
		t.Fatal(err)
	}
	var decoded TugQLTree
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal canonical tree: %v\n%s", err, data)
	}
	result, diagnostics := ResolveTugQL(tugqlTestDocument(&decoded), TugQLResolveContext{
		AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "wire-schema-v1", Tables: []TugQLTable{{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}}}}}},
	})
	if len(diagnostics) != 0 || result.Query == nil || result.SchemaVersion != "wire-schema-v1" || len(result.Columns) != 1 || result.Columns[0].Name != "Id" || result.Columns[0].Type != "integer" {
		t.Fatalf("resolved=%+v diagnostics=%+v", result, diagnostics)
	}
}
