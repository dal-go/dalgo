package dtql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
)

// compileSchema compiles the generated DTQL JSON Schema for validation.
func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	c.Draft = jsonschema.Draft2020
	if err := c.AddResource(SchemaID, bytes.NewReader(SchemaJSON())); err != nil {
		t.Fatalf("AddResource: %v", err)
	}
	sch, err := c.Compile(SchemaID)
	if err != nil {
		t.Fatalf("Compile schema: %v", err)
	}
	return sch
}

// validateDTQL validates a DTQL-YAML document against the compiled schema. It
// normalizes the YAML to JSON-native types (string keys, float64 numbers) so
// the validator sees the same value space the schema describes.
func validateDTQL(sch *jsonschema.Schema, dtqlYAML []byte) error {
	var asYAML any
	if err := yaml.Unmarshal(dtqlYAML, &asYAML); err != nil {
		return fmt.Errorf("not YAML: %w", err)
	}
	jsonBytes, err := json.Marshal(asYAML)
	if err != nil {
		return fmt.Errorf("to JSON: %w", err)
	}
	var doc any
	if err := json.Unmarshal(jsonBytes, &doc); err != nil {
		return fmt.Errorf("from JSON: %w", err)
	}
	return sch.Validate(doc)
}

// TestSerializedValidatesAgainstSchema asserts every DTQL document produced by
// Serialize for an in-scope query validates against the generated schema, so the
// schema and the serializer cannot drift.
func TestSerializedValidatesAgainstSchema(t *testing.T) {
	sch := compileSchema(t)
	for name, q := range roundTripCases() {
		t.Run(name, func(t *testing.T) {
			data, err := Serialize(q)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}
			if err := validateDTQL(sch, data); err != nil {
				t.Fatalf("serialized DTQL failed schema validation: %v\nYAML:\n%s", err, data)
			}
		})
	}
}

// TestSchemaRejectsInvalid guards that the schema is not vacuously permissive.
func TestSchemaRejectsInvalid(t *testing.T) {
	sch := compileSchema(t)
	invalid := map[string]string{
		"missing from":     "columns:\n  - field: name\n",
		"unknown operator": "from:\n  name: users\nwhere:\n  op: \"!=\"\n  left:\n    field: a\n  right:\n    value: 1\n",
		"unknown top key":  "from:\n  name: users\nbogus: 1\n",
		"empty expression": "from:\n  name: users\ncolumns:\n  - as: x\n",
	}
	for name, doc := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := validateDTQL(sch, []byte(doc)); err == nil {
				t.Fatalf("expected schema to reject %q", name)
			}
		})
	}
}
