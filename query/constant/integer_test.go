package constant

import (
	"reflect"
	"testing"
)

func TestInt(t *testing.T) {
	type args struct {
		v int
	}
	tests := []struct {
		name string
		args args
		want integer
	}{
		{
			name: "0",
			args: args{0},
			want: integer{value: 0},
		},
		{
			name: "10",
			args: args{10},
			want: integer{value: 10},
		},
		{
			name: "-20",
			args: args{-20},
			want: integer{value: -20},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Int(tt.args.v); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Int() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStr(t *testing.T) {
	type args struct {
		v string
	}
	tests := []struct {
		name string
		args args
		want str
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Str(tt.args.v); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Str() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_integer_String(t *testing.T) {
	type fields struct {
		value int
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name:   "1",
			fields: fields{value: 1},
			want:   "1",
		},
		{
			name:   "-21",
			fields: fields{value: -21},
			want:   "-21",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := integer{
				value: tt.fields.value,
			}
			if got := i.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_str_String(t *testing.T) {
	type fields struct {
		value string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name:   "empty",
			fields: fields{value: ""},
			want:   "''",
		},
		{
			name:   "aBc",
			fields: fields{value: "aBc"},
			want:   "'aBc'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := str{
				value: tt.fields.value,
			}
			if got := v.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}
