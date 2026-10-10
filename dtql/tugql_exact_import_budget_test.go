package dtql

import (
	"fmt"
	"strings"
	"testing"
)

func TestTugQLFullAuthoringBudgetLeavesNoRoomForPinnedImport(t *testing.T) {
	var s strings.Builder
	s.WriteString("parameters (\n")
	for i := 0; i < 2496; i++ {
		fmt.Fprintf(&s, "  @P%d integer default 0\n", i)
	}
	s.WriteString(")\nwith Saved from './saved.tql'\nfrom Saved\nlimit 1\nselect Id\n")
	d, ds := ParseTugQL(s.String())
	if len(ds) > 0 || d.Tree == nil {
		t.Fatalf("bounded root should parse: %+v", ds)
	}
	r, ds := ResolveTugQL(d, TugQLResolveContext{ProjectRoot: "/project", ImportingPath: "queries/main.tql", ProjectRevision: "r1", PinnedImports: []TugQLPinnedImport{{Path: "queries/saved.tql", Revision: "r1", Source: "from T\nselect Id\n"}}, AuthorizedSchemas: []TugQLAuthorizedSchema{{Version: "v1", Tables: []TugQLTable{{Name: "T", Fields: []TugQLField{{Name: "Id", Type: "integer", Authorized: true}}}}}}})
	if r.Query != nil || len(ds) != 1 || ds[0].Code != "document_node_limit" {
		t.Fatalf("root plus pinned import exceeded budget: executable=%t diagnostics=%+v", r.Query != nil, ds)
	}
}
