package dtql

import "testing"

func TestTugQLPublicJoinedDerivedErrorsPropagate(t *testing.T) {
	type object = map[string]any
	field := func(source, name string) object {
		if source == "" {
			return object{"field": name}
		}
		return object{"source": source, "field": name}
	}
	equality := func(left, right any) object { return object{"op": "==", "left": left, "right": right} }
	duplicate := func() object {
		return object{"as": "d", "from": object{"name": "T", "alias": "i"}, "columns": []any{
			object{"source": "i", "field": "Id", "as": "Repeated"},
			object{"source": "i", "field": "Id", "as": "Repeated"},
		}}
	}
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "v1", Tables: []TugQLTable{{Name: "T", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}}}}}
	for _, tc := range []struct {
		name  string
		query object
		code  string
	}{
		{"joined derived duplicate outputs", object{
			"from": object{"name": "T", "alias": "o", "joins": []any{object{
				"from": object{"query": duplicate(), "alias": "d"},
				"on":   []any{equality(field("o", "Id"), field("d", "Repeated"))},
			}}},
			"columns": []any{field("o", "Id")},
		}, "duplicate_output_name"},
		{"nested joined derived duplicate outputs", object{
			"from": object{"name": "T", "alias": "o", "joins": []any{object{
				"from": object{"name": "T", "alias": "m", "joins": []any{object{
					"from": object{"query": duplicate(), "alias": "d"},
					"on":   []any{equality(field("m", "Id"), field("d", "Repeated"))},
				}}},
				"on": []any{equality(field("o", "Id"), field("m", "Id"))},
			}}},
			"columns": []any{field("o", "Id")},
		}, "duplicate_output_name"},
		{"joined derived uninferable aggregate output", object{
			"from": object{"name": "T", "alias": "o", "joins": []any{object{
				"from": object{"query": object{"as": "d", "from": object{"name": "T", "alias": "i"}, "columns": []any{object{
					"aggregate": object{"function": "BOGUS", "args": []any{field("i", "Id")}}, "as": "Value",
				}}}, "alias": "d"},
				"on": []any{equality(field("o", "Id"), field("d", "Value"))},
			}}},
			"columns": []any{field("o", "Id")},
		}, "invalid_cte_output"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := tugqlTestDocument(&TugQLTree{Format: "tugqtree", Version: 1, Query: tc.query})
			resolved, diagnostics := ResolveTugQL(doc, context)
			if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != tc.code {
				t.Fatalf("joined-derived diagnostic mismatch: query=%v diagnostics=%+v want=%s", resolved.Query != nil, diagnostics, tc.code)
			}
		})
	}
}

func TestTugQLPublicJoinedUnauthorizedSourcesPropagate(t *testing.T) {
	type object = map[string]any
	field := func(source, name string) object { return object{"source": source, "field": name} }
	equality := func(left, right any) object { return object{"op": "==", "left": left, "right": right} }
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "v1", Tables: []TugQLTable{{Name: "T", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}}}}}
	cases := []struct {
		name  string
		query object
	}{
		{"root join", object{
			"from": object{"name": "T", "alias": "o", "joins": []any{object{
				"from": object{"name": "Hidden", "alias": "h"},
				"on":   []any{equality(field("o", "Id"), field("h", "Id"))},
			}}},
			"columns": []any{field("o", "Id")},
		}},
		{"derived query with join", object{
			"from": object{"query": object{"as": "d", "from": object{"name": "T", "alias": "i", "joins": []any{object{
				"from": object{"name": "Hidden", "alias": "h"},
				"on":   []any{equality(field("i", "Id"), field("h", "Id"))},
			}}}, "columns": []any{field("i", "Id")}}, "alias": "d"},
			"columns": []any{field("d", "Id")},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := tugqlTestDocument(&TugQLTree{Format: "tugqtree", Version: 1, Query: tc.query})
			resolved, diagnostics := ResolveTugQL(doc, context)
			if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "unauthorized_source" {
				t.Fatalf("joined unauthorized source result: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
			}
		})
	}
}

func TestTugQLPublicScalarLocalForwardCTEsPropagateThroughNestedFroms(t *testing.T) {
	type object = map[string]any
	field := func(source, name string) object {
		if source == "" {
			return object{"field": name}
		}
		return object{"source": source, "field": name}
	}
	equality := func(left, right any) object { return object{"op": "==", "left": left, "right": right} }
	definitions := func(firstQuery object) []any {
		return []any{
			object{"kind": "cte", "name": "First", "query": object{"query": firstQuery}},
			object{"kind": "cte", "name": "Later", "query": object{"query": object{"from": object{"name": "T"}, "columns": []any{field("", "Id")}}}},
		}
	}
	scalarColumn := func(query, firstQuery object) object {
		expression := object{"query": object{"query": query, "definitions": definitions(firstQuery)}}
		expression["as"] = "Value"
		return expression
	}
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "v1", Tables: []TugQLTable{{Name: "T", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}}}}}
	cases := []struct {
		name string
		doc  TugQLDocument
	}{
		{"scalar inside CTE body", func() TugQLDocument {
			return TugQLDocument{SourceMetadata: TugQLSourceMetadata{Format: "tugql", Version: 1}, Tree: &TugQLTree{
				Format: "tugqtree", Version: 1,
				Definitions: []TugQLDefinition{{Kind: "cte", Name: "Wrapper", Query: &TugQLBody{Query: object{"from": object{"name": "T"}, "columns": []any{scalarColumn(
					object{"from": object{"name": "T"}, "columns": []any{field("", "Id")}},
					object{"from": object{"name": "Later"}, "columns": []any{field("", "Id")}},
				)}}}}},
				Query: object{"from": object{"name": "Wrapper"}, "columns": []any{field("", "Id")}},
			}}
		}()},
		{"scalar local CTE reference in derived FROM", func() TugQLDocument {
			inner := object{"from": object{"name": "T"}, "columns": []any{field("", "Id")}}
			first := object{"from": object{"query": object{"from": object{"name": "Later"}, "columns": []any{field("", "Id")}}, "alias": "d"}, "columns": []any{field("d", "Id")}}
			return tugqlTestDocument(&TugQLTree{Format: "tugqtree", Version: 1, Query: object{"from": object{"name": "T"}, "columns": []any{scalarColumn(inner, first)}}})
		}()},
		{"scalar local CTE reference in joined-derived FROM", func() TugQLDocument {
			inner := object{"from": object{"name": "T"}, "columns": []any{field("", "Id")}}
			first := object{"from": object{"name": "T", "alias": "o", "joins": []any{object{
				"from": object{"query": object{"from": object{"name": "Later"}, "columns": []any{field("", "Id")}}, "alias": "d"},
				"on":   []any{equality(field("o", "Id"), field("d", "Id"))},
			}}}, "columns": []any{field("o", "Id")}}
			return tugqlTestDocument(&TugQLTree{Format: "tugqtree", Version: 1, Query: object{"from": object{"name": "T"}, "columns": []any{scalarColumn(inner, first)}}})
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolved, diagnostics := ResolveTugQL(tc.doc, context)
			if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "forward_cte_reference" {
				t.Fatalf("nested forward CTE result: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
			}
		})
	}
}
