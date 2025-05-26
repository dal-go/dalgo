package dal

import (
	"testing"
)

func TestWithRandomStringKey(t *testing.T) {
	type args struct {
		length      int
		maxAttempts int
	}
	tests := []struct {
		name string
		args args
		want InsertOption
	}{
		{
			name: "1/1",
			args: args{
				length:      1,
				maxAttempts: 1,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			var io insertOptions
			WithRandomStringKey(tt.args.length, tt.args.maxAttempts)(&io)
		})
	}
}
