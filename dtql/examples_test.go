package dtql

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExamplesValid asserts every example .dtql.yaml document deserializes to a
// dal.StructuredQuery and validates against the generated schema. The set
// together exercises source, columns, comparison + And/Or group filters,
// ordering and limit/offset.
func TestExamplesValid(t *testing.T) {
	files, err := filepath.Glob("examples/*.dtql.yaml")
	if err != nil {
		t.Fatalf("glob examples: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no example documents found under examples/")
	}
	sch := compileSchema(t)

	corpus := ""
	for _, f := range files {
		f := f
		t.Run(filepath.Base(f), func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read %s: %v", f, err)
			}
			corpus += string(data)

			q, err := Deserialize(data)
			if err != nil {
				t.Fatalf("%s does not deserialize: %v", f, err)
			}
			if q == nil {
				t.Fatalf("%s deserialized to a nil query", f)
			}
			if err := validateDTQL(sch, data); err != nil {
				t.Fatalf("%s fails schema validation: %v", f, err)
			}
		})
	}

	// Together the examples must exercise the whole in-scope subset.
	for _, feature := range []string{
		"columns:", // columns
		"where:",   // filters
		"and:",     // And group
		"or:",      // Or group
		"values:",  // In with array
		"orderBy:", // ordering
		"limit:",   // limit
		"offset:",  // offset
	} {
		if !strings.Contains(corpus, feature) {
			t.Errorf("example corpus does not exercise %q", feature)
		}
	}
}
