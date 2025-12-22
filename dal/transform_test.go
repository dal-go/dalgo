package dal

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIncrement(t *testing.T) {
	type args struct {
		v int
	}
	tests := []struct {
		name string
		args args
		want transform
	}{
		{name: "+1", args: args{v: 1}, want: transform{name: "increment", value: 1}},
		{name: "+2", args: args{v: 2}, want: transform{name: "increment", value: 2}},
		{name: "+3", args: args{v: 3}, want: transform{name: "increment", value: 3}},
		{name: "0", args: args{v: 0}, want: transform{name: "increment", value: 0}},
		{name: "-1", args: args{v: -1}, want: transform{name: "increment", value: -1}},
		{name: "-2", args: args{v: -2}, want: transform{name: "increment", value: -2}},
		{name: "-3", args: args{v: -3}, want: transform{name: "increment", value: -3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Increment(tt.args.v); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Increment() = %v, want %v", got, tt.want)
			} else if v := got.Value(); v != tt.want.value {
				t.Errorf("Increment().value() = %v, want.value = %v", v, tt.want)
			}
		})
	}
}

func TestIsTransform(t *testing.T) {
	t1 := transform{}
	t2, ok := IsTransform(t1)
	assert.True(t, ok)
	assert.Equal(t, t1, t2)
}

func TestTransform(t *testing.T) {
	t1 := transform{name: "t1", value: "v1"}
	assert.Equal(t, t1.name, t1.Name())
	assert.Equal(t, t1.value, t1.Value())
}
