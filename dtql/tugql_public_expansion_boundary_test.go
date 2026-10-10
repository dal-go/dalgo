package dtql

import (
	"fmt"
	"strings"
	"testing"
)

func TestTugQLPublicImportDepthBoundary(t *testing.T) {
	const importCount = 129
	source := "with Chain from './node-000.tql'\nfrom Chain\nselect Id\n"
	doc, parseDiagnostics := ParseTugQL(source)
	if len(parseDiagnostics) != 0 || doc.Tree == nil {
		t.Fatalf("root import draft did not parse: %+v", parseDiagnostics)
	}
	pinned := make([]TugQLPinnedImport, 0, importCount)
	for i := 0; i < importCount; i++ {
		path := fmt.Sprintf("queries/node-%03d.tql", i)
		query := "from T\nselect Id\n"
		if i+1 < importCount {
			query = fmt.Sprintf("with Chain from './node-%03d.tql'\nfrom Chain\nselect Id\n", i+1)
		}
		pinned = append(pinned, TugQLPinnedImport{Path: path, Revision: "r1", Source: query})
	}
	context := TugQLResolveContext{
		ProjectRoot: "/project", ImportingPath: "queries/main.tql", ProjectRevision: "r1", PinnedImports: pinned,
		AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "s1", Tables: []TugQLTable{{Name: "T", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}}}},
	}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "import_depth_exceeded" {
		t.Fatalf("deep pinned chain result: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
	}
}

func TestTugQLPublicDependencyReceiptsDeduplicateRootAndNestedOccurrence(t *testing.T) {
	source := "with Parent from './parent.tql'\nwith Direct from './child.tql'\nfrom Parent\nselect Id\n"
	doc, parseDiagnostics := ParseTugQL(source)
	if len(parseDiagnostics) != 0 || doc.Tree == nil {
		t.Fatalf("root import draft did not parse: %+v", parseDiagnostics)
	}
	context := TugQLResolveContext{
		ProjectRoot: "/project", ImportingPath: "queries/main.tql", ProjectRevision: "r1",
		PinnedImports: []TugQLPinnedImport{
			{Path: "queries/parent.tql", Revision: "r1", Source: "with Child from './child.tql'\nfrom Child\nselect Id\n"},
			{Path: "queries/child.tql", Revision: "r1", Source: "from T\nselect Id\n"},
		},
		AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "s1", Tables: []TugQLTable{{Name: "T", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}}}},
	}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if len(diagnostics) != 0 || resolved.Query == nil {
		t.Fatalf("nested/direct imports did not resolve: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
	}
	counts := map[TugQLDependencyReceipt]int{}
	for _, receipt := range resolved.Dependencies {
		counts[receipt]++
	}
	want := []TugQLDependencyReceipt{{Path: "queries/parent.tql", Revision: "r1"}, {Path: "queries/child.tql", Revision: "r1"}}
	if len(resolved.Dependencies) != len(want) {
		t.Fatalf("dependency receipts=%+v, want unique path/revision receipts %+v", resolved.Dependencies, want)
	}
	for _, receipt := range want {
		if counts[receipt] != 1 {
			t.Fatalf("receipt %+v occurred %d times: %+v", receipt, counts[receipt], resolved.Dependencies)
		}
	}
}

func TestTugQLPublicWideAuthorizedDefaultAndCTEProjectionsStayBounded(t *testing.T) {
	fields := make([]TugQLField, 5100)
	for i := range fields {
		fields[i] = TugQLField{Name: fmt.Sprintf("F%05d", i), Type: "integer", Authorized: true}
	}
	context := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "s1", Tables: []TugQLTable{{Name: "T", Fields: fields}}}}}
	for _, tc := range []struct {
		name   string
		source string
	}{
		{"omitted root projection", "from T\n"},
		{"omitted CTE projection", "with Wide as (\n  from T\n)\nfrom Wide\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, parseDiagnostics := ParseTugQL(tc.source)
			if len(parseDiagnostics) != 0 || doc.Tree == nil {
				t.Fatalf("small authored query did not parse: %+v", parseDiagnostics)
			}
			resolved, diagnostics := ResolveTugQL(doc, context)
			if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "document_node_limit" {
				t.Fatalf("wide schema projection escaped budget: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
			}
		})
	}
	// This smaller projection stays below the conservative typed-AST preflight
	// but crosses the exact canonical-node budget while enqueueing its columns.
	canonicalLimitContext := TugQLResolveContext{AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "s1", Tables: []TugQLTable{{Name: "T", Fields: fields[:1300]}}}}}
	doc, parseDiagnostics := ParseTugQL("from T\n")
	if len(parseDiagnostics) != 0 || doc.Tree == nil {
		t.Fatalf("small authored query did not parse: %+v", parseDiagnostics)
	}
	resolved, diagnostics := ResolveTugQL(doc, canonicalLimitContext)
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "document_node_limit" {
		t.Fatalf("canonical-node limit was not enforced: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
	}
}

func TestTugQLPublicMissingImportPropagatesFromScalarInsidePinnedImport(t *testing.T) {
	doc, parseDiagnostics := ParseTugQL("with Saved from './outer.tql'\nfrom Saved\nselect Id\n")
	if len(parseDiagnostics) != 0 || doc.Tree == nil {
		t.Fatalf("outer import draft did not parse: %+v", parseDiagnostics)
	}
	outer := "from T as t\nselect (\n  Value as (\n    with Missing from './missing.tql'\n    from Missing\n    select Id\n  )\n)\n"
	context := TugQLResolveContext{
		ProjectRoot: "/project", ImportingPath: "queries/main.tql", ProjectRevision: "r1",
		PinnedImports: []TugQLPinnedImport{{Path: "queries/outer.tql", Revision: "r1", Source: outer}},
	}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "unpinned_import" || !strings.Contains(diagnostics[0].Message, "queries/missing.tql") {
		t.Fatalf("nested missing import result: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
	}
}

func TestTugQLPublicMissingImportPropagatesThroughJoinedDerivedQuery(t *testing.T) {
	type object = map[string]any
	field := func(source, name string) object { return object{"source": source, "field": name} }
	inner := object{
		"from": object{"name": "T", "alias": "i"},
		"columns": []any{object{
			"query": object{
				"query":       object{"from": object{"name": "T", "alias": "s"}, "columns": []any{field("s", "Id")}},
				"definitions": []any{object{"kind": "import", "name": "Missing", "path": "./missing.tql"}},
			},
			"as": "Value",
		}},
	}
	query := object{
		"from": object{"name": "T", "alias": "o", "joins": []any{object{
			"from": object{"query": inner, "alias": "d"},
			"on":   []any{object{"op": "==", "left": field("o", "Id"), "right": field("d", "Value")}},
		}}},
		"columns": []any{field("o", "Id")},
	}
	tree := &TugQLTree{Format: "tugqtree", Version: 1, Query: query}
	doc := tugqlTestDocument(tree)
	context := TugQLResolveContext{ProjectRoot: "/project", ImportingPath: "queries/main.tql", ProjectRevision: "r1"}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "unpinned_import" || !strings.Contains(diagnostics[0].Message, "queries/missing.tql") {
		t.Fatalf("joined-derived missing import result: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
	}
}
