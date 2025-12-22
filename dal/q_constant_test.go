package dal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

func TestNewConstant(t *testing.T) {
	// Create a constant to test the Constant case
	testConstant := Constant{Value: "test"}

	// Create a time value for testing
	testTime := time.Now()

	tests := []struct {
		name      string
		input     any
		want      any
		wantPanic bool
	}{
		{
			name:      "nil",
			input:     nil,
			want:      nil,
			wantPanic: false,
		},
		{
			name:      "bool_true",
			input:     true,
			want:      true,
			wantPanic: false,
		},
		{
			name:      "bool_false",
			input:     false,
			want:      false,
			wantPanic: false,
		},
		{
			name:      "string",
			input:     "test string",
			want:      "test string",
			wantPanic: false,
		},
		{
			name:      "float32",
			input:     float32(3.14),
			want:      float32(3.14),
			wantPanic: false,
		},
		{
			name:      "float64",
			input:     3.14159,
			want:      3.14159,
			wantPanic: false,
		},
		{
			name:      "int",
			input:     42,
			want:      42,
			wantPanic: false,
		},
		{
			name:      "int8",
			input:     int8(8),
			want:      int8(8),
			wantPanic: false,
		},
		{
			name:      "int16",
			input:     int16(16),
			want:      int16(16),
			wantPanic: false,
		},
		{
			name:      "int32",
			input:     int32(32),
			want:      int32(32),
			wantPanic: false,
		},
		{
			name:      "int64",
			input:     int64(64),
			want:      int64(64),
			wantPanic: false,
		},
		{
			name:      "uint",
			input:     uint(42),
			want:      uint(42),
			wantPanic: false,
		},
		{
			name:      "uint8",
			input:     uint8(8),
			want:      uint8(8),
			wantPanic: false,
		},
		{
			name:      "uint16",
			input:     uint16(16),
			want:      uint16(16),
			wantPanic: false,
		},
		{
			name:      "uint32",
			input:     uint32(32),
			want:      uint32(32),
			wantPanic: false,
		},
		{
			name:      "uint64",
			input:     uint64(64),
			want:      uint64(64),
			wantPanic: false,
		},
		{
			name:      "time",
			input:     testTime,
			want:      testTime,
			wantPanic: false,
		},
		{
			name:      "constant",
			input:     testConstant,
			want:      testConstant.Value,
			wantPanic: false,
		},
		{
			name:      "unsupported_slice",
			input:     []string{"a", "b", "c"},
			want:      nil,
			wantPanic: true,
		},
		{
			name:      "unsupported_map",
			input:     map[string]string{"key": "value"},
			want:      nil,
			wantPanic: true,
		},
		{
			name:      "unsupported_struct",
			input:     struct{ Name string }{"test"},
			want:      nil,
			wantPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				assert.Panics(t, func() {
					NewConstant(tt.input)
				})
			} else {
				constant := NewConstant(tt.input)
				assert.Equal(t, tt.want, constant.Value)
			}
		})
	}
}
