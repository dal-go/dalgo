package dtql

import (
	"fmt"
	"strings"
	"testing"
)

// These inputs stay below the source/token limits but exceed the preliminary
// typed-AST work budget, so the parser must reject before canonical export.
func TestTugQLBroadSourcePreflightStopsBeforeExport(t *testing.T) {
	const count = 20001
	fields := make([]string, count)
	parameters := make([]string, count)
	definitions := make([]string, count)
	// Leave room for the scalar/query wrappers so local definitions exercise
	// both the body-enqueue cap and the first nested CTE body's enqueue cap.
	nestedDefinitions := make([]string, maxTugQLPreflightNodes-7)
	nestedDefinitionsAtPendingCap := make([]string, maxTugQLPreflightNodes-6)
	for i := range fields {
		fields[i] = fmt.Sprintf("Field%d", i)
		parameters[i] = fmt.Sprintf("  @P%d integer default 0", i)
		definitions[i] = fmt.Sprintf("with Q%d as (\n  from T\n)", i)
	}
	for i := range nestedDefinitions {
		nestedDefinitions[i] = fmt.Sprintf("    with Local%d as (\n      from T\n    )", i)
	}
	for i := range nestedDefinitionsAtPendingCap {
		nestedDefinitionsAtPendingCap[i] = fmt.Sprintf("    with Local%d as (\n      from T\n    )", i)
	}
	joins := make([]string, count)
	conditions := make([]string, count)
	mappings := make([]string, count)
	for i := range joins {
		joins[i] = "join T as j\n  on r.Id = j.Id"
		conditions[i] = "Id = 1"
		mappings[i] = fmt.Sprintf("@P%d = 1", i)
	}
	cases := map[string]string{
		"parameters":                           "parameters (\n" + strings.Join(parameters, "\n") + "\n)\nfrom T\n",
		"definitions":                          strings.Join(definitions, "\n") + "\nfrom T\n",
		"scalar local definitions":             "from T\nselect (\n  Scalar as (\n" + strings.Join(nestedDefinitions, "\n") + "\n    from T\n  )\n)\n",
		"scalar local definitions pending cap": "from T\nselect (\n  Scalar as (\n" + strings.Join(nestedDefinitionsAtPendingCap, "\n") + "\n    from T\n  )\n)\n",
		"columns":                              "from T\nselect " + strings.Join(fields, ", ") + "\n",
		"group keys":                           "from T\ngroup by " + strings.Join(fields, ", ") + "\n",
		"order keys":                           "from T\norder by " + strings.Join(fields, ", ") + "\n",
		"function arguments":                   "from T\nselect F(" + strings.Join(fields, ", ") + ")\n",
		"aggregate arguments":                  "from T\nselect SUM(" + strings.Join(fields, ", ") + ")\n",
		"join list":                            "from T as r\n" + strings.Join(joins, "\n") + "\nselect r.Id as Id\n",
		"condition list":                       "from T\nwhere " + strings.Join(conditions, " and ") + "\n",
		"import mappings":                      "with Saved from './saved.tql'\n  using (\n    " + strings.Join(mappings, "\n    ") + "\n  )\nfrom Saved\n",
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			doc, diagnostics := ParseTugQL(source)
			if doc.Tree != nil || doc.Source != source || len(diagnostics) != 1 || diagnostics[0].Code != "document_node_limit" {
				t.Fatalf("broad input escaped the bounded parser: tree=%v diagnostics=%+v source preserved=%v", doc.Tree != nil, diagnostics, doc.Source == source)
			}
		})
	}
}
