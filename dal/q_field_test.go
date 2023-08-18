package dal

import (
	"github.com/stretchr/testify/assert"
	"testing"
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
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, Field(tt.args.name), "Field(%v)", tt.args.name)
		})
	}
}

func Test_orderExpression_Descending(t *testing.T) {
	type fields struct {
		expression Expression
		descending bool
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := orderExpression{
				expression: tt.fields.expression,
				descending: tt.fields.descending,
			}
			assert.Equalf(t, tt.want, v.Descending(), "Descending()")
		})
	}
}

func Test_orderExpression_Expression(t *testing.T) {
	type fields struct {
		expression Expression
		descending bool
	}
	tests := []struct {
		name   string
		fields fields
		want   Expression
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := orderExpression{
				expression: tt.fields.expression,
				descending: tt.fields.descending,
			}
			assert.Equalf(t, tt.want, v.Expression(), "Expression()")
		})
	}
}

func Test_orderExpression_String(t *testing.T) {
	type fields struct {
		expression Expression
		descending bool
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := orderExpression{
				expression: tt.fields.expression,
				descending: tt.fields.descending,
			}
			assert.Equalf(t, tt.want, v.String(), "String()")
		})
	}
}
