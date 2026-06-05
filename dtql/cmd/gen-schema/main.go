// Command gen-schema writes the DTQL JSON Schema, generated from the dtql Go
// types, to dtql/schema/schema.json and dtql/schema/schema.yaml.
//
// Run from the module root: go run ./dtql/cmd/gen-schema
package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/dal-go/dalgo/dtql"
)

func main() {
	outDir := "dtql/schema"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", outDir, err)
	}

	jsonBytes, err := dtql.SchemaJSON()
	if err != nil {
		log.Fatalf("render JSON schema: %v", err)
	}
	yamlBytes, err := dtql.SchemaYAML()
	if err != nil {
		log.Fatalf("render YAML schema: %v", err)
	}

	writeFile(filepath.Join(outDir, "schema.json"), jsonBytes)
	writeFile(filepath.Join(outDir, "schema.yaml"), yamlBytes)
}

func writeFile(path string, data []byte) {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Fatalf("write %s: %v", path, err)
	}
	log.Printf("wrote %s (%d bytes)", path, len(data))
}
