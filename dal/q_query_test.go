package dal

import (
	"reflect"
	"testing"
)

func Test_field_EqualTo(t *testing.T) {
	const fieldName = "test_field"
	type fields struct {
		Name string
	}
	type args struct {
		v any
	}
	type tst struct {
		name   string
		fields fields
		args   args
		want   Condition
	}
	test := func(name string, value any) tst {
		return tst{
			name:   name,
			fields: fields{Name: fieldName},
			args:   args{v: 1},
			want: equal{
				Comparison: Comparison{
					Operator:    "==",
					Expressions: []Expression{FieldRef{Name: fieldName}, constantCondition{Value: 1}},
				},
			},
		}
	}
	tests := []tst{
		test("int_0", 0),
		test("int_1", 1),
		test("string_empty", ""),
		test("string_abc", "abc"),
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := FieldRef{
				Name: tt.fields.Name,
			}
			if got := f.EqualTo(tt.args.v); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("EqualTo(%v) = %v, want %v", tt.args.v, got, tt.want)
			}
		})
	}
}
