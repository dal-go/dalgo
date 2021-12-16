package orm

import (
	"reflect"
	"testing"
)

func TestNewStringField(t *testing.T) {
	type args struct {
		name string
	}
	tests := []struct {
		name string
		args args
		want StringField
	}{
		{
			name: "str1",
			args: args{name: "str1"},
			want: stringField{field: field{name: "str1"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewStringField(tt.args.name); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewStringField() = %v, want %v", got, tt.want)
			}
		})
	}
}
