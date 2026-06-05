package dtql_test

import (
	"os/exec"
	"strings"
	"testing"
)

// TestDalHasNoYAMLDependency asserts the import boundary required by
// AC:lives-in-dtql-package: the dtql package lives at
// github.com/dal-go/dalgo/dtql and the dal package gains no YAML dependency.
func TestDalHasNoYAMLDependency(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "github.com/dal-go/dalgo/dal").Output()
	if err != nil {
		t.Fatalf("go list -deps dal failed: %v", err)
	}
	for _, dep := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.Contains(dep, "yaml") {
			t.Errorf("dal package must not depend on a YAML library, but depends on %q", dep)
		}
	}
}

// TestDtqlImportsDal asserts dtql imports dal (the package compiles against
// dal types, verified at build time via the rest of the package).
func TestDtqlImportsDal(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", "{{ join .Imports \"\\n\" }}", "github.com/dal-go/dalgo/dtql").Output()
	if err != nil {
		t.Fatalf("go list dtql imports failed: %v", err)
	}
	if !strings.Contains(string(out), "github.com/dal-go/dalgo/dal") {
		t.Errorf("dtql package must import dal; imports were:\n%s", out)
	}
}
