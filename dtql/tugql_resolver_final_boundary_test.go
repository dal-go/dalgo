package dtql

import "testing"

func TestTugQLFinalBoundaryRejectsNestedDuplicateOutputNames(t *testing.T) {
	dupe := func() map[string]any {
		return map[string]any{
			"from": map[string]any{"name": "Inner"},
			"columns": []any{
				map[string]any{"field": "A", "as": "Repeated"},
				map[string]any{"field": "B", "as": "Repeated"},
			},
		}
	}
	scalar := func() map[string]any { return map[string]any{"query": dupe()} }
	base := func() map[string]any { return map[string]any{"from": map[string]any{"name": "Outer"}} }
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"binary operand scalar", func(q map[string]any) {
			q["columns"] = []any{map[string]any{"binary": map[string]any{"op": "+", "left": scalar(), "right": map[string]any{"value": 1}}, "as": "Total"}}
		}},
		{"aggregate argument scalar", func(q map[string]any) {
			q["columns"] = []any{map[string]any{"aggregate": map[string]any{"function": "SUM", "args": []any{scalar()}}, "as": "Total"}}
		}},
		{"aggregate order scalar", func(q map[string]any) {
			q["columns"] = []any{map[string]any{"aggregate": map[string]any{"function": "SUM", "args": []any{map[string]any{"field": "A"}}, "orderBy": []any{scalar()}}, "as": "Total"}}
		}},
		{"where expression scalar", func(q map[string]any) {
			q["where"] = map[string]any{"op": "=", "left": scalar(), "right": map[string]any{"value": 1}}
		}},
		{"and exists", func(q map[string]any) {
			q["where"] = map[string]any{"and": []any{map[string]any{"exists": map[string]any{"query": dupe()}}}}
		}},
		{"or not exists", func(q map[string]any) {
			q["where"] = map[string]any{"or": []any{map[string]any{"notExists": map[string]any{"query": dupe()}}}}
		}},
		{"direct exists", func(q map[string]any) {
			q["where"] = map[string]any{"exists": map[string]any{"query": dupe()}}
		}},
		{"derived source", func(q map[string]any) {
			q["from"] = map[string]any{"query": dupe(), "alias": "Derived"}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := base()
			tc.mutate(query)
			doc := TugQLDocument{
				SourceMetadata: TugQLSourceMetadata{Format: "tugql", Version: 1},
				Tree:           &TugQLTree{Format: "tugqtree", Version: 1, Query: query},
			}
			resolved, diagnostics := ResolveTugQL(doc, TugQLResolveContext{})
			if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "duplicate_output_name" {
				t.Fatalf("nested duplicate projection resolved=%+v diagnostics=%+v", resolved, diagnostics)
			}
		})
	}
}

func TestTugQLFinalBoundaryRejectsUndeclaredParameterInNestedCTEDefinition(t *testing.T) {
	leaf := TugQLDefinition{
		Kind: "cte", Name: "Leaf",
		Query: &TugQLBody{Query: map[string]any{
			"from":    map[string]any{"name": "Inner"},
			"columns": []any{map[string]any{"param": "Missing", "as": "Value"}},
		}},
	}
	outer := TugQLDefinition{
		Kind: "cte", Name: "Outer",
		Query: &TugQLBody{
			Definitions: []TugQLDefinition{leaf},
			Query: map[string]any{
				"from":    map[string]any{"name": "Leaf"},
				"columns": []any{map[string]any{"field": "Value"}},
			},
		},
	}
	doc := TugQLDocument{
		SourceMetadata: TugQLSourceMetadata{Format: "tugql", Version: 1},
		Tree: &TugQLTree{
			Format: "tugqtree", Version: 1,
			Definitions: []TugQLDefinition{outer},
			Query:       map[string]any{"from": map[string]any{"name": "Outer"}, "columns": []any{map[string]any{"field": "Value"}}},
		},
	}
	resolved, diagnostics := ResolveTugQL(doc, TugQLResolveContext{})
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "undeclared_parameter" || diagnostics[0].Message != "parameter is not declared: @Missing" {
		t.Fatalf("nested CTE parameter resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
}

func TestTugQLFinalBoundaryRejectsForwardCTEReferencesInPublicTrees(t *testing.T) {
	cte := func(name, source string) TugQLDefinition {
		return TugQLDefinition{
			Kind: "cte", Name: name,
			Query: &TugQLBody{Query: map[string]any{
				"from":    map[string]any{"name": source},
				"columns": []any{map[string]any{"field": "Id", "as": "Id"}},
			}},
		}
	}
	for _, tc := range []struct {
		name        string
		definitions []TugQLDefinition
	}{
		{"top level", []TugQLDefinition{cte("First", "Later"), cte("Later", "Base")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := TugQLDocument{
				SourceMetadata: TugQLSourceMetadata{Format: "tugql", Version: 1},
				Tree: &TugQLTree{
					Format: "tugqtree", Version: 1,
					Definitions: tc.definitions,
					Query:       map[string]any{"from": map[string]any{"name": "First"}, "columns": []any{map[string]any{"field": "Id"}}},
				},
			}
			resolved, diagnostics := ResolveTugQL(doc, TugQLResolveContext{})
			if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "forward_cte_reference" {
				t.Fatalf("forward CTE reference resolved=%+v diagnostics=%+v", resolved, diagnostics)
			}
		})
	}
}

func TestTugQLFinalBoundaryRejectsDuplicateOutputsInsideCTE(t *testing.T) {
	definition := TugQLDefinition{
		Kind: "cte", Name: "Ambiguous",
		Query: &TugQLBody{Query: map[string]any{
			"from": map[string]any{"name": "Inner"},
			"columns": []any{
				map[string]any{"field": "A", "as": "Repeated"},
				map[string]any{"field": "B", "as": "Repeated"},
			},
		}},
	}
	doc := TugQLDocument{
		SourceMetadata: TugQLSourceMetadata{Format: "tugql", Version: 1},
		Tree: &TugQLTree{
			Format: "tugqtree", Version: 1,
			Definitions: []TugQLDefinition{definition},
			Query:       map[string]any{"from": map[string]any{"name": "Ambiguous"}, "columns": []any{map[string]any{"field": "Repeated"}}},
		},
	}
	resolved, diagnostics := ResolveTugQL(doc, TugQLResolveContext{})
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "duplicate_output_name" {
		t.Fatalf("ambiguous CTE resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
}

func TestTugQLFinalBoundaryRejectsUnauthorizedSourceInScalarLocalCTE(t *testing.T) {
	source := `from ParentRows as o
select (
  Computed as (
    with LocalRows as (
      from Unavailable as u
      select u.Id as Id
    )
    from LocalRows as l
    select l.Id as Id
  )
)`
	doc, parseDiagnostics := ParseTugQL(source)
	if len(parseDiagnostics) != 0 || doc.Tree == nil {
		t.Fatalf("parse diagnostics=%+v tree=%+v", parseDiagnostics, doc.Tree)
	}
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "v1", Tables: []TugQLTable{{Name: "ParentRows", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}}}}}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "unauthorized_source" {
		t.Fatalf("scalar local CTE source resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
}

func TestTugQLFinalBoundaryRejectsDuplicateCTEsInScalarBody(t *testing.T) {
	source := `from ParentRows as p
select (
  Computed as (
    with LocalRows as (
      from ParentRows as l
      select l.Id as Id
    )
    with LocalRows as (
      from ParentRows as r
      select r.Id as Id
    )
    from LocalRows as x
    select x.Id as Id
  )
)`
	doc, parseDiagnostics := ParseTugQL(source)
	if len(parseDiagnostics) != 0 || doc.Tree == nil {
		t.Fatalf("parse diagnostics=%+v tree=%+v", parseDiagnostics, doc.Tree)
	}
	resolved, diagnostics := ResolveTugQL(doc, TugQLResolveContext{})
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "duplicate_definition" {
		t.Fatalf("duplicate scalar CTE resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
}

func TestTugQLFinalBoundaryDelegatesExecutableOperatorValidation(t *testing.T) {
	doc := TugQLDocument{
		SourceMetadata: TugQLSourceMetadata{Format: "tugql", Version: 1},
		Tree: &TugQLTree{Format: "tugqtree", Version: 1, Query: map[string]any{
			"from":    map[string]any{"name": "T"},
			"columns": []any{map[string]any{"field": "Id"}},
			"where": map[string]any{
				"op": "bogus", "left": map[string]any{"field": "Id"}, "right": map[string]any{"value": 1},
			},
		}},
	}
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{
		Version: "v1", Tables: []TugQLTable{{Name: "T", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}},
	}}}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "invalid_resolved_query" || diagnostics[0].Message != `invalid DTQL: unknown comparison operator "bogus"` {
		t.Fatalf("unsupported comparison resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
}
