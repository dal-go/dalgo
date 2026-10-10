package dtql

import (
	"strconv"
	"strings"
	"testing"
)

func TestTugQLParseExpressionDepthBoundary(t *testing.T) {
	terms := func(count int) string {
		values := make([]string, count)
		for i := range values {
			values[i] = "1"
		}
		return strings.Join(values, " + ")
	}

	acceptedSource := "from T\nselect " + terms(63) + " as Total"
	accepted, diagnostics := ParseTugQL(acceptedSource)
	if len(diagnostics) != 0 || accepted.Tree == nil {
		t.Fatalf("63 terms must fit the canonical depth limit: tree=%v diagnostics=%+v", accepted.Tree != nil, diagnostics)
	}

	rejectedSource := "from T\nselect " + terms(64) + " as Total"
	rejected, diagnostics := ParseTugQL(rejectedSource)
	if rejected.Tree != nil || rejected.Source != rejectedSource || len(diagnostics) != 1 {
		t.Fatalf("64 terms must fail atomically and preserve source: tree=%v diagnostics=%+v", rejected.Tree != nil, diagnostics)
	}
	if got := diagnostics[0]; got.Code != "invalid_select" || got.Message != "expression nesting exceeds 128" || got.Span.Start != (TugQLPosition{Line: 2, Column: 1}) || got.Span.End != (TugQLPosition{Line: 2, Column: 270}) {
		t.Fatalf("unexpected boundary diagnostic: %+v", got)
	}

	deepFilter := "from T\nwhere " + terms(64) + " > 0"
	filtered, diagnostics := ParseTugQL(deepFilter)
	if filtered.Tree != nil || len(diagnostics) != 1 {
		t.Fatalf("over-depth filter must fail atomically: tree=%v diagnostics=%+v", filtered.Tree != nil, diagnostics)
	}
	if got := diagnostics[0]; got.Code != "document_depth_exceeded" || got.Message != "TugQL semantic tree depth exceeds 128" || got.Span.Start != (TugQLPosition{Line: 1, Column: 1}) || got.Span.End != (TugQLPosition{Line: 1, Column: 7}) {
		t.Fatalf("unexpected generic depth diagnostic: %+v", got)
	}

	cteSource := "with C as (\n  from T\n  select " + terms(64) + " as Total\n)\nfrom C\nselect Total"
	cteDocument, diagnostics := ParseTugQL(cteSource)
	if cteDocument.Tree != nil || len(diagnostics) != 1 || diagnostics[0].Code != "document_depth_exceeded" || diagnostics[0].Span.Start != (TugQLPosition{Line: 5, Column: 1}) {
		t.Fatalf("nested CTE overflow should use the root query span: tree=%v diagnostics=%+v", cteDocument.Tree != nil, diagnostics)
	}

	// A scalar adds two canonical wrapper edges (query body and query) before
	// the nested projection. Sixty-one terms fit; sixty-two exceed depth 128.
	scalarWithin := "from T\nselect (\n  Total as (\n    from U\n    select " + terms(61) + " as Total\n  )\n)"
	withinDocument, withinDiagnostics := ParseTugQL(scalarWithin)
	if len(withinDiagnostics) != 0 || withinDocument.Tree == nil {
		t.Fatalf("61-term scalar projection must fit the canonical depth limit: tree=%v diagnostics=%+v", withinDocument.Tree != nil, withinDiagnostics)
	}
	scalarAtLimit := "from T\nselect (\n  Total as (\n    from U\n    select " + terms(62) + " as Total\n  )\n)"
	atLimitDocument, atLimitDiagnostics := ParseTugQL(scalarAtLimit)
	if atLimitDocument.Tree != nil || len(atLimitDiagnostics) != 1 || atLimitDiagnostics[0].Code != "invalid_select" || atLimitDiagnostics[0].Message != "expression nesting exceeds 128" || atLimitDiagnostics[0].Span.Start != (TugQLPosition{Line: 2, Column: 1}) {
		t.Fatalf("62-term scalar projection should exceed the canonical depth limit at root SELECT: tree=%v diagnostics=%+v", atLimitDocument.Tree != nil, atLimitDiagnostics)
	}

	scalarCTE := "from T\nselect (\n  Total as (\n    with inner_query as (\n      from U\n      select " + terms(61) + " as Total\n    )\n    from inner_query\n    select Total\n  )\n)"
	scalarCTEDocument, scalarCTEDiagnostics := ParseTugQL(scalarCTE)
	if scalarCTEDocument.Tree != nil || len(scalarCTEDiagnostics) != 1 || scalarCTEDiagnostics[0].Code != "invalid_select" || scalarCTEDiagnostics[0].Message != "expression nesting exceeds 128" || scalarCTEDiagnostics[0].Span.Start != (TugQLPosition{Line: 2, Column: 1}) {
		t.Fatalf("scalar-local CTE overflow should retain root SELECT attribution: tree=%v diagnostics=%+v", scalarCTEDocument.Tree != nil, scalarCTEDiagnostics)
	}

	scalarSource := "from T\nselect (\n  Total as (\n    from U\n    select " + terms(64) + " as Total\n  )\n)"
	scalarDocument, diagnostics := ParseTugQL(scalarSource)
	if scalarDocument.Tree != nil || len(diagnostics) != 1 || diagnostics[0].Code != "invalid_select" || diagnostics[0].Span.Start != (TugQLPosition{Line: 2, Column: 1}) {
		t.Fatalf("nested scalar overflow should use the root SELECT span: tree=%v diagnostics=%+v", scalarDocument.Tree != nil, diagnostics)
	}
}

func TestTugQLParseNodeBudgetBeforeExport(t *testing.T) {
	plainColumns := make([]string, 2000)
	for i := range plainColumns {
		plainColumns[i] = "Id"
	}
	plainSource := "from T\nselect (\n  " + strings.Join(plainColumns, "\n  ") + "\n)"
	plainDocument, plainDiagnostics := ParseTugQL(plainSource)
	if len(plainDiagnostics) != 0 || plainDocument.Tree == nil {
		t.Fatalf("large unaliased projection below the canonical node budget must parse: tree=%v diagnostics=%+v", plainDocument.Tree != nil, plainDiagnostics)
	}

	columns := make([]string, 2501)
	for i := range columns {
		columns[i] = "Id as C" + strconv.Itoa(i)
	}
	source := "from T\nselect (\n  " + strings.Join(columns, "\n  ") + "\n)"
	document, diagnostics := ParseTugQL(source)
	if document.Tree != nil || document.Source != source || len(diagnostics) != 1 {
		t.Fatalf("large semantic tree must fail before export and preserve source: tree=%v diagnostics=%+v", document.Tree != nil, diagnostics)
	}
	if diagnostics[0].Code != "document_node_limit" {
		t.Fatalf("expected node budget diagnostic, got %+v", diagnostics[0])
	}
}

func TestTugQLPreflightCoversCanonicalNodeShapes(t *testing.T) {
	value := any("x")
	leaf := exprYAML{Field: "Id", Source: "t"}
	nested := document{From: fromYAML{Name: "Nested"}, Columns: []columnYAML{{exprYAML: leaf}}}
	call := tugqlCallYAML{Function: "COALESCE", Args: []exprYAML{leaf, {Value: &value}}}
	body := tugqlBody{Query: nested, Definitions: []tugqlDefinition{{Kind: "cte", Name: "Inner", Query: &tugqlBody{Query: nested}}}}
	expressions := []exprYAML{
		{Binary: &binaryYAML{Op: "+", Left: &leaf, Right: &exprYAML{Value: &value}}},
		{Aggregate: &aggregateYAML{Function: "COUNT", Args: []exprYAML{leaf}, OrderBy: []orderYAML{{exprYAML: leaf}}}},
		{Query: &nested},
		{tugqlQueryBody: &body, Query: &nested},
		{tugqlCall: &call},
		{Field: "Id", Source: "t"},
		{Value: &value},
		{Values: []any{"x"}},
		{Param: "Limit"},
		{Star: true},
	}
	condition := condYAML{
		Left: &leaf, Right: &exprYAML{Value: &value}, IsNull: &leaf, IsNotNull: &leaf,
		Exists: &existsYAML{Query: &nested}, NotExists: &existsYAML{Query: &nested},
		And: []condYAML{{Left: &leaf}}, Or: []condYAML{{Right: &leaf}},
	}
	query := document{
		From: fromYAML{
			Query: &nested,
			Joins: []joinYAML{{From: &fromYAML{Name: "Join"}, On: []condYAML{condition}}},
		},
		Where: &condition, Having: &condition,
		GroupBy: expressions, OrderBy: []orderYAML{{exprYAML: expressions[0]}},
		Columns: []columnYAML{{exprYAML: expressions[0]}, {exprYAML: expressions[1]}, {Wildcard: &wildcardYAML{Source: "t"}}},
	}
	tree := tugqlTree{
		Query:       query,
		Parameters:  []tugqlParameter{{Name: "Limit", Type: "integer", Default: &value}},
		Definitions: []tugqlDefinition{{Kind: "import", Name: "Saved", Path: "./saved.tql", Using: []tugqlMapping{{Name: "Limit", Expression: expressions[0]}}}},
	}
	if diagnostics := preflightTugQLTree(tree); len(diagnostics) != 0 {
		t.Fatalf("a broad but shallow canonical shape should pass preflight: %+v", diagnostics)
	}
}
