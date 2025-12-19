package dal

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewQueryExecutor(t *testing.T) {
	t.Run("panic_on_nil", func(t *testing.T) {
		assert.Panics(t, func() {
			NewQueryExecutor(nil)
		})
	})
	t.Run("should_pass", func(t *testing.T) {
		var getReader = func(ctx context.Context, query Query) (Reader, error) {
			return nil, nil
		}
		selector := NewQueryExecutor(getReader)
		assert.NotNil(t, selector)
	})
}

func Test_selector_SelectReader(t *testing.T) {
	t.Skip("TODO: implement test")
	type fields struct {
		getReader func(ctx context.Context, query Query) (Reader, error)
	}
	type args struct {
		c     context.Context
		query structuredQuery
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    Reader
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := queryExecutor{
				getReader: tt.fields.getReader,
			}
			got, err := s.GetReader(tt.args.c, tt.args.query)
			if !tt.wantErr(t, err, fmt.Sprintf("Select(%v, %v)", tt.args.c, tt.args.query)) {
				return
			}
			assert.Equalf(t, tt.want, got, "Select(%v, %v)", tt.args.c, tt.args.query)
		})
	}
}

func Test_selector_QueryAllRecords(t *testing.T) {
	t.Skip("TODO: implement test")
	type fields struct {
		getReader func(ctx context.Context, query Query) (Reader, error)
	}
	type args struct {
		c     context.Context
		query structuredQuery
	}
	tests := []struct {
		name        string
		fields      fields
		args        args
		wantRecords []Record
		wantErr     assert.ErrorAssertionFunc
	}{
		{name: "empty", args: args{c: context.Background()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := queryExecutor{
				getReader: tt.fields.getReader,
			}
			gotRecords, err := s.ReadAllRecords(tt.args.c, tt.args.query)
			if !tt.wantErr(t, err, fmt.Sprintf("SelectAll(%v)", tt.args.c)) {
				return
			}
			assert.Equalf(t, tt.wantRecords, gotRecords, "SelectAll(%v)", tt.args.c)
		})
	}
}

func Test_queryExecutor(t *testing.T) {
	type args struct {
		c     context.Context
		query StructuredQuery
	}
	tests := []struct {
		name        string
		qe          queryExecutor
		args        args
		shouldPanic bool
		wantErr     assert.ErrorAssertionFunc
	}{
		{
			name: "returns_error",
			qe: queryExecutor{
				getReader: func(ctx context.Context, query Query) (reader Reader, err error) {
					return nil, errors.New("test not implemented")
				},
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.NotNil(t, err, i...)
			},
		},
		{
			name: "nil_reader",
			qe: queryExecutor{
				getReader: func(ctx context.Context, query Query) (reader Reader, err error) {
					return nil, nil
				},
			},
			shouldPanic: true,
		},
		{
			name: "empty_reader",
			qe: queryExecutor{
				getReader: func(ctx context.Context, query Query) (reader Reader, err error) {
					return &EmptyReader{}, nil
				},
			},
			shouldPanic: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runTest := func(t *testing.T, execut func() (any, error)) {
				if tt.shouldPanic {
					defer func() {
						if r := recover(); r == nil {
							t.Errorf("ReadAllRecords() should have panicked!")
						}
					}()
				}
				got, err := execut()
				if tt.wantErr(t, err, fmt.Sprintf("GetReader(%v, %v)", tt.args.c, tt.args.query)) {
					return
				}
				if err == nil {
					assert.Nil(t, got)
				}
			}
			t.Run("ReadAllRecords", func(t *testing.T) {
				runTest(t, func() (any, error) {
					return tt.qe.ReadAllRecords(tt.args.c, tt.args.query)
				})
			})
			t.Run("GetReader", func(t *testing.T) {
				runTest(t, func() (any, error) {
					return tt.qe.GetReader(tt.args.c, tt.args.query)
				})
			})
		})
	}
}
