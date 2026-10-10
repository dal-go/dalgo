package dtql

import "testing"

func TestTugQLRelationshipResolutionBoundaries(t *testing.T) {
	relation := TugQLRelationship{
		ID:                 "invoice-customer",
		Version:            "v1",
		From:               TugQLRelationEndpoint{Table: "Invoice", Source: "i"},
		To:                 TugQLRelationEndpoint{Table: "Customer", Source: "c"},
		Pairs:              []TugQLRelationshipPair{{FromField: "CustomerId", ToField: "Id"}},
		ExactTypedEquality: true,
	}
	newContext := func() TugQLResolveContext {
		return TugQLResolveContext{
			AuthorizedSchemas: []TugQLAuthorizedSchema{{
				Version: "schema-v1",
				Tables: []TugQLTable{
					{Name: "Invoice", Fields: []TugQLField{{Name: "CustomerId", Type: "integer", Authorized: true}, {Name: "InvoiceId", Type: "integer", Authorized: true}}},
					{Name: "Customer", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}},
				},
			}},
			Relationships: []TugQLRelationship{relation},
		}
	}
	baseContext := newContext()
	field := func(source, name string) *exprYAML { return &exprYAML{Source: source, Field: name} }
	equality := func(leftSource, leftName, rightSource, rightName string) condYAML {
		return condYAML{Op: "==", Left: field(leftSource, leftName), Right: field(rightSource, rightName)}
	}
	query := func(on ...condYAML) *document {
		joined := fromYAML{Name: "Customer", Alias: "c"}
		return &document{From: fromYAML{Name: "Invoice", Alias: "i", Joins: []joinYAML{{From: &joined, On: on}}}}
	}
	tests := []struct {
		name         string
		context      TugQLResolveContext
		on           []condYAML
		wantCode     string
		wantReceipts int
		wantPairs    int
	}{
		{name: "omitted ON expands exact relation", context: baseContext, wantReceipts: 1, wantPairs: 1},
		{name: "shorthand selects declared pair", context: baseContext, on: []condYAML{{Op: "relationship", Left: field("i", "CustomerId")}}, wantReceipts: 1, wantPairs: 1},
		{name: "explicit equality records receipt", context: baseContext, on: []condYAML{equality("i", "CustomerId", "c", "Id")}, wantReceipts: 1, wantPairs: 1},
		{name: "explicit unrelated equality stays explicit", context: baseContext, on: []condYAML{equality("i", "InvoiceId", "c", "Id")}},
		{name: "non equality ON is rejected", context: baseContext, on: []condYAML{{Op: ">", Left: field("i", "CustomerId"), Right: field("c", "Id")}}, wantCode: "invalid_join_condition"},
		{name: "shorthand cannot be mixed", context: baseContext, on: []condYAML{{Op: "relationship", Left: field("i", "CustomerId")}, equality("i", "CustomerId", "c", "Id")}, wantCode: "invalid_relationship_shorthand"},
		{name: "shorthand requires a field", context: baseContext, on: []condYAML{{Op: "relationship"}}, wantCode: "invalid_relationship_shorthand"},
		{name: "no relation is diagnosed", context: TugQLResolveContext{AuthorizedSchemas: baseContext.AuthorizedSchemas}, wantCode: "relationship_not_found"},
		{name: "inexact relation is not used", context: TugQLResolveContext{AuthorizedSchemas: baseContext.AuthorizedSchemas, Relationships: []TugQLRelationship{{From: relation.From, To: relation.To, Pairs: relation.Pairs}}}, wantCode: "relationship_not_found"},
		{name: "ambiguous relations are diagnosed", context: TugQLResolveContext{AuthorizedSchemas: baseContext.AuthorizedSchemas, Relationships: []TugQLRelationship{relation, relation}}, wantCode: "ambiguous_relationship"},
		{name: "unauthorized pair is rejected", context: func() TugQLResolveContext {
			c := newContext()
			c.AuthorizedSchemas[0].Tables[0].Fields[0].Authorized = false
			return c
		}(), wantCode: "unauthorized_relationship"},
		{name: "incompatible pair types are rejected", context: func() TugQLResolveContext {
			c := newContext()
			c.AuthorizedSchemas[0].Tables[1].Fields[0].Type = "string"
			return c
		}(), wantCode: "unauthorized_relationship"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := query(test.on...)
			var receipts []TugQLRelationshipExpansion
			diagnostics := resolveTugQLRelationships(doc, test.context, &receipts)
			if test.wantCode != "" {
				if len(diagnostics) != 1 || diagnostics[0].Code != test.wantCode {
					t.Fatalf("diagnostics = %+v, want code %q", diagnostics, test.wantCode)
				}
				return
			}
			if len(diagnostics) != 0 || len(receipts) != test.wantReceipts {
				t.Fatalf("diagnostics=%+v receipts=%+v", diagnostics, receipts)
			}
			if test.wantPairs != 0 && len(doc.From.Joins[0].On) != test.wantPairs {
				t.Fatalf("expanded ON has %d pairs, want %d", len(doc.From.Joins[0].On), test.wantPairs)
			}
		})
	}
}

func TestTugQLRelationshipJoinEqualityShapeBoundaries(t *testing.T) {
	relation := TugQLRelationship{Pairs: []TugQLRelationshipPair{{FromField: "CustomerId", ToField: "Id"}, {FromField: "Region", ToField: "Region"}}}
	field := func(source, name string) *exprYAML { return &exprYAML{Source: source, Field: name} }
	equality := func(leftSource, leftName, rightSource, rightName string) condYAML {
		return condYAML{Op: "=", Left: field(leftSource, leftName), Right: field(rightSource, rightName)}
	}
	valid := []condYAML{equality("i", "CustomerId", "c", "Id"), equality("i", "Region", "c", "Region")}
	if _, ok := flattenTugQLJoinEqualities([]condYAML{{And: valid}}); !ok {
		t.Fatal("nested AND equality pairs did not flatten")
	}
	for name, conditions := range map[string][]condYAML{
		"empty":           nil,
		"or":              {{Or: valid}},
		"unsupported op":  {{Op: ">", Left: field("i", "CustomerId"), Right: field("c", "Id")}},
		"missing operand": {{Op: "=", Left: field("i", "CustomerId")}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := flattenTugQLJoinEqualities(conditions); ok {
				t.Fatal("unsupported equality structure was accepted")
			}
		})
	}
	for name, conditions := range map[string][]condYAML{
		"forward":        valid,
		"reverse":        {equality("c", "Id", "i", "CustomerId"), equality("c", "Region", "i", "Region")},
		"partial":        valid[:1],
		"duplicate pair": {valid[0], valid[0]},
		"wrong source":   {equality("x", "CustomerId", "c", "Id"), valid[1]},
		"wrong operator": {{Op: ">", Left: field("i", "CustomerId"), Right: field("c", "Id")}, valid[1]},
	} {
		t.Run("match "+name, func(t *testing.T) {
			want := name == "forward" || name == "reverse"
			got := tugqlJoinMatchesRelationship(conditions, relation, "i", "c", false)
			if name == "reverse" {
				got = tugqlJoinMatchesRelationship(conditions, relation, "c", "i", true)
			}
			if got != want {
				t.Fatalf("match = %v, want %v", got, want)
			}
		})
	}
}
