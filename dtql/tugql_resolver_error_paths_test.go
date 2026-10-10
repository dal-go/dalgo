package dtql

import (
	"fmt"
	"testing"
)

func TestTugQLPublicCTEExpansionPropagatesNestedDiagnostics(t *testing.T) {
	type object = map[string]any
	field := func(source, name string) object {
		if source == "" {
			return object{"field": name}
		}
		return object{"source": source, "field": name}
	}
	cte := func(name string, query object) object {
		return object{"kind": "cte", "name": name, "query": object{"query": query}}
	}
	baseContext := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "s1", Tables: []TugQLTable{
		{Name: "T", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}},
		{Name: "U", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}},
	}}}}
	query := func(query object) TugQLDocument {
		return tugqlTestDocument(&TugQLTree{Format: "tugqtree", Version: 1, Query: query})
	}
	tests := []struct {
		name string
		doc  TugQLDocument
		code string
		ctx  TugQLResolveContext
	}{
		{
			name: "scalar-local CTE inside a definition",
			doc: func() TugQLDocument {
				scalar := object{"query": object{"from": object{"name": "First"}, "columns": []any{field("", "Id")}}, "definitions": []any{
					cte("First", object{"from": object{"name": "Later"}, "columns": []any{field("", "Id")}}),
					cte("Later", object{"from": object{"name": "T"}, "columns": []any{field("", "Id")}}),
				}}
				root := query(object{"from": object{"name": "Wrapper"}, "columns": []any{field("", "Id")}})
				root.Tree.Definitions = []TugQLDefinition{{Kind: "cte", Name: "Wrapper", Query: &TugQLBody{Query: map[string]any{
					"from": object{"name": "T"}, "columns": []any{object{"query": scalar, "as": "Value"}},
				}}}}
				return root
			}(),
			code: "forward_cte_reference", ctx: baseContext,
		},
		{
			name: "unresolved relationship in a definition",
			doc: func() TugQLDocument {
				root := query(object{"from": object{"name": "Joined"}, "columns": []any{field("", "Id")}})
				root.Tree.Definitions = []TugQLDefinition{{Kind: "cte", Name: "Joined", Query: &TugQLBody{Query: map[string]any{
					"from":    object{"name": "T", "alias": "t", "joins": []any{object{"from": object{"name": "U", "alias": "u"}}}},
					"columns": []any{field("t", "Id")},
				}}}}
				return root
			}(),
			code: "relationship_not_found", ctx: baseContext,
		},
		{
			name: "EXISTS references a later sibling definition",
			doc: func() TugQLDocument {
				root := query(object{"from": object{"name": "First"}, "columns": []any{field("", "Id")}})
				first := object{"from": object{"name": "T"}, "where": object{"exists": object{"query": object{"from": object{"name": "Later"}, "columns": []any{field("", "Id")}}}}, "columns": []any{field("", "Id")}}
				later := object{"from": object{"name": "T"}, "columns": []any{field("", "Id")}}
				root.Tree.Definitions = []TugQLDefinition{{Kind: "cte", Name: "First", Query: &TugQLBody{Query: first}}, {Kind: "cte", Name: "Later", Query: &TugQLBody{Query: later}}}
				return root
			}(),
			code: "forward_cte_reference", ctx: baseContext,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolved, diagnostics := ResolveTugQL(tc.doc, tc.ctx)
			if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != tc.code {
				t.Fatalf("nested CTE error did not propagate: query=%v diagnostics=%+v, want %s", resolved.Query != nil, diagnostics, tc.code)
			}
		})
	}
}

func TestTugQLScalarExpansionPropagatesForwardReferences(t *testing.T) {
	query := document{From: fromYAML{Name: "T"}, Columns: []columnYAML{{exprYAML: exprYAML{
		tugqlQueryBody: &tugqlBody{Query: document{From: fromYAML{Name: "Later"}}},
	}}}}
	diagnostics := expandTugQLScalarBodies(&query, map[string]tugqlBody{}, map[string]bool{"Later": true}, TugQLResolveContext{}, nil)
	if len(diagnostics) != 1 || diagnostics[0].Code != "forward_cte_reference" {
		t.Fatalf("scalar forward-reference diagnostic=%+v", diagnostics)
	}
}

func TestTugQLCTEExpansionPropagatesScalarDefinitionErrors(t *testing.T) {
	localDefinitions := []tugqlDefinition{
		{Kind: "cte", Name: "First", Query: &tugqlBody{Query: document{From: fromYAML{Name: "Later"}}}},
		{Kind: "cte", Name: "Later", Query: &tugqlBody{Query: document{From: fromYAML{Name: "T"}}}},
	}
	root := document{From: fromYAML{Name: "T"}, Columns: []columnYAML{{exprYAML: exprYAML{
		tugqlQueryBody: &tugqlBody{Query: document{From: fromYAML{Name: "T"}}, Definitions: localDefinitions},
	}}}}
	definitions := []tugqlDefinition{{Kind: "cte", Name: "Wrapper", Query: &tugqlBody{Query: root}}}
	root = document{From: fromYAML{Name: "Wrapper"}}
	diagnostics := expandTugQLCTEs(&root, definitions, TugQLResolveContext{}, nil)
	if len(diagnostics) != 1 || diagnostics[0].Code != "forward_cte_reference" {
		t.Fatalf("nested scalar definition diagnostic=%+v", diagnostics)
	}
}

func TestTugQLPublicNestedJoinAuthorizationFailurePropagates(t *testing.T) {
	type object = map[string]any
	field := func(source, name string) object { return object{"source": source, "field": name} }
	equality := func(left, right any) object { return object{"op": "==", "left": left, "right": right} }
	derived := object{
		"from": object{"name": "T", "alias": "i", "joins": []any{object{
			"from": object{"name": "U", "alias": "u"},
			"on":   []any{equality(field("i", "Secret"), field("u", "Id"))},
		}}},
		"columns": []any{field("i", "Id")},
	}
	query := object{
		"from": object{"name": "T", "alias": "o", "joins": []any{object{
			"from": object{"query": derived, "alias": "d"},
			"on":   []any{equality(field("o", "Id"), field("d", "Id"))},
		}}},
		"columns": []any{field("o", "Id")},
	}
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "s1", Tables: []TugQLTable{
		{Name: "T", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}, {Name: "Secret", Type: "integer", Authorized: false}}},
		{Name: "U", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}},
	}}}}
	resolved, diagnostics := ResolveTugQL(tugqlTestDocument(&TugQLTree{Format: "tugqtree", Version: 1, Query: query}), context)
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "unauthorized_field" {
		t.Fatalf("nested join authorization failure did not propagate: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
	}
}

func TestTugQLPublicNestedQuerySourceFailurePropagates(t *testing.T) {
	type object = map[string]any
	field := func(source, name string) object { return object{"source": source, "field": name} }
	equality := func(left, right any) object { return object{"op": "==", "left": left, "right": right} }
	badAggregate := object{"query": object{"from": object{"name": "T", "alias": "b"}, "columns": []any{
		object{"aggregate": object{"function": "BOGUS", "args": []any{field("b", "Id")}}, "as": "Value"},
	}}, "alias": "bad"}
	joinedSource := object{"query": object{"from": object{"name": "T", "alias": "i"}, "columns": []any{field("i", "Id")}}, "alias": "d", "joins": []any{
		object{"from": badAggregate, "on": []any{equality(field("d", "Id"), field("bad", "Value"))}},
	}}
	query := object{"from": object{"name": "T", "alias": "o", "joins": []any{
		object{"from": joinedSource, "on": []any{equality(field("o", "Id"), field("d", "Id"))}},
	}}, "columns": []any{field("o", "Id")}}
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "s1", Tables: []TugQLTable{{Name: "T", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}}}}}
	resolved, diagnostics := ResolveTugQL(tugqlTestDocument(&TugQLTree{Format: "tugqtree", Version: 1, Query: query}), context)
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "invalid_cte_output" {
		t.Fatalf("nested derived source error did not propagate: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
	}
}

func TestTugQLPublicExistsExpansionEnforcesExpandedBodyBudget(t *testing.T) {
	const max = maxTugQLSemanticNodes
	columns := make([]columnYAML, 0, 2000)
	body := document{From: fromYAML{Name: "T"}}
	for len(columns) < cap(columns) {
		columns = append(columns, columnYAML{exprYAML: exprYAML{Field: "Id"}, As: fmt.Sprintf("Field%d", len(columns))})
		body.Columns = columns
		if tugqlDocumentNodeCount(&body, max) >= max-1 {
			break
		}
	}
	bodyNodes := tugqlDocumentNodeCount(&body, max)
	if bodyNodes >= max {
		t.Fatalf("fixture body must remain individually within the limit: %d", bodyNodes)
	}
	root := document{From: fromYAML{Name: "T"}, Where: &condYAML{Exists: &existsYAML{Query: &document{From: fromYAML{Name: "Large"}}}}}
	all := map[string]bool{"Large": true}
	earlier := map[string]tugqlBody{"Large": {Query: body}}
	diagnostics := expandTugQLScalarBodies(&root, earlier, all, TugQLResolveContext{}, nil)
	if len(diagnostics) == 0 || diagnostics[0].Code != "document_node_limit" {
		t.Fatalf("expanded EXISTS body budget result=%+v, expected document_node_limit (base=%d)", diagnostics, bodyNodes)
	}
}
