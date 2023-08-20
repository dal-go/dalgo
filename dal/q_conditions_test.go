package dal

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestComparison(t *testing.T) {
	t.Run("Equal", testComparisonEqual)
	t.Run("String", testComparisonString)
}

func testComparisonEqual(t *testing.T) {

	tests := []struct {
		name string
		a    Comparison
		b    Comparison
		want bool
	}{
		{
			name: "both_empty",
			a:    Comparison{},
			b:    Comparison{},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.a.Equal(tt.b)
			assert.Equal(t, tt.want, actual)
		})
	}
}

func testComparisonString(t *testing.T) {
	tests := []struct {
		name       string
		comparison Comparison
		want       string
	}{
		{
			name:       "empty",
			comparison: Comparison{},
			want:       "<nil> {NO_OPERATOR} <nil>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.comparison.String()
			assert.Equal(t, tt.want, actual)
		})
	}
}

func TestIsGroupOperator(t *testing.T) {
	tests := []struct {
		input Operator
		want  bool
	}{
		{input: Operator(""), want: false},
		{input: In, want: true},
		{input: Equal, want: false},
		{input: LessThen, want: false},
		{input: LessOrEqual, want: false},
		{input: GreaterThen, want: false},
		{input: GreaterOrEqual, want: false},
	}
	for _, tt := range tests {
		name := string(tt.input)
		if name == "" {
			name = "empty_string"
		}
		t.Run(name, func(t *testing.T) {
			actual := IsGroupOperator(tt.input)
			assert.Equal(t, tt.want, actual)
		})
	}
}

func TestNewComparison(t *testing.T) {
	type args struct {
		left  Expression
		o     Operator
		right Expression
	}
	tests := []struct {
		name string
		args args
		want Comparison
	}{
		{
			name: "empty",
			args: args{},
			want: Comparison{},
		},
		{
			name: "equal_constants",
			args: args{
				left:  Constant{Value: 1},
				o:     Equal,
				right: Constant{Value: 2},
			},
			want: Comparison{
				Operator: Equal,
				Left:     Constant{Value: 1},
				Right:    Constant{Value: 2},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := NewComparison(tt.args.left, tt.args.o, tt.args.right)
			assert.Equal(t, tt.want, actual)
		})
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Expression
	}{
		{
			name:  "empty",
			input: "",
			want:  Constant{Value: ""},
		},
		{
			name:  "not_empty",
			input: "s1",
			want:  Constant{Value: "s1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := String(tt.input)
			assert.Equal(t, tt.want, actual)
		})
	}
}
