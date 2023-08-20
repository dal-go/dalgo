package dal

import (
	"errors"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSelectAll(t *testing.T) {
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
		{name: "empty_reader", args: args{reader: EmptyReader{}}, wantIds: []int{}, wantErr: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertErr := func(err error) {
				if tt.wantErr == nil && err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tt.wantErr != nil && err == nil {
					t.Errorf("expected error: %v", tt.wantErr)
				}
				if tt.wantErr != nil && err != nil && !errors.Is(err, tt.wantErr) {
					t.Errorf("expected %v, got %v", tt.wantErr, err)
				}
			}
			t.Run("SelectAllIDs", func(t *testing.T) {
				gotIds, err := SelectAllIDs[int](tt.args.reader, tt.args.limit)
				assertErr(err)
				assert.Equal(t, tt.wantIds, gotIds)
			})
			t.Run("SelectAllRecords", func(t *testing.T) {
				gotRecords, err := SelectAllRecords(tt.args.reader, tt.args.limit)
				assertErr(err)
				if err == nil {
					assert.NotNil(t, gotRecords)
				}
			})
		})
	}
}
