package dtql

import (
	"reflect"
	"testing"
)

func TestTugQLOutputBoundaryRejectsUnsafeInference(t *testing.T) {
	sources := []tugqlResolvedSource{{alias: "i", fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}, {Name: "Label", Type: "string", Authorized: true}}}}
	integer := exprYAML{Field: "Id", Source: "i"}
	text := exprYAML{Field: "Label", Source: "i"}
	unknown := exprYAML{Field: "Missing", Source: "i"}
	for _, tc := range []struct {
		name       string
		expression exprYAML
	}{
		{"scalar function", exprYAML{tugqlCall: &tugqlCallYAML{Function: "CUSTOM"}}},
		{"unavailable field", unknown},
		{"mixed arithmetic", exprYAML{Binary: &binaryYAML{Op: "+", Left: &integer, Right: &text}}},
		{"aggregate arity", exprYAML{Aggregate: &aggregateYAML{Function: "SUM", Args: []exprYAML{integer, integer}}}},
		{"aggregate missing field", exprYAML{Aggregate: &aggregateYAML{Function: "SUM", Args: []exprYAML{unknown}}}},
		{"average text", exprYAML{Aggregate: &aggregateYAML{Function: "AVG", Args: []exprYAML{text}}}},
		{"scalar two columns", exprYAML{Query: &document{Columns: []columnYAML{{exprYAML: integer}, {exprYAML: text}}}}},
		{"scalar unauthorized source", exprYAML{Query: &document{From: fromYAML{Name: "Unavailable"}, Columns: []columnYAML{{exprYAML: integer}}}}},
		{"unbound parameter", exprYAML{Param: "NotBound"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name, kind, lineage, ok := tugqlInferOutputExpression(tc.expression, sources, TugQLResolveContext{})
			if ok || name != "" || kind != "" || lineage != nil {
				t.Fatalf("unsafe expression yielded output: name=%q type=%q lineage=%v ok=%v", name, kind, lineage, ok)
			}
		})
	}
	columns, diagnostics := tugqlOutputColumns([]columnYAML{{exprYAML: unknown}}, sources, TugQLResolveContext{})
	if columns != nil || len(diagnostics) != 1 || diagnostics[0].Code != "unauthorized_field" {
		t.Fatalf("unavailable output must fail: columns=%v diagnostics=%v", columns, diagnostics)
	}
}

func TestTugQLOutputBoundaryRequiresNamesForComputedColumns(t *testing.T) {
	sources := []tugqlResolvedSource{{alias: "i", fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}}
	field := exprYAML{Field: "Id", Source: "i"}
	value := exprYAML{Value: valuePtr(any(int64(1)))}
	computed := exprYAML{Binary: &binaryYAML{Op: "+", Left: ptrExpr(field), Right: ptrExpr(value)}}

	for _, tc := range []struct {
		name     string
		column   columnYAML
		wantName string
		wantDiag string
	}{
		{name: "field infers name", column: columnYAML{exprYAML: field}, wantName: "Id"},
		{name: "aggregate infers function name", column: columnYAML{exprYAML: exprYAML{Aggregate: &aggregateYAML{Function: "SUM", Args: []exprYAML{field}}}}, wantName: "sum"},
		{name: "literal requires alias", column: columnYAML{exprYAML: value}, wantDiag: "invalid_projection"},
		{name: "arithmetic requires alias", column: columnYAML{exprYAML: computed}, wantDiag: "invalid_projection"},
		{name: "aliased arithmetic accepted", column: columnYAML{exprYAML: computed, As: "Raised"}, wantName: "Raised"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			columns, diagnostics := tugqlOutputColumns([]columnYAML{tc.column}, sources, TugQLResolveContext{})
			if tc.wantDiag != "" {
				if columns != nil || len(diagnostics) != 1 || diagnostics[0].Code != tc.wantDiag || diagnostics[0].Message != "every computed output column requires an alias" {
					t.Fatalf("columns=%+v diagnostics=%+v", columns, diagnostics)
				}
				return
			}
			if len(diagnostics) != 0 || len(columns) != 1 || columns[0].Name != tc.wantName {
				t.Fatalf("columns=%+v diagnostics=%+v", columns, diagnostics)
			}
		})
	}
	_, diagnostics := tugqlOutputColumns([]columnYAML{
		{exprYAML: exprYAML{Aggregate: &aggregateYAML{Function: "COUNT", Args: []exprYAML{{Star: true}}}}},
		{exprYAML: exprYAML{Aggregate: &aggregateYAML{Function: "COUNT", Args: []exprYAML{{Star: true}}}}},
	}, sources, TugQLResolveContext{})
	if len(diagnostics) != 1 || diagnostics[0].Code != "duplicate_output_name" {
		t.Fatalf("same-name aggregates should require explicit distinct aliases: %+v", diagnostics)
	}
}

func TestTugQLOutputBoundaryRejectsAmbiguousUnqualifiedField(t *testing.T) {
	field := TugQLField{Name: "Id", Type: "integer", Authorized: true}
	sources := []tugqlResolvedSource{{alias: "a", fields: []TugQLField{field}}, {alias: "b", fields: []TugQLField{field}}}
	if _, _, _, ok := tugqlOutputField(exprYAML{Field: "Id"}, sources); ok {
		t.Fatal("ambiguous Id must not select an arbitrary source")
	}
	name, alias, kind, ok := tugqlOutputField(exprYAML{Field: "Id", Source: "b"}, sources)
	if !ok || name != "Id" || alias != "b" || kind != "integer" {
		t.Fatalf("qualification did not resolve ambiguity: %q %q %q %v", name, alias, kind, ok)
	}
}

func TestTugQLOutputBoundaryIgnoresUnrelatedRelationshipReceipts(t *testing.T) {
	sources := []tugqlResolvedSource{{alias: "a"}, {alias: "b"}}
	for _, relationship := range []TugQLRelationshipExpansion{
		{FromSource: "missing", ToSource: "b", JoinType: "inner", Pairs: []TugQLRelationshipPair{{FromField: "Id", ToField: "Id"}}},
		{FromSource: "a", ToSource: "a", JoinType: "inner", Pairs: []TugQLRelationshipPair{{FromField: "Id", ToField: "Id"}}},
	} {
		merged, skipped := tugqlDefaultProjectionKeys(sources, []TugQLRelationshipExpansion{relationship})
		if len(merged) != 0 || len(skipped) != 0 {
			t.Fatalf("unrelated receipt affected projection: merged=%v skipped=%v", merged, skipped)
		}
	}
}

func TestTugQLOutputBoundaryMergesTransitiveRelationshipKeys(t *testing.T) {
	sources := []tugqlResolvedSource{
		{alias: "a", fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}},
		{alias: "b", fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}},
		{alias: "c", fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}},
	}
	relationships := []TugQLRelationshipExpansion{
		{ID: "ab", Version: "r1", FromSource: "a", ToSource: "b", JoinType: "inner", Pairs: []TugQLRelationshipPair{{FromField: "Id", ToField: "Id"}}},
		{ID: "bc", Version: "r1", FromSource: "b", ToSource: "c", JoinType: "inner", Pairs: []TugQLRelationshipPair{{FromField: "Id", ToField: "Id"}}},
	}
	merged, skipped := tugqlDefaultProjectionKeys(sources, relationships)
	wantLineage := []TugQLOutputLineage{{Source: "a", Field: "Id"}, {Source: "b", Field: "Id"}, {Source: "c", Field: "Id"}}
	if !reflect.DeepEqual(merged["a\x00Id"], wantLineage) || !skipped["b\x00Id"] || !skipped["c\x00Id"] {
		t.Fatalf("transitive exact-equality keys were not merged: merged=%v skipped=%v", merged, skipped)
	}
}

func TestTugQLOutputBoundaryKeepsOrderedUniqueLineage(t *testing.T) {
	a := TugQLOutputLineage{Source: "a", Field: "Id"}
	b := TugQLOutputLineage{Source: "b", Field: "Id"}
	left := []TugQLOutputLineage{a}
	got := mergeTugQLLineage(left, []TugQLOutputLineage{a, b, b})
	if !reflect.DeepEqual(got, []TugQLOutputLineage{a, b}) || !reflect.DeepEqual(left, []TugQLOutputLineage{a}) {
		t.Fatalf("lineage order, uniqueness or source immutability lost: got=%v original=%v", got, left)
	}
}

func TestTugQLOutputBoundaryQualifiedWildcardSkipsOtherSources(t *testing.T) {
	sources := []tugqlResolvedSource{
		{alias: "a", fields: []TugQLField{{Name: "Wrong", Type: "integer", Authorized: true}}},
		{alias: "b", fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}, {Name: "Secret", Authorized: false}, {Name: "Omit", Authorized: true}}},
	}
	got := expandTugQLWildcardColumns([]columnYAML{{Wildcard: &wildcardYAML{Source: "b", Exclude: []string{"Omit"}}}}, sources)
	if len(got) != 1 || got[0].Field != "Id" || got[0].Source != "b" || got[0].As != "Id" {
		t.Fatalf("qualified wildcard exposed wrong field: %+v", got)
	}
}
