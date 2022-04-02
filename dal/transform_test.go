package dal

import (
	"reflect"
	"testing"
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
				t.Errorf("Increment().Value() = %v, want.value = %v", v, tt.want)
			}
		})
	}
}
