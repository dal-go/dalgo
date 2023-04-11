package dal

import (
	"reflect"
	"testing"
)

func TestArrayUnion(t *testing.T) {
	type args struct {
		elems []any
	}
	tests := []struct {
		name string
		args args
		want Transform
	}{
		{
			name: "nil args",
			args: args{elems: nil},
			want: ArrayUnion(),
		},
		{
			name: "single element",
			args: args{elems: []any{"s1"}},
			want: ArrayUnion([]any{"s1"}...),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ArrayUnion(tt.args.elems...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ArrayUnion() = %v, want %v", got, tt.want)
			}
		})
	}
}
