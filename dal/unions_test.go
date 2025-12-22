package dal

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestArrayUnion_Name(t *testing.T) {
	assert.Equal(t, "ArrayUnion", ArrayUnion().Name())
}

func TestArrayUnion_Value(t *testing.T) {
	assert.Nil(t, arrayUnion{}.Value())
	assert.Equal(t, []any{"s1"}, arrayUnion{elems: []any{"s1"}}.Value())
}

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
