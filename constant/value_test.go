package constant

import (
	"reflect"
	"testing"
)

func TestValue(t *testing.T) {
	type args struct {
		v any
	}
	tests := []struct {
		name string
		args args
		want ValueConst
	}{
		{
			name: "nil",
			args: args{v: nil},
			want: ValueConst{nil},
		},
		{
			name: "abc123",
			args: args{v: "abc123"},
			want: ValueConst{"abc123"},
		},
		{
			name: "number",
			args: args{v: 123},
			want: ValueConst{123},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Value(tt.args.v); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Str() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValueConst_String(t *testing.T) {
	tests := []struct {
		name string
		v    ValueConst
		want string
	}{
		{
			name: "nil",
			v:    ValueConst{value: nil},
			want: "<nil>",
		},
		{
			name: "string",
			v:    ValueConst{value: "aBc"},
			want: "'aBc'",
		},
		{
			name: "int",
			v:    ValueConst{value: 123},
			want: "123",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}
