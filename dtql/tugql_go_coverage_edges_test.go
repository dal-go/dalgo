package dtql

import "testing"

func TestTugQLCoverageEdgesAuthorizedTableSelection(t *testing.T) {
	document := TugQLDocument{
		SourceMetadata: TugQLSourceMetadata{Format: "tugql", Version: 1},
		Tree: &TugQLTree{Format: "tugqtree", Version: 1, Query: map[string]any{
			"from":    map[string]any{"database": "analytics", "schema": "sales", "name": "Invoice", "alias": "i"},
			"columns": []any{map[string]any{"field": "InvoiceId", "source": "i"}},
		}},
	}
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{
		{Database: "archive", Schema: "sales", Version: "archive-v2", Tables: []TugQLTable{{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "string", Authorized: true}}}}},
		{Database: "analytics", Schema: "sales", Version: "sales-v3", Tables: []TugQLTable{{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}}}}},
	}}
	resolved, diagnostics := ResolveTugQL(document, context)
	if len(diagnostics) != 0 || resolved.Query == nil {
		t.Fatalf("qualified source did not resolve: resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
	if resolved.SchemaVersion != "sales-v3" || len(resolved.Columns) != 1 || resolved.Columns[0].Type != "integer" || len(resolved.Columns[0].Lineage) != 1 || resolved.Columns[0].Lineage[0] != (TugQLOutputLineage{Source: "i", Field: "InvoiceId"}) {
		t.Fatalf("wrong authorized source reached output: %+v", resolved)
	}

	document.Tree.Query["from"] = map[string]any{"name": "Invoice", "alias": "i"}
	resolved, diagnostics = ResolveTugQL(document, context)
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "ambiguous_table" {
		t.Fatalf("unqualified duplicate table should fail closed: resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
}

func TestTugQLCoverageEdgesInheritedOutputLineage(t *testing.T) {
	document, diagnostics := ParseTugQL("with Invoices as (\n  from Invoice as i\n  select i.InvoiceId as ExternalId\n)\nfrom Invoices as v\nselect v.ExternalId as Id")
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics=%+v", diagnostics)
	}
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{
		Version: "schema-r4",
		Tables:  []TugQLTable{{Name: "Invoice", Fields: []TugQLField{{Name: "InvoiceId", Type: "integer", Authorized: true}}}},
	}}}
	resolved, diagnostics := ResolveTugQL(document, context)
	if len(diagnostics) != 0 || resolved.Query == nil || resolved.SchemaVersion != "schema-r4" || len(resolved.Columns) != 1 {
		t.Fatalf("derived output did not resolve: resolved=%+v diagnostics=%+v", resolved, diagnostics)
	}
	column := resolved.Columns[0]
	if column.Name != "Id" || column.Type != "integer" || len(column.Lineage) != 1 || column.Lineage[0] != (TugQLOutputLineage{Source: "i", Field: "InvoiceId"}) {
		t.Fatalf("derived output lost its type or source lineage: %+v", column)
	}
}
