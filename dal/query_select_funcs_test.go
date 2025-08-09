package dal

import (
	"errors"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSelectAll(t *testing.T) {
	type args struct {
		reader func() Reader
		limit  int
	}
	type testCase[T comparable] struct {
		name        string
		args        args
		shouldPanic bool
		wantIds     []T
		wantErr     error
	}

	getRecordsReader := func() Reader {
		return NewRecordsReader([]Record{
			&record{key: &Key{ID: 1, collection: "test"}},
			&record{key: &Key{ID: 2, collection: "test"}},
			&record{key: &Key{ID: 3, collection: "test"}},
			&record{key: &Key{ID: 4, collection: "test"}},
		})
	}

	tests := []testCase[int]{
		{name: "nil_reader", shouldPanic: true, args: args{reader: func() Reader {
			return nil
		}}},
		{name: "empty_reader", args: args{reader: func() Reader {
			return &EmptyReader{}
		}}, wantIds: []int{}, wantErr: nil},
		{
			name: "with_records_0_limit",
			args: args{
				limit:  0,
				reader: getRecordsReader,
			},
			wantIds: []int{1, 2, 3, 4},
		},
		{
			name: "with_records_negative_limit",
			args: args{
				reader: getRecordsReader,
				limit:  -1,
			},
			wantIds: []int{1, 2, 3, 4},
		},
		{
			name: "with_records_limit_2",
			args: args{
				limit:  2,
				reader: getRecordsReader,
			},
			wantIds: []int{1, 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertErr := func(t *testing.T, err error) {
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
				if tt.shouldPanic {
					defer func() {
						if r := recover(); r == nil {
							t.Errorf("expected panic")
						}
					}()
				}
				gotIds, err := SelectAllIDs[int](tt.args.reader(), WithLimit(tt.args.limit))
				assertErr(t, err)
				assert.Equal(t, tt.wantIds, gotIds)
			})
			t.Run("SelectAllRecords", func(t *testing.T) {
				if tt.shouldPanic {
					defer func() {
						if r := recover(); r == nil {
							t.Errorf("expected panic")
						}
					}()
				}
				gotRecords, err := SelectAllRecords(tt.args.reader(), WithLimit(tt.args.limit))
				assertErr(t, err)
				if err == nil {
					assert.NotNil(t, gotRecords)
				}
			})
		})
	}
}

func TestSelectAll_WithOffset(t *testing.T) {
	getRecordsReader := func() Reader {
		return NewRecordsReader([]Record{
			&record{key: &Key{ID: 1, collection: "test"}},
			&record{key: &Key{ID: 2, collection: "test"}},
			&record{key: &Key{ID: 3, collection: "test"}},
			&record{key: &Key{ID: 4, collection: "test"}},
		})
	}
	// Offset only
	ids, err := SelectAllIDs[int](getRecordsReader(), WithOffset(2))
	assert.NoError(t, err)
	assert.Equal(t, []int{3, 4}, ids)
	// Offset + limit smaller than remaining
	ids, err = SelectAllIDs[int](getRecordsReader(), WithOffset(1), WithLimit(2))
	assert.NoError(t, err)
	assert.Equal(t, []int{2, 3}, ids)
	// Offset exceeds available
	ids, err = SelectAllIDs[int](getRecordsReader(), WithOffset(10))
	assert.NoError(t, err)
	assert.Equal(t, []int(nil), ids)
	// Zero offset behaves as no offset
	ids, err = SelectAllIDs[int](getRecordsReader(), WithOffset(0))
	assert.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3, 4}, ids)
}

func TestWithOffset(t *testing.T) {
	ro := new(readerOptions)
	WithOffset(3)(ro)
	assert.Equal(t, 3, ro.offset)
	ro.offset = 0
	assert.Equal(t, readerOptions{}, *ro)
}

func TestWithLimit(t *testing.T) {
	ro := new(readerOptions)
	WithLimit(4)(ro)
	assert.Equal(t, 4, ro.limit)
	ro.limit = 0
	assert.Equal(t, readerOptions{}, *ro)
}
