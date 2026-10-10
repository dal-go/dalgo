package dtql

import "testing"

func TestTugQLPublicRelationshipFailuresPropagateThroughScopes(t *testing.T) {
	field := func(source, name string) map[string]any { return map[string]any{"source": source, "field": name} }
	badQuery := func() map[string]any {
		return map[string]any{
			"from":    map[string]any{"name": "Invoice", "alias": "i", "joins": []any{map[string]any{"from": map[string]any{"name": "Customer", "alias": "c"}}}},
			"columns": []any{field("i", "InvoiceId")},
		}
	}
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "schema-r1", Tables: []TugQLTable{
		{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}, {Name: "CustomerId", Type: "integer", Authorized: true}}},
		{Name: "Customer", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}},
	}}}}
	for _, name := range []string{"scalar", "binary", "aggregate", "where", "and", "or", "exists", "having", "derived source", "joined derived source", "nested joined source"} {
		t.Run(name, func(t *testing.T) {
			query := map[string]any{"from": map[string]any{"name": "Invoice", "alias": "i"}, "columns": []any{field("i", "InvoiceId")}}
			scalar := map[string]any{"query": badQuery()}
			condition := map[string]any{"op": "==", "left": scalar, "right": map[string]any{"value": 0}}
			switch name {
			case "scalar":
				query["columns"] = []any{scalar}
			case "binary":
				query["columns"] = []any{map[string]any{"binary": map[string]any{"op": "+", "left": scalar, "right": map[string]any{"value": 1}}, "as": "Value"}}
			case "aggregate":
				query["columns"] = []any{map[string]any{"aggregate": map[string]any{"function": "SUM", "args": []any{scalar}}, "as": "Value"}}
			case "where":
				query["where"] = condition
			case "and":
				query["where"] = map[string]any{"and": []any{condition}}
			case "or":
				query["where"] = map[string]any{"or": []any{condition}}
			case "exists":
				query["where"] = map[string]any{"exists": map[string]any{"query": badQuery()}}
			case "having":
				query["having"] = condition
			case "derived source":
				derived := badQuery()
				derived["as"] = "d"
				query["from"] = map[string]any{"query": derived}
				query["columns"] = []any{field("d", "InvoiceId")}
			case "joined derived source":
				derived := badQuery()
				derived["as"] = "d"
				query["from"].(map[string]any)["joins"] = []any{map[string]any{"from": map[string]any{"query": derived}, "on": []any{map[string]any{"op": "==", "left": field("i", "InvoiceId"), "right": field("d", "InvoiceId")}}}}
			case "nested joined source":
				nested := map[string]any{"name": "Customer", "alias": "c", "joins": []any{map[string]any{"from": map[string]any{"name": "Invoice", "alias": "other"}}}}
				query["from"].(map[string]any)["joins"] = []any{map[string]any{"from": nested, "on": []any{map[string]any{"op": "==", "left": field("i", "CustomerId"), "right": field("c", "Id")}}}}
			}
			resolved, diagnostics := ResolveTugQL(tugqlTestDocument(&TugQLTree{Format: "tugqtree", Version: 1, Query: query}), context)
			if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "relationship_not_found" {
				t.Fatalf("nested unresolved join escaped scope: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
			}
		})
	}
}

func TestTugQLPublicReverseShorthandAndRelationshipMetadata(t *testing.T) {
	newContext := func() TugQLResolveContext {
		return TugQLResolveContext{
			AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "schema-r1", Tables: []TugQLTable{
				{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}, {Name: "CustomerId", Type: "integer", Authorized: true}}},
				{Name: "Customer", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}},
			}}},
			Relationships: []TugQLRelationship{{ID: "invoice-customer", Version: "r1", From: TugQLRelationEndpoint{Table: "Invoice", Source: "i"}, To: TugQLRelationEndpoint{Table: "Customer", Source: "c"}, Pairs: []TugQLRelationshipPair{{FromField: "CustomerId", ToField: "Id"}}, ExactTypedEquality: true}},
		}
	}
	for _, tc := range []struct{ name, source, code string }{
		{"reverse shorthand merges exact key lineage", "from Customer as c\njoin Invoice as i\n  on Id\n", ""},
		{"unknown shorthand field", "from Invoice as i\njoin Customer as c\n  on InvoiceId\n", "relationship_not_found"},
		{"empty declared pairs", "from Invoice as i\njoin Customer as c\n", "unauthorized_relationship"},
		{"conflicting authorized field types", "from Invoice as i\njoin Customer as c\n", "unauthorized_relationship"},
		{"invalid equality inside AND", "from Invoice as i\njoin Customer as c\n  on i.CustomerId = c.Id and i.InvoiceId > c.Id\n", "invalid_join_condition"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			context := newContext()
			if tc.name == "empty declared pairs" {
				context.Relationships[0].Pairs = nil
			}
			if tc.name == "conflicting authorized field types" {
				context.AuthorizedSchemas[0].Tables[0].Fields = append(context.AuthorizedSchemas[0].Tables[0].Fields, TugQLField{Name: "CustomerId", Type: "string", Authorized: true})
			}
			doc, diagnostics := ParseTugQL(tc.source)
			if len(diagnostics) != 0 {
				t.Fatalf("parse diagnostics=%+v", diagnostics)
			}
			resolved, diagnostics := ResolveTugQL(doc, context)
			if tc.code != "" {
				if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != tc.code {
					t.Fatalf("query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
				}
				return
			}
			if len(diagnostics) != 0 || resolved.Query == nil || len(resolved.Relationships) != 1 {
				t.Fatalf("resolved=%+v diagnostics=%+v", resolved, diagnostics)
			}
			merged := 0
			for _, column := range resolved.Columns {
				if len(column.Lineage) == 2 {
					merged++
				}
			}
			if merged != 1 {
				t.Fatalf("expected exactly one shared key with both origins: %+v", resolved.Columns)
			}
		})
	}
}

func TestTugQLPublicDerivedOutputProjection(t *testing.T) {
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "schema-r1", Tables: []TugQLTable{
		{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}, {Name: "CustomerId", Type: "integer", Authorized: true}}},
	}}}}
	for _, name := range []string{"default authorized fields", "unaliased field keeps its name", "unnamed computed output is rejected"} {
		t.Run(name, func(t *testing.T) {
			inner := map[string]any{"as": "d", "from": map[string]any{"name": "Invoice", "alias": "i"}}
			if name == "unaliased field keeps its name" {
				inner["columns"] = []any{map[string]any{"source": "i", "field": "InvoiceId"}}
			}
			if name == "unnamed computed output is rejected" {
				inner["columns"] = []any{map[string]any{"value": 1}}
			}
			query := map[string]any{"from": map[string]any{"query": inner}, "columns": []any{map[string]any{"source": "d", "field": "InvoiceId", "as": "ID"}}}
			resolved, diagnostics := ResolveTugQL(tugqlTestDocument(&TugQLTree{Format: "tugqtree", Version: 1, Query: query}), context)
			if name == "unnamed computed output is rejected" {
				if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "invalid_projection" {
					t.Fatalf("query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
				}
				return
			}
			if resolved.Query == nil || len(diagnostics) != 0 || len(resolved.Columns) != 1 || resolved.Columns[0].Name != "ID" || resolved.Columns[0].Type != "integer" {
				t.Fatalf("derived projection lost output identity/type: resolved=%+v diagnostics=%+v", resolved, diagnostics)
			}
		})
	}
}
