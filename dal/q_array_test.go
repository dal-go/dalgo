package dal

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestArray_Equal(t *testing.T) {
	tests := []struct {
		name  string
		array Array
		input Array
		want  bool
	}{
		{
			name:  "both_empty",
			array: Array{},
			input: Array{},
			want:  true,
		},
		{
			name:  "non_slice_equal",
			array: Array{Value: "s1"},
			input: Array{Value: "s1"},
			want:  true,
		},
		{
			name:  "non_slice_not_equal",
			array: Array{Value: "s1"},
			input: Array{Value: "s2"},
			want:  false,
		},
		{
			name:  "string_slice_equal",
			array: Array{Value: []string{"a", "b", "c"}},
			input: Array{Value: []string{"a", "b", "c"}},
			want:  true,
		},
		{
			name:  "string_slice_not_equal",
			array: Array{Value: []string{"a", "b", "c"}},
			input: Array{Value: []string{"a", "b", "d"}},
			want:  false,
		},
		{
			name:  "different_length_slices",
			array: Array{Value: []string{"a", "b", "c"}},
			input: Array{Value: []string{"a", "b"}},
			want:  false,
		},
		{
			name:  "any_slice_equal",
			array: Array{Value: []any{"a", 1, true}},
			input: Array{Value: []any{"a", 1, true}},
			want:  true,
		},
		{
			name:  "any_slice_not_equal",
			array: Array{Value: []any{"a", 1, true}},
			input: Array{Value: []any{"a", 1, false}},
			want:  false,
		},
		{
			name:  "mixed_slice_types_equal",
			array: Array{Value: []any{"a", "b", "c"}},
			input: Array{Value: []string{"a", "b", "c"}},
			want:  true,
		},
		{
			name:  "mixed_slice_types_not_equal",
			array: Array{Value: []any{"a", "b", "c"}},
			input: Array{Value: []string{"a", "b", "d"}},
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.array.Equal(tt.input)
			assert.Equal(t, tt.want, actual)
		})
	}
}

func TestNewArray(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		wantPanic bool
	}{
		{
			name:      "string_slice",
			input:     []string{"a", "b", "c"},
			wantPanic: false,
		},
		{
			name:      "int_slice",
			input:     []int{1, 2, 3},
			wantPanic: false,
		},
		{
			name:      "int64_slice",
			input:     []int64{1, 2, 3},
			wantPanic: false,
		},
		{
			name:      "uint_slice",
			input:     []uint{1, 2, 3},
			wantPanic: false,
		},
		{
			name:      "uint8_slice",
			input:     []uint8{1, 2, 3},
			wantPanic: false,
		},
		{
			name:      "uint16_slice",
			input:     []uint16{1, 2, 3},
			wantPanic: false,
		},
		{
			name:      "uint32_slice",
			input:     []uint32{1, 2, 3},
			wantPanic: false,
		},
		{
			name:      "uint64_slice",
			input:     []uint64{1, 2, 3},
			wantPanic: false,
		},
		{
			name:      "float32_slice",
			input:     []float32{1.1, 2.2, 3.3},
			wantPanic: false,
		},
		{
			name:      "float64_slice",
			input:     []float64{1.1, 2.2, 3.3},
			wantPanic: false,
		},
		{
			name:      "empty_slice",
			input:     []string{},
			wantPanic: false,
		},
		{
			name:      "any_slice",
			input:     []any{"a", 1, true},
			wantPanic: true,
		},
		{
			name:      "non_slice_type",
			input:     "not a slice",
			wantPanic: true,
		},
		{
			name:      "nil_value",
			input:     nil,
			wantPanic: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				assert.Panics(t, func() {
					NewArray(tt.input)
				})
			} else {
				array := NewArray(tt.input)
				assert.Equal(t, tt.input, array.Value)
			}
		})
	}
}

func TestArray_String(t *testing.T) {
	tests := []struct {
		name      string
		array     Array
		want      string
		wantPanic bool
	}{
		{
			name:      "empty_array",
			array:     Array{},
			want:      "()",
			wantPanic: false,
		},
		{
			name:      "string_slice",
			array:     Array{Value: []string{"a", "b", "c"}},
			want:      "('a','b','c')",
			wantPanic: false,
		},
		{
			name:      "int_slice",
			array:     Array{Value: []int{1, 2, 3}},
			want:      "(1,2,3)",
			wantPanic: false,
		},
		{
			name:      "any_slice",
			array:     Array{Value: []any{"a", 1, true}},
			want:      "(a,1,true)",
			wantPanic: false,
		},
		{
			name:      "empty_slice",
			array:     Array{Value: []string{}},
			want:      "()",
			wantPanic: false,
		},
		{
			name:      "non_slice_type",
			array:     Array{Value: "not a slice"},
			want:      "",
			wantPanic: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				assert.Panics(t, func() {
					_ = tt.array.String()
				})
			} else {
				actual := tt.array.String()
				assert.Equal(t, tt.want, actual)
			}
		})
	}
}
