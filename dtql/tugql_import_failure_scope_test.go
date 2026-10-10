package dtql

import (
	"fmt"
	"strings"
	"testing"
)

func TestTugQLPublicImportFailureScopesPreserveDraft(t *testing.T) {
	for _, tc := range []struct {
		name, source, pinned, code string
	}{
		{
			name:   "import in a CTE definition",
			source: "with Derived as (\n  with Missing from './missing.tql'\n  from Missing\n)\nfrom Derived\n",
			code:   "unpinned_import",
		},
		{
			name:   "scalar-local import inside CTE query",
			source: "with Derived as (\n  from T\n  select (\n    Child as (\n      with Missing from './missing.tql'\n      from Missing\n    )\n  )\n)\nfrom Derived\n",
			code:   "unpinned_import",
		},
		{
			name:   "mapping references undeclared caller parameter",
			source: "with Saved from './saved.tql'\n  using (\n    @Minimum = @Unknown\n  )\nfrom Saved\n",
			pinned: "parameters (\n  @Minimum integer required\n)\nfrom T\nwhere Id >= @Minimum\nselect Id\n",
			code:   "undeclared_parameter",
		},
		{
			name:   "imported predicate references undeclared parameter",
			source: "with Saved from './saved.tql'\nfrom Saved\n",
			pinned: "from T\nwhere Id >= @Unknown\nselect Id\n",
			code:   "undeclared_parameter",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, diagnostics := ParseTugQL(tc.source)
			if len(diagnostics) != 0 || doc.Tree == nil {
				t.Fatalf("authoring draft rejected: %+v", diagnostics)
			}
			context := TugQLResolveContext{
				ProjectRoot: "/project", ImportingPath: "queries/main.tql", ProjectRevision: "r1",
				AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "schema-r1", Tables: []TugQLTable{{Name: "T", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}}}},
			}
			if tc.pinned != "" {
				context.PinnedImports = []TugQLPinnedImport{{Path: "queries/saved.tql", Revision: "r1", Source: tc.pinned}}
			}
			resolved, diagnostics := ResolveTugQL(doc, context)
			if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != tc.code || doc.Source != tc.source {
				t.Fatalf("import failure produced wrong result: query=%v diagnostics=%+v source preserved=%v", resolved.Query != nil, diagnostics, doc.Source == tc.source)
			}
		})
	}
}

func TestTugQLRootAndPinnedImportShareOneSemanticBudget(t *testing.T) {
	var declarations strings.Builder
	declarations.WriteString("parameters (\n")
	for i := 0; i < 1300; i++ {
		fmt.Fprintf(&declarations, "  @P%d integer default 0\n", i)
	}
	declarations.WriteString(")\n")
	source := declarations.String() + "with Saved from './saved.tql'\nfrom Saved\nselect Id\n"
	pinned := declarations.String() + "from T\nselect Id\n"
	doc, diagnostics := ParseTugQL(source)
	if len(diagnostics) != 0 || doc.Tree == nil {
		t.Fatalf("root alone should fit the semantic budget: %+v", diagnostics)
	}
	if imported, diagnostics := ParseTugQL(pinned); len(diagnostics) != 0 || imported.Tree == nil {
		t.Fatalf("import alone should fit the semantic budget: %+v", diagnostics)
	}
	context := TugQLResolveContext{
		ProjectRoot: "/project", ImportingPath: "queries/main.tql", ProjectRevision: "r1",
		PinnedImports:     []TugQLPinnedImport{{Path: "queries/saved.tql", Revision: "r1", Source: pinned}},
		AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "schema-r1", Tables: []TugQLTable{{Name: "T", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}}}},
	}
	resolved, diagnostics := ResolveTugQL(doc, context)
	if resolved.Query != nil || len(diagnostics) != 1 || diagnostics[0].Code != "document_node_limit" {
		t.Fatalf("root and import escaped their shared budget: query=%v diagnostics=%+v", resolved.Query != nil, diagnostics)
	}
}
