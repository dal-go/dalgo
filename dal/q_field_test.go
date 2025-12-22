package dal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAscending(t *testing.T) {
	type args struct {
		expression Expression
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Ascending",
			args: args{
				expression: Field("date"),
			},
			want: "date",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := Ascending(tt.args.expression)
			assert.Equal(t, tt.want, actual.String())
		})
	}
}

func TestAscendingField(t *testing.T) {
	type args struct {
		name string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "AscendingField",
			args: args{
				name: "date1",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := AscendingField(tt.args.name)
			assert.Equal(t, tt.args.name, actual.String())
		})
	}
}

func TestDescending(t *testing.T) {
	type args struct {
		expression Expression
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "field",
			args: args{
				expression: Field("date"),
			},
			want: "date DESC",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := Descending(tt.args.expression)
			assert.Equal(t, tt.want, actual.String())
		})
	}
}

func TestDescendingField(t *testing.T) {
	type args struct {
		name string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "DescendingField",
			args: args{name: "date1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := DescendingField(tt.args.name)
			assert.Equal(t, tt.args.name+" DESC", actual.String())
		})
	}
}

func TestField(t *testing.T) {
	type args struct {
		name string
	}
	tests := []struct {
		name string
		args args
		want FieldRef
	}{
		{name: "field", args: args{name: "date"}, want: FieldRef{name: "date"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := Field(tt.args.name)
			assert.Equal(t, tt.want, actual)
		})
	}
}

func Test_orderExpression(t *testing.T) {
	type fields struct {
		expression Expression
		descending bool
	}
	tests := []struct {
		name     string
		fields   fields
		expected string
	}{
		{
			name: "order",
			fields: fields{
				expression: Field("date1"),
				descending: true,
			},
			expected: "date1 DESC",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := orderExpression{
				expression: tt.fields.expression,
				descending: tt.fields.descending,
			}
			assert.Equalf(t, tt.fields.descending, v.Descending(), "Descending()")
			assert.Equalf(t, tt.fields.expression, v.Expression(), "Expression()")
			if tt.fields.descending {
				assert.Equalf(t, tt.expected, v.String(), "String()")
			} else {
				assert.Equalf(t, tt.expected, v.String(), "String()")
			}
		})
	}
}
