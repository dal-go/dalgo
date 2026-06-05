package dtql

import (
	"encoding/json"
	"flag"
	"os"
	"testing"
)

// updateSchema regenerates the committed golden schema files instead of
// asserting them. Run: go test ./dtql/ -run TestSchemaIsFresh -update
var updateSchema = flag.Bool("update", false, "regenerate committed schema/schema.{json,yaml}")

// TestSchemaIsFresh re-derives the schema from the Go types and asserts the
// committed schema/schema.json and schema/schema.yaml are byte-identical — the
// regenerate-and-diff staleness check. With -update it rewrites the golden files
// (the generator), so there is no separate, untestable generator binary.
func TestSchemaIsFresh(t *testing.T) {
	jsonGolden, yamlGolden := "schema/schema.json", "schema/schema.yaml"
	gotJSON, gotYAML := SchemaJSON(), SchemaYAML()
	if *updateSchema {
		writeGolden(t, jsonGolden, gotJSON)
		writeGolden(t, yamlGolden, gotYAML)
		return
	}
	assertFileEquals(t, jsonGolden, gotJSON)
	assertFileEquals(t, yamlGolden, gotYAML)
}

func writeGolden(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	t.Logf("regenerated %s (%d bytes)", path, len(data))
}

func assertFileEquals(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (regenerate: go test ./dtql/ -run TestSchemaIsFresh -update)", path, err)
	}
	if string(got) != string(want) {
		t.Fatalf("%s is stale; regenerate: go test ./dtql/ -run TestSchemaIsFresh -update", path)
	}
}

// TestSchemaMetadata asserts the schema declares draft 2020-12 and the canonical $id.
func TestSchemaMetadata(t *testing.T) {
	var m map[string]any
	if err := json.Unmarshal(SchemaJSON(), &m); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	if m["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Errorf("$schema = %v, want draft 2020-12", m["$schema"])
	}
	if m["$id"] != SchemaID {
		t.Errorf("$id = %v, want %s", m["$id"], SchemaID)
	}
}
