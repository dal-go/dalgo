package dal

import (
	"context"
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewSelector(t *testing.T) {
	t.Run("panic_on_nil", func(t *testing.T) {
		assert.Panics(t, func() {
			NewQueryExecutor(nil)
		})
	})
	t.Run("should_pass", func(t *testing.T) {
		var getReader = func(c context.Context, query Query) (Reader, error) {
			return nil, nil
		}
		selector := NewQueryExecutor(getReader)
		assert.NotNil(t, selector)
	})
}

func Test_selector_SelectReader(t *testing.T) {
	t.Skip("TODO: implement test")
	type fields struct {
		getReader func(c context.Context, query Query) (Reader, error)
	}
	type args struct {
		c     context.Context
		query Query
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
				query:     tt.args.query,
				getReader: tt.fields.getReader,
			}
			got, err := s.QueryReader(tt.args.c, tt.args.query)
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
		getReader func(c context.Context, query Query) (Reader, error)
	}
	type args struct {
		c     context.Context
		query Query
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
			gotRecords, err := s.QueryAllRecords(tt.args.c, tt.args.query)
			if !tt.wantErr(t, err, fmt.Sprintf("SelectAll(%v)", tt.args.c)) {
				return
			}
			assert.Equalf(t, tt.wantRecords, gotRecords, "SelectAll(%v)", tt.args.c)
		})
	}
}

func TestSelectAllIDs(t *testing.T) {
	type args struct {
		reader Reader
		limit  int
	}
	type testCase[T comparable] struct {
		name    string
		args    args
		wantIds []T
		wantErr error
	}
	tests := []testCase[int]{
		{name: "empty_reader", args: args{reader: emptyReader{}}, wantIds: nil, wantErr: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIds, err := SelectAllIDs[int](tt.args.reader, tt.args.limit)
			if tt.wantErr == nil && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.wantErr != nil && err == nil {
				t.Errorf("expected error: %v", tt.wantErr)
			}
			if tt.wantErr != nil && err != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("expected %v, got %v", tt.wantErr, err)
			}
			assert.Equal(t, tt.wantIds, gotIds)
		})
	}
}

var _ Reader = (*emptyReader)(nil)

type emptyReader struct {
}

func (e emptyReader) Next() (Record, error) {
	return nil, ErrNoMoreRecords
}

func (e emptyReader) Cursor() (string, error) {
	return "", ErrNotSupported
}
