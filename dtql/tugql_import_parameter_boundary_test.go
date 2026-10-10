package dtql

import (
	"strings"
	"testing"
)

func TestTugQLImportParameterBoundaryRejectsUnsafeInt64Binding(t *testing.T) {
	document, diagnostics := ParseTugQL("parameters (\n  @Limit integer required\n)\nfrom Invoice as i\nselect @Limit as BoundValue")
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics=%+v", diagnostics)
	}
	for _, value := range []any{int(1 << 53), int64(1 << 53)} {
		context := TugQLResolveContext{
			AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "schema-r1", Tables: []TugQLTable{{Name: "Invoice"}}}},
			Bindings:          []TugQLBinding{{Name: "Limit", Set: true, Value: value}},
		}
		resolved, diagnostics := ResolveTugQL(document, context)
		if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "invalid_binding" {
			t.Fatalf("unsafe integer binding %T was not rejected: resolved=%+v diagnostics=%+v", value, resolved, diagnostics)
		}
	}
}

func TestTugQLImportParameterBoundaryAppliesMappingInsideImportedCTE(t *testing.T) {
	source := "parameters (\n  @Floor integer required\n)\n" +
		"with Saved from './saved.tql'\n  using (\n    @Minimum = @Floor\n  )\n" +
		"from Saved as s\nselect s.InvoiceId"
	document, diagnostics := ParseTugQL(source)
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics=%+v", diagnostics)
	}
	context := TugQLResolveContext{
		ProjectRoot: "/project", ImportingPath: "queries/main.tql", ProjectRevision: "r1",
		PinnedImports: []TugQLPinnedImport{{Path: "queries/saved.tql", Revision: "r1", Source: "parameters (\n  @Minimum integer required\n)\n" +
			"with Filtered as (\n  from Invoice as i\n  where i.InvoiceId >= @Minimum\n  select i.InvoiceId\n)\n" +
			"from Filtered as f\nselect f.InvoiceId",
		}},
		AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "schema-r1", Tables: []TugQLTable{{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}}}}}},
		Bindings:          []TugQLBinding{{Name: "Floor", Set: true, Value: int64(7)}},
	}
	resolved, diagnostics := ResolveTugQL(document, context)
	if len(diagnostics) != 0 || resolved.Query == nil || len(resolved.Dependencies) != 1 {
		t.Fatalf("nested imported CTE did not resolve: resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
	serialized, err := Serialize(resolved.Query)
	if err != nil || strings.Contains(string(serialized), "param:") || !strings.Contains(string(serialized), "value: 7") {
		t.Fatalf("nested imported parameter was not substituted: %s, err=%v", serialized, err)
	}
}

func TestTugQLImportParameterBoundaryRejectsIncompatibleMapping(t *testing.T) {
	source := "parameters (\n  @Label string required\n)\n" +
		"with Saved from './saved.tql'\n  using (\n    @Minimum = @Label\n  )\n" +
		"from Saved as s\nselect s.InvoiceId"
	document, diagnostics := ParseTugQL(source)
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics=%+v", diagnostics)
	}
	context := TugQLResolveContext{
		ProjectRoot: "/project", ImportingPath: "queries/main.tql", ProjectRevision: "r1",
		PinnedImports:     []TugQLPinnedImport{{Path: "queries/saved.tql", Revision: "r1", Source: "parameters (\n  @Minimum integer required\n)\nfrom Invoice as i\nwhere i.InvoiceId >= @Minimum\nselect i.InvoiceId"}},
		AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "schema-r1", Tables: []TugQLTable{{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}}}}}},
		Bindings:          []TugQLBinding{{Name: "Label", Set: true, Value: "seven"}},
	}
	resolved, diagnostics := ResolveTugQL(document, context)
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "invalid_import_mapping" {
		t.Fatalf("incompatible mapping did not fail closed: resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
}
