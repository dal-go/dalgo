package dal

import (
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewErrNotFoundByKey(t *testing.T) {
	type args struct {
		key   *Key
		cause error
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "ErrRecordNotFound",
			args: args{
				key:   NewKeyWithID("Foo", "bar"),
				cause: ErrRecordNotFound,
			},
		},
		{
			name: "nil",
			args: args{
				key:   NewKeyWithID("Foo", "bar"),
				cause: nil,
			},
		},
		{
			name: "some_error",
			args: args{
				key:   NewKeyWithID("Foo", "bar"),
				cause: errors.New("some error"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewErrNotFoundByKey(tt.args.key, tt.args.cause)
			assert.True(t, IsNotFound(err))

			err2 := fmt.Errorf("wrapper: %w", err)
			assert.True(t, IsNotFound(err2))
		})
	}
}
