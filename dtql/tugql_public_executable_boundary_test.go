package dtql

import "testing"

func TestTugQLPublicExecutableFunctionsFailClosedAcrossQueryScopes(t *testing.T) {
	type object = map[string]any
	field := func(source, name string) object {
		if source == "" {
			return object{"field": name}
		}
		return object{"source": source, "field": name}
	}
	call := func() object {
		return object{"call": object{"function": "COALESCE", "args": []any{object{"value": "x"}, object{"value": "y"}}}}
	}
	condition := func(left any) object { return object{"op": "==", "left": left, "right": object{"value": "x"}} }
	equality := func(left, right any) object { return object{"op": "==", "left": left, "right": right} }
	for _, tc := range []struct {
		name  string
		query func() object
	}{
		{"scalar query projection", func() object {
			return object{"from": object{"name": "T", "alias": "o"}, "columns": []any{object{"query": object{"from": object{"name": "T", "alias": "i"}, "columns": []any{object{"call": object{"function": "COALESCE", "args": []any{field("i", "Id"), object{"value": 1}}}, "as": "Value"}}}, "as": "Value"}}}
		}},
		{"AND WHERE operand", func() object {
			return object{"from": object{"name": "T", "alias": "o"}, "columns": []any{field("o", "Id")}, "where": object{"and": []any{condition(call())}}}
		}},
		{"OR WHERE operand", func() object {
			return object{"from": object{"name": "T", "alias": "o"}, "columns": []any{field("o", "Id")}, "where": object{"or": []any{condition(call())}}}
		}},
		{"joined-derived WHERE", func() object {
			return object{
				"from": object{"name": "T", "alias": "o", "joins": []any{object{
					"from": object{"query": object{"as": "d", "from": object{"name": "T", "alias": "i"}, "columns": []any{field("i", "Id")}, "where": condition(call())}},
					"on":   []any{equality(field("o", "Id"), field("d", "Id"))},
				}}},
				"columns": []any{field("o", "Id")},
			}
		}},
		{"joined-derived projection", func() object {
			return object{
				"from": object{"name": "T", "alias": "o", "joins": []any{object{
					"from": object{"query": object{"as": "d", "from": object{"name": "T", "alias": "i"}, "columns": []any{object{"call": object{"function": "COALESCE", "args": []any{field("i", "Id"), object{"value": 1}}}, "as": "Value"}}}},
					"on":   []any{equality(field("o", "Id"), field("d", "Value"))},
				}}},
				"columns": []any{field("o", "Id")},
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "s1", Tables: []TugQLTable{{Name: "T", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}}}}}
			doc := tugqlTestDocument(&TugQLTree{Format: "tugqtree", Version: 1, Query: tc.query()})
			resolved, diagnostics := ResolveTugQL(doc, context)
			if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "unsupported_function" {
				t.Fatalf("non-executable nested function resolved: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
			}
		})
	}
}

func TestTugQLPublicScalarLocalForwardCTEPropagatesAcrossExpressions(t *testing.T) {
	type object = map[string]any
	field := func(source, name string) object {
		if source == "" {
			return object{"field": name}
		}
		return object{"source": source, "field": name}
	}
	forwardScalar := func() object {
		return object{"query": object{
			"query": object{"from": object{"name": "First"}, "columns": []any{field("", "Id")}},
			"definitions": []any{
				object{"kind": "cte", "name": "First", "query": object{"query": object{"from": object{"name": "Later"}, "columns": []any{field("", "Id")}}}},
				object{"kind": "cte", "name": "Later", "query": object{"query": object{"from": object{"name": "T"}, "columns": []any{field("", "Id")}}}},
			},
		}}
	}
	condition := func(expr any) object { return object{"op": "==", "left": expr, "right": object{"value": 1}} }
	for _, tc := range []struct {
		name  string
		query func() object
	}{
		{"binary operand", func() object {
			return object{"from": object{"name": "T"}, "columns": []any{object{"binary": object{"op": "+", "left": forwardScalar(), "right": object{"value": 1}}, "as": "Value"}}}
		}},
		{"aggregate argument", func() object {
			return object{"from": object{"name": "T"}, "columns": []any{object{"aggregate": object{"function": "SUM", "args": []any{forwardScalar()}}, "as": "Value"}}}
		}},
		{"WHERE operand", func() object {
			return object{"from": object{"name": "T"}, "columns": []any{field("", "Id")}, "where": condition(forwardScalar())}
		}},
		{"AND operand", func() object {
			return object{"from": object{"name": "T"}, "columns": []any{field("", "Id")}, "where": object{"and": []any{condition(forwardScalar())}}}
		}},
		{"OR operand", func() object {
			return object{"from": object{"name": "T"}, "columns": []any{field("", "Id")}, "where": object{"or": []any{condition(forwardScalar())}}}
		}},
		{"EXISTS nested projection", func() object {
			return object{"from": object{"name": "T"}, "columns": []any{field("", "Id")}, "where": object{"exists": object{"query": object{"from": object{"name": "T"}, "columns": []any{forwardScalar()}}}}}
		}},
		{"GROUP BY expression", func() object {
			return object{"from": object{"name": "T"}, "columns": []any{field("", "Id")}, "groupBy": []any{forwardScalar()}}
		}},
		{"ORDER BY expression", func() object {
			return object{"from": object{"name": "T"}, "columns": []any{field("", "Id")}, "orderBy": []any{forwardScalar()}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := tugqlTestDocument(&TugQLTree{Format: "tugqtree", Version: 1, Query: tc.query()})
			resolved, diagnostics := ResolveTugQL(doc, TugQLResolveContext{})
			if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "forward_cte_reference" {
				t.Fatalf("forward scalar-local CTE did not fail closed: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
			}
		})
	}
}

func TestTugQLPublicDerivedProjectionRejectsUninferableAggregate(t *testing.T) {
	type object = map[string]any
	query := object{
		"from": object{"query": object{
			"as": "d", "from": object{"name": "T", "alias": "i"},
			"columns": []any{object{"aggregate": object{"function": "BOGUS", "args": []any{object{"source": "i", "field": "Id"}}}, "as": "Value"}},
		}},
		"columns": []any{object{"source": "d", "field": "Value"}},
	}
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "s1", Tables: []TugQLTable{{Name: "T", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}}}}}
	resolved, diagnostics := ResolveTugQL(tugqlTestDocument(&TugQLTree{Format: "tugqtree", Version: 1, Query: query}), context)
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "invalid_cte_output" {
		t.Fatalf("uninferrable derived aggregate result: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
	}
}
