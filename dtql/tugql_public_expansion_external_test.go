package dtql_test

import (
	"testing"

	"github.com/dal-go/dalgo/dtql"
)

func TestPublicNestedScalarImportDiagnosticPropagates(t *testing.T) {
	source := "from T\nselect (\n  outer as (\n    from T\n    select (\n      inner as (\n        with Missing from './missing.tql'\n        from Missing\n        select Id\n      )\n    )\n  )\n)\n"
	doc, diagnostics := dtql.ParseTugQL(source)
	if len(diagnostics) != 0 || doc.Tree == nil {
		t.Fatalf("nested scalar source did not parse: %+v", diagnostics)
	}
	resolved, diagnostics := dtql.ResolveTugQL(doc, dtql.TugQLResolveContext{
		ProjectRoot: "/project", ImportingPath: "queries/main.tql", ProjectRevision: "r1",
		AuthorizedSchemas: []dtql.TugQLAuthorizedSchema{{Version: "schema-r1", Tables: []dtql.TugQLTable{{Name: "T", Fields: []dtql.TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}}}},
	})
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "unpinned_import" {
		t.Fatalf("nested scalar import failure did not propagate: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
	}
}

func TestPublicImportedNestedUsingRejectsUndeclaredOuterParameter(t *testing.T) {
	source := "with Saved from './saved.tql'\n  using (\n    @Minimum = 7\n  )\nfrom Saved\nselect Id\n"
	doc, diagnostics := dtql.ParseTugQL(source)
	if len(diagnostics) != 0 || doc.Tree == nil {
		t.Fatalf("root import source did not parse: %+v", diagnostics)
	}
	context := dtql.TugQLResolveContext{
		ProjectRoot: "/project", ImportingPath: "queries/main.tql", ProjectRevision: "r1",
		PinnedImports:     []dtql.TugQLPinnedImport{{Path: "queries/saved.tql", Revision: "r1", Source: "parameters (\n  @Minimum integer required\n)\nwith Child from './child.tql'\n  using (\n    @Floor = @Unknown\n  )\nfrom T\nselect Id\n"}},
		AuthorizedSchemas: []dtql.TugQLAuthorizedSchema{{Version: "schema-r1", Tables: []dtql.TugQLTable{{Name: "T", Fields: []dtql.TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}}}},
	}
	resolved, diagnostics := dtql.ResolveTugQL(doc, context)
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "undeclared_parameter" {
		t.Fatalf("nested imported USING reference did not fail closed: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
	}
}

func TestPublicNestedDerivedAuthorizationFailurePropagates(t *testing.T) {
	type object = map[string]any
	field := func(source, name string) object { return object{"source": source, "field": name} }
	equality := func(left, right any) object { return object{"op": "==", "left": left, "right": right} }
	tree := &dtql.TugQLTree{Format: "tugqtree", Version: 1, Query: object{
		"from": object{"query": object{
			"as": "d",
			"from": object{"name": "T", "alias": "i", "joins": []any{object{
				"from": object{"name": "Hidden", "alias": "h"},
				"on":   []any{equality(field("i", "Id"), field("h", "Id"))},
			}}},
			"columns": []any{field("i", "Id")},
		}, "alias": "d"},
		"columns": []any{field("d", "Id")},
	}}
	doc := dtql.TugQLDocument{SourceMetadata: dtql.TugQLSourceMetadata{Format: "tugql", Version: 1}, Tree: tree}
	resolved, diagnostics := dtql.ResolveTugQL(doc, dtql.TugQLResolveContext{
		AuthorizedSchemas: []dtql.TugQLAuthorizedSchema{{Version: "schema-r1", Tables: []dtql.TugQLTable{{Name: "T", Fields: []dtql.TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}}}},
	})
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "unauthorized_source" {
		t.Fatalf("nested derived source authorization failure did not propagate: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
	}
}
