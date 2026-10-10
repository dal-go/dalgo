package dtql

import "testing"

// Every executable position must reject undeclared values before a query can
// reach an executor, including source-free documents and nested scopes.
func TestTugQLUndeclaredParameterCannotEscapeNestedScopes(t *testing.T) {
	type object = map[string]any
	parameter := func() object { return object{"param": "Missing"} }
	field := func() object { return object{"field": "Id"} }
	base := func() object { return object{"from": object{"name": "T"}} }
	nested := func() object {
		query := base()
		query["columns"] = []any{object{"param": "Missing", "as": "Id"}}
		return query
	}
	scalar := func() object { return object{"query": nested(), "as": "Value"} }
	comparison := func() object { return object{"op": "=", "left": field(), "right": parameter()} }
	tests := []struct {
		name   string
		mutate func(object)
	}{
		{"binary-left", func(q object) {
			q["columns"] = []any{object{"binary": object{"op": "+", "left": parameter(), "right": object{"value": 1}}, "as": "Value"}}
		}},
		{"binary-right", func(q object) {
			q["columns"] = []any{object{"binary": object{"op": "+", "left": object{"value": 1}, "right": parameter()}, "as": "Value"}}
		}},
		{"aggregate-argument", func(q object) {
			q["columns"] = []any{object{"aggregate": object{"function": "SUM", "args": []any{parameter()}}, "as": "Value"}}
		}},
		{"aggregate-order", func(q object) {
			q["columns"] = []any{object{"aggregate": object{"function": "SUM", "args": []any{field()}, "orderBy": []any{parameter()}}, "as": "Value"}}
		}},
		{"call-argument", func(q object) {
			q["columns"] = []any{object{"call": object{"function": "COALESCE", "args": []any{parameter(), field()}}, "as": "Value"}}
		}},
		{"scalar-query", func(q object) { q["columns"] = []any{scalar()} }},
		{"scalar-body", func(q object) { q["columns"] = []any{object{"query": object{"query": nested()}, "as": "Value"}} }},
		{"scalar-local-cte", func(q object) {
			q["columns"] = []any{object{"query": object{"query": base(), "definitions": []any{object{"kind": "cte", "name": "Local", "query": object{"query": nested()}}}}, "as": "Value"}}
		}},
		{"group", func(q object) { q["groupBy"] = []any{parameter()} }},
		{"order", func(q object) { q["orderBy"] = []any{parameter()} }},
		{"where-left", func(q object) { q["where"] = object{"op": "=", "left": parameter(), "right": field()} }},
		{"where-right", func(q object) { q["where"] = comparison() }},
		{"is-null", func(q object) { q["where"] = object{"isNull": parameter()} }},
		{"is-not-null", func(q object) { q["where"] = object{"isNotNull": parameter()} }},
		{"having", func(q object) { q["having"] = comparison() }},
		{"and", func(q object) { q["where"] = object{"and": []any{comparison()}} }},
		{"or", func(q object) { q["where"] = object{"or": []any{comparison()}} }},
		{"exists", func(q object) { q["where"] = object{"exists": object{"query": nested()}} }},
		{"not-exists", func(q object) { q["where"] = object{"notExists": object{"query": nested()}} }},
		{"derived-source", func(q object) { q["from"] = object{"query": nested(), "alias": "d"} }},
		{"joined-source", func(q object) {
			q["from"].(object)["joins"] = []any{object{"from": object{"query": nested(), "alias": "d"}}}
		}},
		{"join-on", func(q object) {
			q["from"].(object)["joins"] = []any{object{"from": object{"name": "U"}, "on": []any{comparison()}}}
		}},
		{"nested-join-on", func(q object) {
			q["from"].(object)["joins"] = []any{object{"from": object{"name": "U", "joins": []any{object{"from": object{"name": "V"}, "on": []any{comparison()}}}}}}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			query := base()
			tc.mutate(query)
			document := TugQLDocument{SourceMetadata: TugQLSourceMetadata{Format: "tugql", Version: 1}, Tree: &TugQLTree{Format: "tugqtree", Version: 1, Query: query}}
			resolved, diagnostics := ResolveTugQL(document, TugQLResolveContext{})
			if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "undeclared_parameter" || diagnostics[0].Message != "parameter is not declared: @Missing" {
				t.Fatalf("undeclared binding escaped its scope: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
			}
		})
	}
}
func TestTugQLUnpinnedImportsCannotEscapeNestedScopes(t *testing.T) {
	type object = map[string]any
	base := func() object { return object{"from": object{"name": "T"}} }
	dependency := func() object {
		return object{"query": object{"query": base(), "definitions": []any{object{"kind": "import", "name": "Saved", "path": "./saved.tugql"}}}}
	}
	queryWithDependency := func() object {
		query := base()
		expression := dependency()
		expression["as"] = "Value"
		query["columns"] = []any{expression}
		return query
	}
	comparison := func() object { return object{"op": "=", "left": object{"value": 1}, "right": dependency()} }
	tests := []struct {
		name   string
		mutate func(object)
	}{
		{"selected-scalar", func(q object) { q["columns"] = queryWithDependency()["columns"] }},
		{"binary-left", func(q object) {
			q["columns"] = []any{object{"binary": object{"op": "+", "left": dependency(), "right": object{"value": 1}}, "as": "Value"}}
		}},
		{"binary-right", func(q object) {
			q["columns"] = []any{object{"binary": object{"op": "+", "left": object{"value": 1}, "right": dependency()}, "as": "Value"}}
		}},
		{"aggregate-argument", func(q object) {
			q["columns"] = []any{object{"aggregate": object{"function": "SUM", "args": []any{dependency()}}, "as": "Value"}}
		}},
		{"aggregate-order", func(q object) {
			q["columns"] = []any{object{"aggregate": object{"function": "SUM", "args": []any{object{"field": "Id"}}, "orderBy": []any{dependency()}}, "as": "Value"}}
		}},
		{"nested-scalar", func(q object) { q["columns"] = []any{object{"query": queryWithDependency(), "as": "Value"}} }},
		{"group", func(q object) { q["groupBy"] = []any{dependency()} }},
		{"order", func(q object) { q["orderBy"] = []any{dependency()} }},
		{"where-left", func(q object) { q["where"] = object{"op": "=", "left": dependency(), "right": object{"value": 1}} }},
		{"where-right", func(q object) { q["where"] = comparison() }},
		{"is-null", func(q object) { q["where"] = object{"isNull": dependency()} }},
		{"is-not-null", func(q object) { q["where"] = object{"isNotNull": dependency()} }},
		{"having", func(q object) { q["having"] = comparison() }},
		{"and", func(q object) { q["where"] = object{"and": []any{comparison()}} }},
		{"or", func(q object) { q["where"] = object{"or": []any{comparison()}} }},
		{"exists", func(q object) { q["where"] = object{"exists": object{"query": queryWithDependency()}} }},
		{"not-exists", func(q object) { q["where"] = object{"notExists": object{"query": queryWithDependency()}} }},
		{"derived-source", func(q object) { q["from"] = object{"query": queryWithDependency(), "alias": "d"} }},
		{"joined-source", func(q object) {
			q["from"].(object)["joins"] = []any{object{"from": object{"query": queryWithDependency(), "alias": "d"}}}
		}},
		{"join-on", func(q object) {
			q["from"].(object)["joins"] = []any{object{"from": object{"name": "U"}, "on": []any{comparison()}}}
		}},
	}
	context := TugQLResolveContext{ProjectRoot: "/project", ImportingPath: "main.tugql", ProjectRevision: "r1"}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			query := base()
			tc.mutate(query)
			document := TugQLDocument{SourceMetadata: TugQLSourceMetadata{Format: "tugql", Version: 1}, Tree: &TugQLTree{Format: "tugqtree", Version: 1, Query: query}}
			resolved, diagnostics := ResolveTugQL(document, context)
			if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "unpinned_import" || diagnostics[0].Message != "saved-query import is not pinned to the active project revision: saved.tugql" {
				t.Fatalf("unpinned dependency escaped its scope: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
			}
		})
	}
}
func TestTugQLUnauthorizedFieldsCannotEscapeNestedScopes(t *testing.T) {
	type object = map[string]any
	parameter := func() object { return object{"field": "Secret"} }
	field := func() object { return object{"field": "Id"} }
	base := func() object { return object{"from": object{"name": "T"}} }
	nested := func() object {
		query := base()
		query["columns"] = []any{object{"field": "Secret", "as": "Id"}}
		return query
	}
	scalar := func() object { return object{"query": nested(), "as": "Value"} }
	comparison := func() object { return object{"op": "=", "left": field(), "right": parameter()} }
	tests := []struct {
		name   string
		mutate func(object)
	}{
		{"binary-left", func(q object) {
			q["columns"] = []any{object{"binary": object{"op": "+", "left": parameter(), "right": object{"value": 1}}, "as": "Value"}}
		}},
		{"binary-right", func(q object) {
			q["columns"] = []any{object{"binary": object{"op": "+", "left": object{"value": 1}, "right": parameter()}, "as": "Value"}}
		}},
		{"aggregate-argument", func(q object) {
			q["columns"] = []any{object{"aggregate": object{"function": "SUM", "args": []any{parameter()}}, "as": "Value"}}
		}},
		{"aggregate-order", func(q object) {
			q["columns"] = []any{object{"aggregate": object{"function": "SUM", "args": []any{field()}, "orderBy": []any{parameter()}}, "as": "Value"}}
		}},

		{"scalar-query", func(q object) { q["columns"] = []any{scalar()} }},
		{"scalar-body", func(q object) { q["columns"] = []any{object{"query": object{"query": nested()}, "as": "Value"}} }},
		{"scalar-local-cte", func(q object) {
			q["columns"] = []any{object{"query": object{"query": base(), "definitions": []any{object{"kind": "cte", "name": "Local", "query": object{"query": nested()}}}}, "as": "Value"}}
		}},
		{"group", func(q object) { q["groupBy"] = []any{parameter()} }},
		{"order", func(q object) { q["orderBy"] = []any{parameter()} }},
		{"where-left", func(q object) { q["where"] = object{"op": "=", "left": parameter(), "right": field()} }},
		{"where-right", func(q object) { q["where"] = comparison() }},
		{"is-null", func(q object) { q["where"] = object{"isNull": parameter()} }},
		{"is-not-null", func(q object) { q["where"] = object{"isNotNull": parameter()} }},
		{"having", func(q object) { q["having"] = comparison() }},
		{"and", func(q object) { q["where"] = object{"and": []any{comparison()}} }},
		{"or", func(q object) { q["where"] = object{"or": []any{comparison()}} }},
		{"exists", func(q object) { q["where"] = object{"exists": object{"query": nested()}} }},
		{"not-exists", func(q object) { q["where"] = object{"notExists": object{"query": nested()}} }},
		{"derived-source", func(q object) { q["from"] = object{"query": nested(), "alias": "d"} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			query := base()
			tc.mutate(query)
			document := TugQLDocument{SourceMetadata: TugQLSourceMetadata{Format: "tugql", Version: 1}, Tree: &TugQLTree{Format: "tugqtree", Version: 1, Query: query}}
			resolved, diagnostics := ResolveTugQL(document, TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "v1", Tables: []TugQLTable{{Name: "T", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}, {Name: "Secret", Type: "integer", Authorized: false}}}}}}})
			if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "unauthorized_field" {
				t.Fatalf("unauthorized field escaped its scope: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
			}
		})
	}
}
