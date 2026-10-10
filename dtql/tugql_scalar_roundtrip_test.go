package dtql

import (
	"encoding/json"
	"strings"
	"testing"
)

// A nested authoring draft keeps its expression structure and comments even
// when some expressions are outside the current execution profile.
func TestTugQLScalarDraftRoundTripIndentStyles(t *testing.T) {
	for _, indent := range []string{"  ", "\t"} {
		t.Run(strings.ReplaceAll(indent, "\t", "tab"), func(t *testing.T) {
			source := "from Invoice as i -- billing records\nselect (\n" +
				indent + "CustomerName as (\n" +
				indent + indent + "from Customer as c\n" +
				indent + indent + "where c.Id = i.CustomerId\n" +
				indent + indent + "group by c.Name\n" +
				indent + indent + "having COUNT(*) > 0\n" +
				indent + indent + "select COALESCE(c.Name, '名前') as Name -- fallback label\n" +
				indent + ")\n)\n"
			doc, diagnostics := ParseTugQL(source)
			if len(diagnostics) != 0 || doc.Tree == nil {
				t.Fatalf("nested draft rejected: %+v", diagnostics)
			}
			if doc.Source != source {
				t.Fatal("parser changed the original source")
			}
			encoded, err := json.Marshal(doc.Tree)
			if err != nil {
				t.Fatal(err)
			}
			for _, expected := range []string{"groupBy", "having", "COALESCE", "名前", "CustomerName"} {
				if !strings.Contains(string(encoded), expected) {
					t.Fatalf("nested draft lost %q: %s", expected, encoded)
				}
			}
			var tree TugQLTree
			if err := json.Unmarshal(encoded, &tree); err != nil {
				t.Fatal(err)
			}
			roundTrip, err := json.Marshal(&tree)
			if err != nil || string(encoded) != string(roundTrip) {
				t.Fatalf("canonical tree changed after JSON round trip: %v", err)
			}
		})
	}
}
