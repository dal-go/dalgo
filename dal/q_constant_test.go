package dal

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestConstant_Equal(t *testing.T) {
	tests := []struct {
		name     string
		constant Constant
		input    Constant
		want     bool
	}{
		{
			name:     "both_empty",
			constant: Constant{},
			input:    Constant{},
			want:     true,
		},
		{
			name:     "string_equal",
			constant: Constant{Value: "s1"},
			input:    Constant{Value: "s1"},
			want:     true,
		},
		{
			name:     "string_not_equal",
			constant: Constant{Value: "s1"},
			input:    Constant{Value: "s2"},
			want:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.constant.Equal(tt.input)
			assert.Equal(t, tt.want, actual)
		})
	}
}

func TestConstant_String(t *testing.T) {
	tests := []struct {
		name     string
		constant Constant
		want     string
	}{
		{
			name:     "empty",
			constant: Constant{},
			want:     "null",
		},
		{
			name:     "string",
			constant: Constant{Value: "s1"},
			want:     "'s1'",
		},
		{
			name:     "int",
			constant: Constant{Value: 123},
			want:     "123",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.constant.String()
			assert.Equal(t, tt.want, actual)
		})
	}
}
