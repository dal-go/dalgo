package dal

import (
	"reflect"
	"testing"
)

func Test_FieldRef_EqualTo(t *testing.T) {
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
			args:   args{v: value},
			want: Comparison{
				Operator: Equal,
				Left:     FieldRef{name: fieldName},
				Right:    Constant{Value: value},
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
				name: tt.fields.Name,
			}
			if got := f.EqualTo(tt.args.v); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("EqualTo(%v) = %T:%v, want %T:%v", tt.args.v, got, got, tt.want, tt.want)
			}
		})
	}
}
