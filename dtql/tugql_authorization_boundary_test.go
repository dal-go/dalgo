package dtql

import "testing"

func TestTugQLPublicAuthorizationSourceErrorsPropagateThroughPredicates(t *testing.T) {
	type object = map[string]any
	field := func(source, name string) object { return object{"source": source, "field": name} }
	validRoot := func() object {
		return object{"from": object{"name": "T", "alias": "o"}, "columns": []any{field("o", "Id")}}
	}
	badSourceQuery := func() object {
		return object{"from": object{"name": "Hidden", "alias": "h"}, "columns": []any{field("h", "Id")}}
	}
	for _, tc := range []struct {
		name string
		set  func(object)
	}{
		{"where exists", func(q object) { q["where"] = object{"exists": object{"query": badSourceQuery()}} }},
		{"where and exists", func(q object) { q["where"] = object{"and": []any{object{"exists": object{"query": badSourceQuery()}}}} }},
		{"where or not exists", func(q object) {
			q["where"] = object{"or": []any{object{"notExists": object{"query": badSourceQuery()}}}}
		}},
		{"having exists", func(q object) { q["having"] = object{"exists": object{"query": badSourceQuery()}} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := validRoot()
			tc.set(query)
			document := tugqlTestDocument(&TugQLTree{Format: "tugqtree", Version: 1, Query: query})
			resolved, diagnostics := ResolveTugQL(document, TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "v1", Tables: []TugQLTable{{Name: "T", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}}}}})
			if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "unauthorized_source" {
				t.Fatalf("unauthorized nested source escaped: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
			}
		})
	}
}

func TestTugQLPublicAuthorizationScalarSourceErrorPropagates(t *testing.T) {
	type object = map[string]any
	field := func(source, name string) object { return object{"source": source, "field": name} }
	query := object{
		"from":    object{"name": "T", "alias": "o"},
		"columns": []any{object{"query": object{"from": object{"name": "Hidden", "alias": "h"}, "columns": []any{field("h", "Id")}}, "as": "Value"}},
	}
	document := tugqlTestDocument(&TugQLTree{Format: "tugqtree", Version: 1, Query: query})
	resolved, diagnostics := ResolveTugQL(document, TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "v1", Tables: []TugQLTable{{Name: "T", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}}}}})
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "unauthorized_source" {
		t.Fatalf("unauthorized scalar source escaped: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
	}
}

func TestTugQLPublicAuthorizationChecksJoinedDerivedWhere(t *testing.T) {
	type object = map[string]any
	field := func(source, name string) object { return object{"source": source, "field": name} }
	query := object{
		"from": object{
			"name": "T", "alias": "o",
			"joins": []any{object{
				"from": object{"query": object{
					"as": "d", "from": object{"name": "T", "alias": "i"},
					"columns": []any{field("i", "Id")},
					"where":   object{"op": "==", "left": field("i", "Secret"), "right": object{"value": 1}},
				}},
				"on": []any{object{"op": "==", "left": field("o", "Id"), "right": field("d", "Id")}},
			}},
		},
		"columns": []any{field("o", "Id")},
	}
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "v1", Tables: []TugQLTable{{Name: "T", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}, {Name: "Secret", Type: "integer", Authorized: false}}}}}}}
	resolved, diagnostics := ResolveTugQL(tugqlTestDocument(&TugQLTree{Format: "tugqtree", Version: 1, Query: query}), context)
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "unauthorized_field" {
		t.Fatalf("unauthorized joined-derived WHERE escaped: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
	}
}

func TestTugQLPublicAuthorizationAllowsCorrelatedScalarInJoinedDerivedWhere(t *testing.T) {
	type object = map[string]any
	field := func(source, name string) object { return object{"source": source, "field": name} }
	derived := object{
		"as": "d", "from": object{"name": "T", "alias": "i"},
		"columns": []any{field("i", "Id")},
		"where":   object{"op": "==", "left": field("outer", "Id"), "right": field("i", "Id")},
	}
	inner := object{
		"from": object{"name": "T", "alias": "o", "joins": []any{object{
			"from": object{"query": derived},
			"on":   []any{object{"op": "==", "left": field("o", "Id"), "right": field("d", "Id")}},
		}}},
		"columns": []any{field("d", "Id")},
	}
	query := object{"from": object{"name": "T", "alias": "outer"}, "columns": []any{object{"query": inner, "as": "Value"}}}
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "v1", Tables: []TugQLTable{{Name: "T", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}}}}}
	resolved, diagnostics := ResolveTugQL(tugqlTestDocument(&TugQLTree{Format: "tugqtree", Version: 1, Query: query}), context)
	if resolved.Query == nil || len(diagnostics) != 0 {
		t.Fatalf("authorized correlation through joined-derived WHERE rejected: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
	}
}
