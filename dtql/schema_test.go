package dtql

import (
	"encoding/json"
	"os"
	"testing"
)

// TestSchemaIsFresh re-derives the schema from the Go types and asserts the
// committed schema/schema.json and schema/schema.yaml are byte-identical. This
// is the regenerate-and-diff staleness check: CI fails if the committed schema
// drifts from the types. Regenerate with: go run ./dtql/cmd/gen-schema
func TestSchemaIsFresh(t *testing.T) {
	gotJSON, err := SchemaJSON()
	if err != nil {
		t.Fatalf("SchemaJSON: %v", err)
	}
	gotYAML, err := SchemaYAML()
	if err != nil {
		t.Fatalf("SchemaYAML: %v", err)
	}
	assertFileEquals(t, "schema/schema.json", gotJSON)
	assertFileEquals(t, "schema/schema.yaml", gotYAML)
}

func assertFileEquals(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (run: go run ./dtql/cmd/gen-schema)", path, err)
	}
	if string(got) != string(want) {
		t.Fatalf("%s is stale; regenerate with: go run ./dtql/cmd/gen-schema", path)
	}
}

// TestSchemaMetadata asserts the schema declares draft 2020-12 and the canonical $id.
func TestSchemaMetadata(t *testing.T) {
	b, err := SchemaJSON()
	if err != nil {
		t.Fatalf("SchemaJSON: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	if m["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Errorf("$schema = %v, want draft 2020-12", m["$schema"])
	}
	if m["$id"] != SchemaID {
		t.Errorf("$id = %v, want %s", m["$id"], SchemaID)
	}
}
