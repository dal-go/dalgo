package dal

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewSelector(t *testing.T) {
	t.Run("panic_on_nil", func(t *testing.T) {
		assert.Panics(t, func() {
			NewSelector(Query{}, nil)
		})
	})
	t.Run("should_pass", func(t *testing.T) {
		getReader := func(c context.Context, query Query) (Reader, error) {
			return nil, nil
		}
		selector := NewSelector(Query{}, getReader)
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
			s := selector{
				query:     tt.args.query,
				getReader: tt.fields.getReader,
			}
			got, err := s.SelectReader(tt.args.c)
			if !tt.wantErr(t, err, fmt.Sprintf("Select(%v, %v)", tt.args.c, tt.args.query)) {
				return
			}
			assert.Equalf(t, tt.want, got, "Select(%v, %v)", tt.args.c, tt.args.query)
		})
	}
}

func Test_selector_SelectAll(t *testing.T) {
	t.Skip("TODO: implement test")
	type fields struct {
		query     Query
		getReader func(c context.Context, query Query) (Reader, error)
	}
	type args struct {
		c context.Context
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
			s := selector{
				query:     tt.fields.query,
				getReader: tt.fields.getReader,
			}
			gotRecords, err := s.SelectAllRecords(tt.args.c)
			if !tt.wantErr(t, err, fmt.Sprintf("SelectAll(%v)", tt.args.c)) {
				return
			}
			assert.Equalf(t, tt.wantRecords, gotRecords, "SelectAll(%v)", tt.args.c)
		})
	}
}

func Test_selector_SelectAllIDs(t *testing.T) {
	t.Skip("TODO: implement test")
	type fields struct {
		query     Query
		getReader func(c context.Context, query Query) (Reader, error)
	}
	type args struct {
		c context.Context
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantIds []any
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "empty", args: args{c: context.Background()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := selector{
				query:     tt.fields.query,
				getReader: tt.fields.getReader,
			}
			gotIds, err := s.SelectAllIDs(tt.args.c)
			if !tt.wantErr(t, err, fmt.Sprintf("SelectAllIDs(%v)", tt.args.c)) {
				return
			}
			assert.Equalf(t, tt.wantIds, gotIds, "SelectAllIDs(%v)", tt.args.c)
		})
	}
}

func Test_selector_SelectAllInt64IDs(t *testing.T) {
	t.Skip("TODO: implement test")
	type fields struct {
		query     Query
		getReader func(c context.Context, query Query) (Reader, error)
	}
	type args struct {
		c context.Context
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantIds []int64
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "empty", args: args{c: context.Background()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := selector{
				query:     tt.fields.query,
				getReader: tt.fields.getReader,
			}
			gotIds, err := s.SelectAllInt64IDs(tt.args.c)
			if !tt.wantErr(t, err, fmt.Sprintf("SelectAllInt64IDs(%v)", tt.args.c)) {
				return
			}
			assert.Equalf(t, tt.wantIds, gotIds, "SelectAllInt64IDs(%v)", tt.args.c)
		})
	}
}

func Test_selector_SelectAllIntIDs(t *testing.T) {
	t.Skip("TODO: implement test")
	type fields struct {
		query     Query
		getReader func(c context.Context, query Query) (Reader, error)
	}
	type args struct {
		c context.Context
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantIds []int
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "empty", args: args{c: context.Background()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := selector{
				query:     tt.fields.query,
				getReader: tt.fields.getReader,
			}
			gotIds, err := s.SelectAllIntIDs(tt.args.c)
			if !tt.wantErr(t, err, fmt.Sprintf("SelectAllIntIDs(%v)", tt.args.c)) {
				return
			}
			assert.Equalf(t, tt.wantIds, gotIds, "SelectAllIntIDs(%v)", tt.args.c)
		})
	}
}

func Test_selector_SelectAllStrIDs(t *testing.T) {
	t.Skip("TODO: implement test")
	type fields struct {
		query     Query
		getReader func(c context.Context, query Query) (Reader, error)
	}
	type args struct {
		c context.Context
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantIds []string
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := selector{
				query:     tt.fields.query,
				getReader: tt.fields.getReader,
			}
			gotIds, err := s.SelectAllStrIDs(tt.args.c)
			if !tt.wantErr(t, err, fmt.Sprintf("SelectAllStrIDs(%v)", tt.args.c)) {
				return
			}
			assert.Equalf(t, tt.wantIds, gotIds, "SelectAllStrIDs(%v)", tt.args.c)
		})
	}
}

func Test_selector_selectAllIDsWorker(t *testing.T) {
	t.Skip("TODO: implement test")
	type fields struct {
		getReader func(c context.Context, query Query) (Reader, error)
	}
	type args struct {
		c     context.Context
		query Query
		addID func(id any) error
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := selector{
				getReader: tt.fields.getReader,
			}
			tt.wantErr(t, s.selectAllIDsWorker(tt.args.c, tt.args.query, tt.args.addID), fmt.Sprintf("selectAllIDsWorker(%v, %v)", tt.args.c, tt.args.query))
		})
	}
}
