package dal

import (
	"errors"
	"github.com/stretchr/testify/assert"
	"testing"
)

var _ Reader = (*emptyReader)(nil)

type emptyReader struct {
}

func (e emptyReader) Close() error {
	return nil
}

func (e emptyReader) Next() (Record, error) {
	return nil, ErrNoMoreRecords
}

func (e emptyReader) Cursor() (string, error) {
	return "", ErrNotSupported
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
		{name: "empty_reader", args: args{reader: emptyReader{}}, wantIds: []int{}, wantErr: nil},
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
