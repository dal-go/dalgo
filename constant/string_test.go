package constant

import (
	"reflect"
	"testing"
)

func TestStr(t *testing.T) {
	type args struct {
		v string
	}
	tests := []struct {
		name string
		args args
		want StrConst
	}{
		{
			name: "empty",
			args: args{v: ""},
			want: StrConst{""},
		},
		{
			name: "abc123",
			args: args{v: "abc123"},
			want: StrConst{"abc123"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Str(tt.args.v); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Str() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStrConst_String(t *testing.T) {
	tests := []struct {
		name string
		v    StrConst
		want string
	}{
		{
			name: "empty",
			v:    StrConst{value: ""},
			want: "''",
		},
		{
			name: "aBc",
			v:    StrConst{value: "aBc"},
			want: "'aBc'",
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
