package dtql

import "testing"

func TestTugQLISODateExpression(t *testing.T) {
	tests := []struct {
		name  string
		value any
		valid bool
	}{
		{name: "valid ISO date", value: "2026-10-10", valid: true},
		{name: "invalid calendar date", value: "2026-02-30"},
		{name: "non-canonical date", value: "2026-1-01"},
		{name: "non-string value", value: 20261010},
		{name: "null value"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var expression exprYAML
			if test.value != nil {
				expression.Value = &test.value
			}
			if got := tugqlISODateExpression(expression); got != test.valid {
				t.Fatalf("tugqlISODateExpression(%#v) = %v, want %v", test.value, got, test.valid)
			}
		})
	}
}

func TestTugQLSafeOperationsPropagatesUnresolvableScope(t *testing.T) {
	query := document{From: fromYAML{Name: "T"}}
	diagnostics := validateTugQLSafeOperations(query, TugQLResolveContext{})
	if len(diagnostics) == 0 {
		t.Fatal("safe-operation validation accepted a source without an authorized schema")
	}
}
