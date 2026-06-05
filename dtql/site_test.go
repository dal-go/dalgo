package dtql

import (
	"os"
	"strings"
	"testing"
)

// TestSiteIndexLinksArtifacts asserts the published index page links the
// generated artifacts (schema in both serializations and the examples) and
// documents the node→YAML mapping, so the served page stays in sync with what
// the publish workflow ships.
func TestSiteIndexLinksArtifacts(t *testing.T) {
	html, err := os.ReadFile("site/index.html")
	if err != nil {
		t.Fatalf("read site/index.html: %v", err)
	}
	page := string(html)
	for _, want := range []string{
		"./schema.json", // links JSON schema
		"./schema.yaml", // links YAML schema
		"./examples/",   // links examples
		SchemaID,        // canonical $id documented
		"Node → YAML",   // mapping section
		"from:",         // example present
	} {
		if !strings.Contains(page, want) {
			t.Errorf("site/index.html does not reference %q", want)
		}
	}
}
