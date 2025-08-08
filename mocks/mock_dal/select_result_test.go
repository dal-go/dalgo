package mock_dal

import (
	"github.com/dal-go/dalgo/dal"
	"reflect"
	"testing"
)

func TestNewRecordsReader(t *testing.T) {
	NewRecordsReader(0, dal.NewRecordWithIncompleteKey("TestCollection", reflect.String, &struct{}{}))
}

func TestNewSelectResult(t *testing.T) {
	type args struct {
		getReader func(into func() dal.Record) dal.Reader
		err       error
	}
	tests := []struct {
		name string
		args args
		want SelectResult
	}{
		{"empty", args{
			getReader: func(into func() dal.Record) dal.Reader {
				return nil
			},
			err: nil,
		}, SelectResult{nil, nil}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := tt.args.getReader(nil)
			_ = NewSelectResult(reader, tt.args.err)
		})
	}
}
