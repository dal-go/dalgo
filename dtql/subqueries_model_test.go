package dtql

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSubqueryFixturesDeserializeAndRoundTrip proves that the shared recursive
// wire contract is understood by the Go model before an executor is involved.
func TestSubqueryFixturesDeserializeAndRoundTrip(t *testing.T) {
	const directory = "testdata/subqueries"
	data, err := os.ReadFile(filepath.Join(directory, "suite.json"))
	if err != nil {
		t.Fatal(err)
	}
	var suite subqueryFixtureSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range suite.Cases {
		if fixture.Input == "" {
			continue
		}
		t.Run(fixture.Name, func(t *testing.T) {
			input, err := os.ReadFile(filepath.Join(directory, fixture.Input))
			if err != nil {
				t.Fatal(err)
			}
			query, err := Deserialize(input)
			if strings.HasPrefix(fixture.Name, "scope-") && fixture.Name != "scope-ambiguous" && fixture.Error != "" {
				if err == nil {
					t.Fatal("Deserialize succeeded for an expected scope error fixture")
				}
				return
			}
			if err != nil {
				t.Fatalf("Deserialize: %v", err)
			}
			encoded, err := Serialize(query)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}
			roundTripped, err := Deserialize(encoded)
			if err != nil {
				t.Fatalf("Deserialize serialized query: %v\n%s", err, encoded)
			}
			if !Equal(query, roundTripped) {
				t.Fatalf("recursive query changed after round trip\n%s", encoded)
			}
		})
	}
}
