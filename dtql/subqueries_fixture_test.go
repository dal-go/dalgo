package dtql

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type subqueryFixtureManifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	Files         map[string]string `json:"files"`
}

type subqueryFixtureSuite struct {
	SchemaVersion int    `json:"schemaVersion"`
	Schema        string `json:"schema"`
	Dataset       string `json:"dataset"`
	Cases         []struct {
		Name        string `json:"name"`
		Input       string `json:"input"`
		Rows        string `json:"rows"`
		Error       string `json:"error"`
		Expectation string `json:"expectation"`
	} `json:"cases"`
}

func TestSubqueryFixtureManifestAndSuite(t *testing.T) {
	const directory = "testdata/subqueries"
	manifestData, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest subqueryFixtureManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 1 {
		t.Fatalf("fixture manifest schemaVersion = %d, want 1", manifest.SchemaVersion)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(manifest.Files)+1 {
		t.Fatalf("fixture manifest has %d files; directory has %d entries", len(manifest.Files), len(entries)-1)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "manifest.json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if got, want := fmt.Sprintf("%x", sha256.Sum256(data)), manifest.Files[entry.Name()]; got != want {
			t.Fatalf("fixture %s digest = %s, want %s", entry.Name(), got, want)
		}
	}

	suiteData, err := os.ReadFile(filepath.Join(directory, "suite.json"))
	if err != nil {
		t.Fatal(err)
	}
	var suite subqueryFixtureSuite
	if err := json.Unmarshal(suiteData, &suite); err != nil {
		t.Fatal(err)
	}
	if suite.SchemaVersion != 1 || suite.Schema == "" || suite.Dataset == "" || len(suite.Cases) == 0 {
		t.Fatalf("invalid fixture suite: %#v", suite)
	}
	for _, name := range append([]string{suite.Schema, suite.Dataset}, suiteFixtureFiles(suite.Cases)...) {
		if _, ok := manifest.Files[name]; !ok {
			t.Fatalf("suite references unmanaged fixture %q", name)
		}
	}
}

func suiteFixtureFiles(cases []struct {
	Name        string `json:"name"`
	Input       string `json:"input"`
	Rows        string `json:"rows"`
	Error       string `json:"error"`
	Expectation string `json:"expectation"`
}) []string {
	files := make([]string, 0, len(cases)*3)
	for _, fixture := range cases {
		if fixture.Name == "" || (fixture.Input == "" && fixture.Expectation == "") {
			return append(files, "")
		}
		if fixture.Input != "" {
			files = append(files, fixture.Input)
		}
		for _, companion := range []string{fixture.Rows, fixture.Error, fixture.Expectation} {
			if companion != "" {
				files = append(files, companion)
			}
		}
	}
	return files
}
