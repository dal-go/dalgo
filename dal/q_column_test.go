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
