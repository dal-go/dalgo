package dal

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestWithHooks(t *testing.T) {
	t.Run("WithBeforeSave", func(t *testing.T) {
		testHook(t, WithBeforeSave, func(rd *recordData, ctx context.Context, db DB, key *Key) (err error) {
			return rd.BeforeSave(ctx, db, key)
		})
	})
	t.Run("WithAfterLoad", func(t *testing.T) {
		testHook(t, WithAfterLoad, func(rd *recordData, ctx context.Context, db DB, key *Key) (err error) {
			return rd.AfterLoad(ctx, db, key)
		})
	})
}

func testHook(
	t *testing.T,
	hookFactory func(hook RecordDataHook) func(rd *recordData),
	callHook func(rd *recordData, ctx context.Context, db DB, key *Key) (err error),
) {
	originalData := "abc"

	var called int
	var hook = func(ctx context.Context, db DB, key *Key, data any) (err error) {
		called++
		assert.Equal(t, originalData, data)
		return nil
	}
	rd := MakeRecordData(originalData, hookFactory(hook)).(*recordData)
	assert.NotNil(t, rd)
	ctx := context.Background()
	err := callHook(rd, ctx, nil, nil)
	assert.Nil(t, err)
	assert.Equal(t, 1, called)

}
