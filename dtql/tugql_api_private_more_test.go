package dtql

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTugQLPrivateMoreNestedCTEBodyPublicRoundTrip(t *testing.T) {
	source := `with outer as (
  with inner as (
    from customer as c
    select c.customerId as customerId
  )
  from inner as i
  select i.customerId as customerId
)
from outer as o
select o.customerId as customerId`

	document, diagnostics := ParseTugQL(source)
	if len(diagnostics) != 0 || document.Tree == nil {
		t.Fatalf("nested CTE source did not parse: document=%+v diagnostics=%+v", document, diagnostics)
	}
	if len(document.Tree.Definitions) != 1 || document.Tree.Definitions[0].Name != "outer" || document.Tree.Definitions[0].Query == nil {
		t.Fatalf("outer CTE was not exported: %+v", document.Tree.Definitions)
	}
	body := document.Tree.Definitions[0].Query
	if len(body.Definitions) != 1 || body.Definitions[0].Name != "inner" || body.Definitions[0].Query == nil {
		t.Fatalf("nested CTE body was not exported: %+v", body.Definitions)
	}
	if got := body.Query["from"].(map[string]any)["name"]; got != "inner" {
		t.Fatalf("outer CTE query source=%v, want inner", got)
	}

	wire, err := yaml.Marshal(document.Tree)
	if err != nil {
		t.Fatalf("marshal parsed tree: %v", err)
	}
	var roundTrip TugQLTree
	if err := yaml.Unmarshal(wire, &roundTrip); err != nil {
		t.Fatalf("unmarshal tree: %v\n%s", err, wire)
	}
	if len(roundTrip.Definitions) != 1 || roundTrip.Definitions[0].Query == nil || len(roundTrip.Definitions[0].Query.Definitions) != 1 {
		t.Fatalf("nested CTE body did not survive public round-trip: %+v", roundTrip.Definitions)
	}
	if !strings.Contains(string(wire), "name: inner") || !strings.Contains(string(wire), "name: customer") {
		t.Fatalf("nested body content missing from wire:\n%s", wire)
	}
}

func TestTugQLPrivateMoreTreeUnmarshalRejectsWithoutReplacingReceiver(t *testing.T) {
	original := TugQLTree{Format: "tugqtree", Version: 1, Query: map[string]any{"from": map[string]any{"name": "Before"}}}
	for _, wire := range []string{
		"format: tugqtree\nversion: 1\nquery: {from: {name: After}}\nunknown: true\n",
		"format: tugqtree\nversion: 1\nquery: {from: {name: After}}\nformat: tugqtree\n",
	} {
		t.Run(strings.TrimSpace(wire), func(t *testing.T) {
			tree := original
			if err := yaml.Unmarshal([]byte(wire), &tree); err == nil {
				t.Fatalf("malformed envelope accepted: %s", wire)
			}
			if got := tree.Query["from"].(map[string]any)["name"]; got != "Before" || tree.Format != original.Format || tree.Version != original.Version {
				t.Fatalf("failed decode partially replaced receiver: %+v", tree)
			}
		})
	}
}

func TestTugQLPrivateMoreImportMappingsSurvivePublicSourceRoundTrip(t *testing.T) {
	source := `with external from "./saved.tugql"
  using (
    @minimum = 12
    @label = 'ready'
    @tenant = @tenant
  )
from external
select Id`
	document, diagnostics := ParseTugQL(source)
	if len(diagnostics) != 0 || document.Tree == nil {
		t.Fatalf("import source did not parse: document=%+v diagnostics=%+v", document, diagnostics)
	}
	if len(document.Tree.Definitions) != 1 || document.Tree.Definitions[0].Kind != "import" {
		t.Fatalf("import definition missing from tree: %+v", document.Tree.Definitions)
	}
	mappings := document.Tree.Definitions[0].Using
	if len(mappings) != 3 || mappings[0].Name != "minimum" || mappings[0].Expression["value"] != 12 || mappings[1].Expression["value"] != "ready" || mappings[2].Expression["param"] != "tenant" {
		t.Fatalf("import mappings were not exported with their expression forms: %+v", mappings)
	}
	wire, err := yaml.Marshal(document.Tree)
	if err != nil {
		t.Fatalf("marshal imported tree: %v", err)
	}
	var roundTrip TugQLTree
	if err := yaml.Unmarshal(wire, &roundTrip); err != nil {
		t.Fatalf("unmarshal imported tree: %v\n%s", err, wire)
	}
	if len(roundTrip.Definitions) != 1 || len(roundTrip.Definitions[0].Using) != 3 {
		t.Fatalf("import mappings did not survive public round-trip: %+v", roundTrip.Definitions)
	}
}

func TestTugQLPrivateMoreRejectsMalformedNestedScalarBodyKeys(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query string
	}{
		{"unknown body field", `query:
  from: {name: T}
  columns:
    - query:
        query: {from: {name: U}}
        extra: true`},
		{"duplicate body query", `query:
  from: {name: T}
  columns:
    - query:
        query: {from: {name: U}}
        query: {from: {name: V}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := "format: tugqtree\nversion: 1\n" + tc.query + "\n"
			var tree TugQLTree
			if err := yaml.Unmarshal([]byte(wire), &tree); err == nil {
				t.Fatalf("malformed nested scalar body was accepted: %s", wire)
			}
		})
	}
}

func TestTugQLDocumentJSONRejectsUnknownFieldsTransactionally(t *testing.T) {
	original := TugQLDocument{Source: "prior", Tree: &TugQLTree{Format: "tugqtree", Version: 1, Query: map[string]any{"from": map[string]any{"name": "Before"}}}, SourceMetadata: TugQLSourceMetadata{Format: "tugql", Version: 1}}
	for _, input := range []string{
		`{"source":"next","sourceMetadata":{"format":"tugql","version":1},"unexpected":true}`,
		`{"source":"next","sourceMetadata":{"format":"tugql","version":1,"unexpected":true}}`,
		`{"tree":{"format":"tugqtree","version":1,"unexpected":true,"query":{"from":{"name":"T"}}},"sourceMetadata":{"format":"tugql","version":1}}`,
	} {
		document := original
		if err := json.Unmarshal([]byte(input), &document); err == nil {
			t.Errorf("unknown document field accepted: %s", input)
		}
		if document.Source != original.Source || document.Tree == nil || document.Tree.Query["from"].(map[string]any)["name"] != "Before" || document.SourceMetadata != original.SourceMetadata {
			t.Errorf("failed decode replaced prior document: %+v", document)
		}
	}
}

func TestTugQLNodeBudgetCountsNilAndTypedCallerContainers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query map[string]any
	}{
		{name: "nil root"},
		{name: "typed map", query: map[string]any{"payload": map[string]string{"key": "value"}}},
		{name: "typed slice", query: map[string]any{"payload": []string{"value"}}},
		{name: "array", query: map[string]any{"payload": [1]string{"value"}}},
		{name: "scalar", query: map[string]any{"payload": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			budget := &tugqlResolveBudget{}
			if err := countTugQLTree(TugQLTree{Query: tc.query}, budget); err != nil {
				t.Fatalf("count failed: %v", err)
			}
			if budget.nodes == 0 {
				t.Fatal("tree root was not charged")
			}
		})
	}
}
