package dtql

import (
	"strings"
	"testing"
)

func TestTugQLResolverMoreSubstitutesAcrossQueryClauses(t *testing.T) {
	source := "parameters (\n  @floor integer required\n)\n" +
		"from Invoice as i\n" +
		"join Customer as c\n  on c.Id = i.InvoiceId\n" +
		"where i.InvoiceId > @floor\n" +
		"order by i.InvoiceId desc\n" +
		"select i.InvoiceId + @floor as Raised"
	doc, parseDiagnostics := ParseTugQL(source)
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics=%+v", parseDiagnostics)
	}
	context := TugQLResolveContext{
		AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "r1", Tables: []TugQLTable{
			{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}}},
			{Name: "Customer", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}},
		}}},
		Bindings: []TugQLBinding{{Name: "floor", Set: true, Value: int64(5)}},
	}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if len(diagnostics) != 0 || resolved.Query == nil || len(resolved.Columns) != 1 {
		t.Fatalf("resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
	serialized, err := Serialize(resolved.Query)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), "param:") || strings.Count(string(serialized), "value: 5") < 2 {
		t.Fatalf("binding did not lower across SELECT and WHERE: %s", serialized)
	}
}

func TestTugQLResolverMoreSubstitutesJoinConditionParameters(t *testing.T) {
	parameter := exprYAML{Param: "floor"}
	field := exprYAML{Field: "Id", Source: "c"}
	query := document{From: fromYAML{Joins: []joinYAML{{On: []condYAML{{Op: "==", Left: &field, Right: &parameter}}}}}}
	values := map[string]exprYAML{"floor": {Value: valuePtr(int64(5))}}
	if diagnostic := substituteTugQLQueryValues(&query, values); diagnostic != nil {
		t.Fatalf("join parameter substitution diagnostic=%+v", diagnostic)
	}
	got := query.From.Joins[0].On[0].Right
	if got == nil || got.Param != "" || got.Value == nil || *got.Value != int64(5) {
		t.Fatalf("join ON parameter did not lower to its typed value: %+v", got)
	}
}

func TestTugQLResolverMoreSubstitutesAllQueryExpressionAndConditionSlots(t *testing.T) {
	newParameter := func() *exprYAML { return &exprYAML{Param: "floor"} }
	field := exprYAML{Field: "Id", Source: "i"}
	columnRight := exprYAML{Param: "floor"}
	column := exprYAML{Binary: &binaryYAML{Op: "+", Left: &field, Right: &columnRight}}
	where := condYAML{Op: ">", Left: newParameter(), Right: &field}
	where.And = []condYAML{{Op: "==", Left: &field, Right: newParameter()}}
	having := condYAML{Op: ">", Left: &field, Right: newParameter()}
	joinCondition := condYAML{Op: "==", Left: &field, Right: newParameter()}
	order := exprYAML{Binary: &binaryYAML{Op: "+", Left: &field, Right: newParameter()}}
	query := document{
		From:    fromYAML{Joins: []joinYAML{{On: []condYAML{joinCondition}}}},
		Columns: []columnYAML{{exprYAML: column}},
		GroupBy: []exprYAML{*newParameter()},
		OrderBy: []orderYAML{{exprYAML: order}},
		Where:   &where,
		Having:  &having,
	}
	values := map[string]exprYAML{"floor": {Value: valuePtr(int64(5))}}
	if diagnostic := substituteTugQLQueryValues(&query, values); diagnostic != nil {
		t.Fatalf("query substitution diagnostic=%+v", diagnostic)
	}
	for name, expression := range map[string]*exprYAML{
		"projection binary right": query.Columns[0].Binary.Right,
		"group by":                &query.GroupBy[0],
		"order by binary right":   query.OrderBy[0].Binary.Right,
		"where left":              query.Where.Left,
		"where conjunction":       query.Where.And[0].Right,
		"having right":            query.Having.Right,
		"join condition":          query.From.Joins[0].On[0].Right,
	} {
		if expression == nil || expression.Param != "" || expression.Value == nil || *expression.Value != int64(5) {
			t.Errorf("%s parameter did not become the exact bound value: %+v", name, expression)
		}
	}
}

func TestTugQLResolverMoreSubstitutesNestedScalarAndCTEParameters(t *testing.T) {
	source := "parameters (\n  @floor integer required\n)\n" +
		"with RootRows as (\n  from Invoice as r\n  where r.InvoiceId > @floor\n  select r.InvoiceId + @floor as Raised\n)\n" +
		"from RootRows as i\n" +
		"select (\n  Result as (\n" +
		"    with LocalRows as (\n      from Invoice as n\n      where n.InvoiceId > @floor\n      select n.InvoiceId + @floor as Raised\n    )\n" +
		"    from LocalRows as l\n    select l.Raised\n  )\n)"
	doc, parseDiagnostics := ParseTugQL(source)
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics=%+v", parseDiagnostics)
	}
	context := TugQLResolveContext{
		AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "r1", Tables: []TugQLTable{{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}}}}}},
		Bindings:          []TugQLBinding{{Name: "floor", Set: true, Value: 3}},
	}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if len(diagnostics) != 0 || resolved.Query == nil || len(resolved.Columns) != 1 || resolved.Columns[0].Name != "Result" || resolved.Columns[0].Type != "number" {
		t.Fatalf("resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
	serialized, err := Serialize(resolved.Query)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), "param:") || strings.Count(string(serialized), "value: 3") < 4 {
		t.Fatalf("binding was not substituted through CTE and scalar query bodies: %s", serialized)
	}
}

func TestTugQLResolverMoreResolvesImportInsideScalarQuery(t *testing.T) {
	source := "parameters (\n  @floor integer required\n)\n" +
		"from Invoice as i\n" +
		"select (\n  ImportedTotal as (\n" +
		"    with Saved from './saved.tql'\n      using (\n        @Minimum = @floor\n      )\n" +
		"    from Saved as s\n    select s.InvoiceId\n  )\n)\n"
	doc, parseDiagnostics := ParseTugQL(source)
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics=%+v", parseDiagnostics)
	}
	context := TugQLResolveContext{
		ProjectRoot:     "/project",
		ImportingPath:   "queries/main.tql",
		ProjectRevision: "r1",
		PinnedImports: []TugQLPinnedImport{{
			Path: "queries/saved.tql", Revision: "r1",
			Source: "parameters (\n  @Minimum integer required\n)\nfrom Invoice as i\nwhere i.InvoiceId > @Minimum\nselect i.InvoiceId\n",
		}},
		AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "r1", Tables: []TugQLTable{{
			Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}},
		}}}},
		Bindings: []TugQLBinding{{Name: "floor", Set: true, Value: int64(2)}},
	}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if len(diagnostics) != 0 || resolved.Query == nil {
		t.Fatalf("resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
	if len(resolved.Dependencies) != 1 || resolved.Dependencies[0].Path != "queries/saved.tql" || resolved.Dependencies[0].Revision != "r1" {
		t.Fatalf("nested import receipt = %+v", resolved.Dependencies)
	}
	serialized, err := Serialize(resolved.Query)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), "param:") || !strings.Contains(string(serialized), "value: 2") {
		t.Fatalf("nested imported parameter was not substituted: %s", serialized)
	}
}

func TestTugQLResolverMoreRejectsUnauthorizedReferencesInEveryClause(t *testing.T) {
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "r1", Tables: []TugQLTable{
		{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}, {Name: "Secret", Type: "string", Authorized: false}}},
		{Name: "Customer", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}},
	}}}}
	for _, tc := range []struct {
		name, source string
	}{
		{"projection", "from Invoice as i\nselect i.Secret"},
		{"group by", "from Invoice as i\ngroup by i.Secret\nselect i.InvoiceId"},
		{"having", "from Invoice as i\nhaving i.Secret = 'x'\nselect i.InvoiceId"},
		{"order by", "from Invoice as i\norder by i.Secret\nselect i.InvoiceId"},
		{"join condition", "from Invoice as i\njoin Customer as c\n  on i.Secret = c.Id\nselect i.InvoiceId"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, parseDiagnostics := ParseTugQL(tc.source)
			if len(parseDiagnostics) != 0 {
				t.Fatalf("parse diagnostics=%+v", parseDiagnostics)
			}
			resolved, diagnostics := ResolveTugQL(doc, context)
			if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "unauthorized_field" {
				t.Fatalf("resolved=%+v diagnostics=%+v", resolved, diagnostics)
			}
		})
	}
}

func TestTugQLResolverMoreDefaultGroupProjectionAndOutputAliases(t *testing.T) {
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "r1", Tables: []TugQLTable{{Name: "Invoice", Fields: []TugQLField{
		{Name: "InvoiceId", Type: "integer", Authorized: true},
		{Name: "Region", Type: "string", Authorized: true},
	}}}}}}
	doc, parseDiagnostics := ParseTugQL("from Invoice as i\ngroup by i.Region")
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics=%+v", parseDiagnostics)
	}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if len(diagnostics) != 0 || resolved.Query == nil || len(resolved.Columns) != 1 || resolved.Columns[0].Name != "Region" || len(resolved.Columns[0].Lineage) != 1 || resolved.Columns[0].Lineage[0] != (TugQLOutputLineage{Source: "i", Field: "Region"}) {
		t.Fatalf("group projection=%+v diagnostics=%+v", resolved, diagnostics)
	}

	duplicate, parseDiagnostics := ParseTugQL("from Invoice as i\nselect i.InvoiceId as Value, i.Region as Value")
	if len(parseDiagnostics) != 0 {
		t.Fatalf("duplicate alias source parse diagnostics=%+v", parseDiagnostics)
	}
	if result, diagnostics := ResolveTugQL(duplicate, context); result.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "duplicate_output_name" {
		t.Fatalf("duplicate output names accepted: result=%+v diagnostics=%+v", result, diagnostics)
	}

	columns := []columnYAML{{As: "Region"}, {As: "i_Region"}, {As: "i_Region_2"}}
	if got := uniqueTugQLAlias("Region", "i", columns); got != "i_Region_3" {
		t.Fatalf("alias collision suffix=%q, want i_Region_3", got)
	}
}

func TestTugQLResolverMoreOutputTypePromotionAndLineage(t *testing.T) {
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "r1", Tables: []TugQLTable{{Name: "Invoice", Fields: []TugQLField{
		{Name: "Units", Type: "integer", Authorized: true},
		{Name: "Amount", Type: "decimal", Authorized: true},
	}}}}}}
	doc, parseDiagnostics := ParseTugQL("from Invoice as i\nselect i.Units + 1 as Mixed, i.Units / 1 as Ratio")
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics=%+v", parseDiagnostics)
	}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if len(diagnostics) != 0 || resolved.Query == nil || len(resolved.Columns) != 2 {
		t.Fatalf("resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
	for i, name := range []string{"Mixed", "Ratio"} {
		if resolved.Columns[i].Name != name || resolved.Columns[i].Type != "number" {
			t.Fatalf("column %d=%+v", i, resolved.Columns[i])
		}
	}
	if len(resolved.Columns[0].Lineage) != 1 || len(resolved.Columns[1].Lineage) != 1 {
		t.Fatalf("expression lineage=%+v", resolved.Columns)
	}
	average, parseDiagnostics := ParseTugQL("from Invoice as i\nselect AVG(i.Units) as Mean")
	if len(parseDiagnostics) != 0 {
		t.Fatalf("AVG parse diagnostics=%+v", parseDiagnostics)
	}
	resolved, diagnostics = ResolveTugQL(average, context)
	if len(diagnostics) != 0 || resolved.Query == nil || len(resolved.Columns) != 1 || resolved.Columns[0].Name != "Mean" || resolved.Columns[0].Type != "number" || len(resolved.Columns[0].Lineage) != 1 || resolved.Columns[0].Lineage[0].Field != "Units" {
		t.Fatalf("AVG output inference=%+v diagnostics=%+v", resolved, diagnostics)
	}
	unsafeDecimal, parseDiagnostics := ParseTugQL("from Invoice as i\nselect i.Amount + 1 as Total")
	if len(parseDiagnostics) != 0 {
		t.Fatalf("decimal expression parse diagnostics=%+v", parseDiagnostics)
	}
	resolved, diagnostics = ResolveTugQL(unsafeDecimal, context)
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "unsupported_decimal_semantics" {
		t.Fatalf("decimal arithmetic resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
	constants, parseDiagnostics := ParseTugQL("from Invoice\nselect 1 as Number, true as Enabled, 'ready' as Status")
	if len(parseDiagnostics) != 0 {
		t.Fatalf("constant projection parse diagnostics=%+v", parseDiagnostics)
	}
	resolved, diagnostics = ResolveTugQL(constants, context)
	if len(diagnostics) != 0 || resolved.Query == nil || len(resolved.Columns) != 3 {
		t.Fatalf("constant projection=%+v diagnostics=%+v", resolved, diagnostics)
	}
	for i, want := range []struct{ name, typeName string }{{"Number", "integer"}, {"Enabled", "boolean"}, {"Status", "string"}} {
		if resolved.Columns[i].Name != want.name || resolved.Columns[i].Type != want.typeName || len(resolved.Columns[i].Lineage) != 0 {
			t.Fatalf("constant column %d=%+v, want %s/%s without lineage", i, resolved.Columns[i], want.name, want.typeName)
		}
	}
}

func TestTugQLResolverMoreRejectsUninferableCanonicalScalar(t *testing.T) {
	// A float can arrive through canonical caller-provided TugQTree even though
	// source syntax deliberately accepts only exact integers. Resolution must
	// fail closed when the portable output contract has no safe inferred type.
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "r1", Tables: []TugQLTable{{Name: "Invoice"}}}}}
	for _, scalar := range []struct {
		name  string
		value any
	}{{"float", 1.25}, {"null", nil}} {
		t.Run(scalar.name, func(t *testing.T) {
			document := TugQLDocument{SourceMetadata: TugQLSourceMetadata{Format: "tugql", Version: 1}, Tree: &TugQLTree{Format: "tugqtree", Version: 1, Query: map[string]any{
				"from":    map[string]any{"name": "Invoice"},
				"columns": []any{map[string]any{"value": scalar.value, "as": "Value"}},
			}}}
			resolved, diagnostics := ResolveTugQL(document, context)
			if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "unsupported_output_type" {
				t.Fatalf("untyped scalar output was not rejected safely: resolved=%+v diagnostics=%+v", resolved, diagnostics)
			}
		})
	}
}

func TestTugQLResolverMoreCorrelationScopeBoundaries(t *testing.T) {
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "r1", Tables: []TugQLTable{
		{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}}},
		{Name: "Customer", Fields: []TugQLField{{Name: "Name", Type: "string", Authorized: true}}},
	}}}}
	outerSources := []tugqlResolvedSource{{alias: "i", table: "Invoice", fields: context.AuthorizedSchemas[0].Tables[0].Fields}}
	outerReference := exprYAML{Field: "InvoiceId", Source: "i"}
	innerReference := exprYAML{Field: "Name", Source: "c"}
	nested := document{
		From:    fromYAML{Name: "Customer", Alias: "c"},
		Where:   &condYAML{Op: "=", Left: &outerReference, Right: &innerReference},
		Columns: []columnYAML{{exprYAML: innerReference}},
	}
	correlatedScalar := document{From: fromYAML{Name: "Invoice", Alias: "i"}, Columns: []columnYAML{{exprYAML: exprYAML{Query: &nested}}}}
	if diagnostics := validateTugQLAuthorizedReferences(correlatedScalar, outerSources, context); len(diagnostics) != 0 {
		t.Fatalf("correlated scalar reference was rejected: %+v", diagnostics)
	}

	derivedFrom := document{From: fromYAML{Query: &nested}}
	if diagnostics := validateTugQLAuthorizedReferences(derivedFrom, outerSources, context); len(diagnostics) != 1 || diagnostics[0].Code != "unauthorized_field" {
		t.Fatalf("derived FROM unexpectedly captured outer scope: %+v", diagnostics)
	}
}

func TestTugQLResolverMoreRejectsUnsupportedCallsInEveryExecutableSlot(t *testing.T) {
	call := func() exprYAML {
		return exprYAML{tugqlCall: &tugqlCallYAML{Function: "COALESCE", Args: []exprYAML{{Value: valuePtr(any("x"))}}}}
	}
	field := exprYAML{Field: "Id", Source: "t"}
	queryWithCall := func() *document {
		return &document{From: fromYAML{Name: "T", Alias: "t"}, Columns: []columnYAML{{exprYAML: call()}}}
	}
	condition := func(expression exprYAML) *condYAML {
		return &condYAML{Op: "==", Left: &expression, Right: &field}
	}
	cases := []struct {
		name  string
		query document
	}{
		{"projection", document{From: fromYAML{Name: "T", Alias: "t"}, Columns: []columnYAML{{exprYAML: call()}}}},
		{"binary-left", document{From: fromYAML{Name: "T"}, Columns: []columnYAML{{exprYAML: exprYAML{Binary: &binaryYAML{Op: "+", Left: ptrExpr(call()), Right: ptrExpr(field)}}}}}},
		{"binary-right", document{From: fromYAML{Name: "T"}, Columns: []columnYAML{{exprYAML: exprYAML{Binary: &binaryYAML{Op: "+", Left: ptrExpr(field), Right: ptrExpr(call())}}}}}},
		{"aggregate-argument", document{From: fromYAML{Name: "T"}, Columns: []columnYAML{{exprYAML: exprYAML{Aggregate: &aggregateYAML{Function: "SUM", Args: []exprYAML{call()}}}}}}},
		{"group-by", document{From: fromYAML{Name: "T"}, GroupBy: []exprYAML{call()}}},
		{"order-by", document{From: fromYAML{Name: "T"}, OrderBy: []orderYAML{{exprYAML: call()}}}},
		{"where", document{From: fromYAML{Name: "T"}, Where: condition(call())}},
		{"having", document{From: fromYAML{Name: "T"}, Having: condition(call())}},
		{"join-on", document{From: fromYAML{Name: "T", Joins: []joinYAML{{From: &fromYAML{Name: "U"}, On: []condYAML{*condition(call())}}}}}},
		{"derived-source", document{From: fromYAML{Query: queryWithCall()}}},
		{"scalar-body", document{From: fromYAML{Name: "T"}, Columns: []columnYAML{{exprYAML: exprYAML{tugqlQueryBody: &tugqlBody{Query: *queryWithCall()}}}}}},
		{"nested-cte-body", document{From: fromYAML{Name: "T"}, Columns: []columnYAML{{exprYAML: exprYAML{tugqlQueryBody: &tugqlBody{Query: document{From: fromYAML{Name: "T"}}, Definitions: []tugqlDefinition{{Kind: "cte", Name: "C", Query: &tugqlBody{Query: *queryWithCall()}}}}}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diagnostics := validateTugQLExecutableExpressions(tc.query)
			if len(diagnostics) != 1 || diagnostics[0].Code != "unsupported_function" || !strings.Contains(diagnostics[0].Message, "COALESCE") {
				t.Fatalf("unsupported call escaped executable validation: %+v", diagnostics)
			}
		})
	}
}

func ptrExpr(expression exprYAML) *exprYAML { return &expression }
