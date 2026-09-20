package dal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestColumn(t *testing.T) {
	type expected struct {
		string string
	}
	tests := []struct {
		name     string
		column   Column
		expected expected
	}{
		{
			name:   "empty",
			column: Column{},
			expected: expected{
				string: "NULL",
			},
		},
		{
			name: "expression_only",
			column: Column{Expression: Constant{
				Value: "foo",
			}},
			expected: expected{
				string: "'foo'",
			},
		},
		{
			name:   "alias_only",
			column: Column{Alias: "c1"},
			expected: expected{
				string: "NULL AS c1",
			},
		},
		{
			name: "expression_with_alias",
			column: Column{
				Alias:      "c1",
				Expression: Constant{Value: "foo"},
			},
			expected: expected{
				string: "'foo' AS c1",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("String", func(t *testing.T) {
				actual := tt.column.String()
				assert.Equal(t, tt.expected.string, actual)
			})
		})
	}
}

func TestWildcardProjection(t *testing.T) {
	tests := []struct {
		name string
		got  Column
		want string
	}{
		{name: "unqualified", got: AllColumnsExcept("email", "password_hash"), want: "*-(email, password_hash)"},
		{name: "qualified", got: AllColumnsExceptFrom("c", "email"), want: "c.*-(email)"},
		{name: "duplicates retained", got: AllColumnsExcept("email", "email"), want: "*-(email, email)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.got.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}

	excluded := []string{"email"}
	column := AllColumnsExcept(excluded...)
	excluded[0] = "changed"
	if got := column.Wildcard.Exclude[0]; got != "email" {
		t.Fatalf("constructor retained caller slice: got %q", got)
	}
}
