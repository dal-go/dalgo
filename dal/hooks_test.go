package dal

import (
	"context"
	"errors"
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

type testValidateData struct {
	isValid bool
}

func (t testValidateData) Validate() error {
	if t.isValid {
		return nil
	}
	return errors.New("invalid")
}

func TestBeforeSave(t *testing.T) {
	type test struct {
		name string
		data testValidateData
	}
	tests := []test{
		{
			name: "valid",
			data: testValidateData{isValid: true},
		},
		{
			name: "invalid",
			data: testValidateData{isValid: false},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := test.data
			err := BeforeSave(context.Background(), nil, NewRecordWithIncompleteKey("test", reflect.Struct, data).SetError(NoError))
			if data.isValid {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
			}
		})
	}
}

func TestCallRecordHooks(t *testing.T) {
	for _, tt := range []struct {
		name       string
		hooks      []RecordHook
		expectsErr bool
	}{
		{
			name:       "nil_hooks",
			hooks:      nil,
			expectsErr: false,
		},
		{
			name:       "empty_hooks",
			hooks:      []RecordHook{},
			expectsErr: false,
		},
		{
			name: "one_hook_with_no_error",
			hooks: []RecordHook{
				func(c context.Context, record Record) error {
					return nil
				},
			},
			expectsErr: false,
		},
		{
			name: "one_hook_with_error",
			hooks: []RecordHook{
				func(c context.Context, record Record) error {
					return errors.New("error")
				},
			},
			expectsErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := callRecordHooks(context.Background(), nil, tt.hooks)
			if tt.expectsErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}
