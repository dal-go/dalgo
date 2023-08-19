package dal

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMakeRecordData(t *testing.T) {
	type args[T any] struct {
		data T
	}
	type testCase[T any] struct {
		name string
		args args[T]
	}
	tests := []testCase[any]{
		{
			name: "empty",
			args: args[any]{
				data: make(map[string]string),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recordData := MakeRecordData(tt.args.data)
			assert.Equalf(t, tt.args.data, recordData.Data(), "MakeRecordData(%v)", tt.args.data)
		})
	}
}

func TestMakeRecordDataWithCallbacks(t *testing.T) {
	type args[T any] struct {
		data       T
		beforeSave RecordDataHook
		afterLoad  RecordDataHook
	}
	type testCase[T any] struct {
		name string
		args args[T]
	}
	tests := []testCase[any]{
		{
			name: "empty",
			args: args[any]{
				data:       make(map[string]string),
				beforeSave: nil,
				afterLoad:  nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var options []RecordDataOption
			if tt.args.beforeSave != nil {
				options = append(options, WithBeforeSave(tt.args.beforeSave))
			}
			if tt.args.afterLoad != nil {
				options = append(options, WithAfterLoad(tt.args.afterLoad))
			}
			rd := MakeRecordData(tt.args.data, options...)
			assert.NotNil(t, rd)
			assert.NotNil(t, rd.Data())
		})
	}
}

func Test_recordData(t *testing.T) {
	for _, tt := range []struct {
		name string
		rd   *recordData
	}{
		{
			name: "empty",
			rd:   &recordData{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("Data", func(t *testing.T) {
				t.Run("Data", func(t *testing.T) {
					assert.Equal(t, tt.rd.data, tt.rd.Data())
				})
				t.Run("String", func(t *testing.T) {
					assert.Equal(t, fmt.Sprintf("%v", tt.rd.Data()), tt.rd.String())
				})
			})
		})
	}
}
