package dtql

import "testing"

func TestTugQLDefaultProjectionMergesExactRelationshipKeysInsideCTEs(t *testing.T) {
	joined := TugQLDefinition{
		Kind: "cte",
		Name: "Joined",
		Query: &TugQLBody{Query: map[string]any{
			"from": map[string]any{
				"name":  "Invoice",
				"alias": "i",
				"joins": []any{map[string]any{
					"from": map[string]any{"name": "Customer", "alias": "c"},
					"on": []any{map[string]any{
						"op":    "==",
						"left":  map[string]any{"field": "CustomerId", "source": "i"},
						"right": map[string]any{"field": "Id", "source": "c"},
					}},
				}},
			},
		}},
	}
	doc := TugQLDocument{
		SourceMetadata: TugQLSourceMetadata{Format: "tugql", Version: 1},
		Tree: &TugQLTree{
			Format: "tugqtree", Version: 1,
			Definitions: []TugQLDefinition{joined},
			Query:       map[string]any{"from": map[string]any{"name": "Joined", "alias": "j"}},
		},
	}
	context := TugQLResolveContext{
		AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "schema-v1", Tables: []TugQLTable{
			{Name: "Invoice", Fields: []TugQLField{{Name: "CustomerId", Type: "integer", Authorized: true}}},
			{Name: "Customer", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}},
		}}},
		Relationships: []TugQLRelationship{{
			ID: "invoice-customer", Version: "r1",
			From:  TugQLRelationEndpoint{Table: "Invoice", Source: "i"},
			To:    TugQLRelationEndpoint{Table: "Customer", Source: "c"},
			Pairs: []TugQLRelationshipPair{{FromField: "CustomerId", ToField: "Id"}}, ExactTypedEquality: true,
		}},
	}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if len(diagnostics) != 0 || resolved.Query == nil {
		t.Fatalf("resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
	if len(resolved.Columns) != 1 || resolved.Columns[0].Name != "CustomerId" || len(resolved.Columns[0].Lineage) != 2 {
		t.Fatalf("CTE default projection did not merge relationship key: %+v", resolved.Columns)
	}
	want := []TugQLOutputLineage{{Source: "i", Field: "CustomerId"}, {Source: "c", Field: "Id"}}
	for i, lineage := range want {
		if resolved.Columns[0].Lineage[i] != lineage {
			t.Fatalf("lineage[%d]=%+v, want %+v", i, resolved.Columns[0].Lineage[i], lineage)
		}
	}
	if len(resolved.Relationships) != 1 || resolved.Relationships[0].ID != "invoice-customer" {
		t.Fatalf("relationship receipts=%+v", resolved.Relationships)
	}
}

func TestTugQLDefaultProjectionDoesNotUseNestedCTERelationshipEvidence(t *testing.T) {
	definition := TugQLDefinition{
		Kind: "cte", Name: "Joined",
		Query: &TugQLBody{Query: map[string]any{
			"from": map[string]any{
				"name": "Invoice", "alias": "i",
				"joins": []any{map[string]any{
					"from": map[string]any{"name": "Customer", "alias": "c"},
					"on": []any{map[string]any{
						"op": "==", "left": map[string]any{"field": "InvoiceId", "source": "i"},
						"right": map[string]any{"field": "CustomerId", "source": "c"},
					}},
				}},
			},
			"where": map[string]any{"exists": map[string]any{"query": map[string]any{
				"from": map[string]any{
					"name": "Invoice", "alias": "i",
					"joins": []any{map[string]any{
						"from": map[string]any{"name": "Customer", "alias": "c"},
						"on": []any{map[string]any{
							"op": "==", "left": map[string]any{"field": "CustomerId", "source": "i"},
							"right": map[string]any{"field": "Id", "source": "c"},
						}},
					}},
				},
				"columns": []any{map[string]any{"field": "Id", "source": "c"}},
			}}},
		}},
	}
	doc := TugQLDocument{
		SourceMetadata: TugQLSourceMetadata{Format: "tugql", Version: 1},
		Tree: &TugQLTree{
			Format: "tugqtree", Version: 1,
			Definitions: []TugQLDefinition{definition},
			Query:       map[string]any{"from": map[string]any{"name": "Joined", "alias": "j"}},
		},
	}
	context := TugQLResolveContext{
		AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "schema-v1", Tables: []TugQLTable{
			{Name: "Invoice", Fields: []TugQLField{
				{Name: "CustomerId", Type: "integer", Authorized: true},
				{Name: "InvoiceId", Type: "integer", Authorized: true},
			}},
			{Name: "Customer", Fields: []TugQLField{
				{Name: "Id", Type: "integer", Authorized: true},
				{Name: "CustomerId", Type: "integer", Authorized: true},
			}},
		}}},
		Relationships: []TugQLRelationship{{
			ID: "invoice-customer", Version: "r1",
			From:  TugQLRelationEndpoint{Table: "Invoice", Source: "i"},
			To:    TugQLRelationEndpoint{Table: "Customer", Source: "c"},
			Pairs: []TugQLRelationshipPair{{FromField: "CustomerId", ToField: "Id"}}, ExactTypedEquality: true,
		}},
	}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if len(diagnostics) != 0 || resolved.Query == nil {
		t.Fatalf("resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
	if len(resolved.Relationships) != 1 {
		t.Fatalf("nested relationship receipts=%+v", resolved.Relationships)
	}
	lineageByName := make(map[string][]TugQLOutputLineage, len(resolved.Columns))
	for _, column := range resolved.Columns {
		lineageByName[column.Name] = column.Lineage
	}
	if len(lineageByName["CustomerId"]) != 1 || lineageByName["CustomerId"][0] != (TugQLOutputLineage{Source: "i", Field: "CustomerId"}) {
		t.Fatalf("outer Invoice.CustomerId was merged using nested evidence: %+v", resolved.Columns)
	}
	if len(lineageByName["c_CustomerId"]) != 1 || lineageByName["c_CustomerId"][0] != (TugQLOutputLineage{Source: "c", Field: "CustomerId"}) {
		t.Fatalf("outer Customer.CustomerId was merged using nested evidence: %+v", resolved.Columns)
	}
}
