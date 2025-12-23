package dal

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadAllToRecords(t *testing.T) {
	for _, tt := range []struct {
		name                 string
		reader               RecordsReader
		shouldPanic          bool
		expectedRecordsCount int
		expectedErrTexts     []string
	}{
		{name: "nil_reader_no_limit", reader: nil, shouldPanic: true},
		{name: "empty_reader", reader: &EmptyReader{}, expectedRecordsCount: 0},
		{name: "2records", reader: NewRecordsReader([]Record{
			NewRecord(NewKeyWithID("recordsetSource", 1)),
			NewRecord(NewKeyWithID("recordsetSource", 2)),
		}), expectedRecordsCount: 2},
		{name: "fails in next", reader: NewRecordsReader(nil), expectedErrTexts: []string{"no records"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("panic expected")
					}
				}()
			}
			ctx := context.Background()
			records, err := ReadAllToRecords(ctx, tt.reader)
			if tt.expectedErrTexts == nil {
				assert.Nil(t, err)
				assert.Equal(t, tt.expectedRecordsCount, len(records))
			} else {
				assert.Equal(t, 0, len(records))
				assert.NotNil(t, err)
				for _, expectedErrText := range tt.expectedErrTexts {
					assert.Contains(t, err.Error(), expectedErrText)
				}
			}
		})
	}

	t.Run("context_canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		reader := NewRecordsReader([]Record{
			NewRecord(NewKeyWithID("recordsetSource", 1)),
		})
		records, err := ReadAllToRecords(ctx, reader)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, context.Canceled))
		assert.Equal(t, 0, len(records))
	})
}

func TestRecordsReader(t *testing.T) {
	t.Run("Close", func(t *testing.T) {
		reader := &recordsReader{}
		err := reader.Close()
		assert.Nil(t, err)
	})
	t.Run("Cursor", func(t *testing.T) {
		reader := NewRecordsReader([]Record{NewRecord(NewKeyWithID("a", "b"))})

		cursor, err := reader.Cursor()
		assert.True(t, errors.Is(err, ErrReaderNotStarted))
		assert.Equal(t, "", cursor)

		_, err = reader.Next()
		assert.Nil(t, err)
		cursor, err = reader.Cursor()
		assert.Nil(t, err)
		assert.Equal(t, "0", cursor)

		_, err = reader.Next()
		assert.True(t, errors.Is(err, ErrNoMoreRecords))
		cursor, err = reader.Cursor()
		assert.Equal(t, ErrReaderClosed.Error(), err.Error())
		assert.Equal(t, "", cursor)

	})
	t.Run("Next", func(t *testing.T) {
		for _, tt := range []struct {
			name             string
			reader           RecordsReader
			expectedErr      error
			expectedErrTexts []string
		}{
			{name: "no_records", reader: NewRecordsReader(nil), expectedErrTexts: []string{"no records"}},
			{name: "empty_records", reader: NewRecordsReader([]Record{}), expectedErr: ErrNoMoreRecords},
			{name: "single_record", reader: NewRecordsReader([]Record{NewRecord(NewKeyWithID("a", "b"))}), expectedErr: nil},
		} {
			t.Run(tt.name, func(t *testing.T) {
				record, err := tt.reader.Next()

				if tt.expectedErr == nil && tt.expectedErrTexts == nil {
					assert.Nil(t, err)
					assert.NotNil(t, record)
				} else {
					if tt.expectedErr != nil {
						assert.True(t, errors.Is(err, tt.expectedErr))
						if errors.Is(err, ErrNoMoreRecords) {
							assert.Nil(t, record)
						}
					}
					if tt.expectedErrTexts != nil {
						for _, expectedErrText := range tt.expectedErrTexts {
							assert.Contains(t, err.Error(), expectedErrText)
						}
					}
				}
			})
		}
	})
}

func TestSelectAll(t *testing.T) {
	type args struct {
		reader func() RecordsReader
		limit  int
	}
	type testCase[T comparable] struct {
		name        string
		args        args
		shouldPanic bool
		wantIds     []T
		wantErr     error
	}

	getRecordsReader := func() RecordsReader {
		return NewRecordsReader([]Record{
			&record{key: &Key{ID: 1, collection: "test"}},
			&record{key: &Key{ID: 2, collection: "test"}},
			&record{key: &Key{ID: 3, collection: "test"}},
			&record{key: &Key{ID: 4, collection: "test"}},
		})
	}

	tests := []testCase[int]{
		{name: "nil_reader", shouldPanic: true, args: args{reader: func() RecordsReader {
			return nil
		}}},
		{name: "empty_reader", args: args{reader: func() RecordsReader {
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
				gotIds, err := SelectAllIDs[int](context.Background(), tt.args.reader(), WithLimit(tt.args.limit))
				assertErr(t, err)
				assert.Equal(t, tt.wantIds, gotIds)
			})
			t.Run("ReadAllToRecords", func(t *testing.T) {
				if tt.shouldPanic {
					defer func() {
						if r := recover(); r == nil {
							t.Errorf("expected panic")
						}
					}()
				}
				gotRecords, err := ReadAllToRecords(context.Background(), tt.args.reader(), WithLimit(tt.args.limit))
				assertErr(t, err)
				if err == nil {
					assert.NotNil(t, gotRecords)
				}
			})
		})
	}
}

func TestSelectAll_WithOffset(t *testing.T) {
	getRecordsReader := func() RecordsReader {
		return NewRecordsReader([]Record{
			&record{key: &Key{ID: 1, collection: "test"}},
			&record{key: &Key{ID: 2, collection: "test"}},
			&record{key: &Key{ID: 3, collection: "test"}},
			&record{key: &Key{ID: 4, collection: "test"}},
		})
	}
	// Offset only
	ids, err := SelectAllIDs[int](context.Background(), getRecordsReader(), WithOffset(2))
	assert.NoError(t, err)
	assert.Equal(t, []int{3, 4}, ids)
	// Offset + limit smaller than remaining
	ids, err = SelectAllIDs[int](context.Background(), getRecordsReader(), WithOffset(1), WithLimit(2))
	assert.NoError(t, err)
	assert.Equal(t, []int{2, 3}, ids)
	// Offset exceeds available
	ids, err = SelectAllIDs[int](context.Background(), getRecordsReader(), WithOffset(10))
	assert.NoError(t, err)
	assert.Equal(t, []int(nil), ids)
	// Zero offset behaves as no offset
	ids, err = SelectAllIDs[int](context.Background(), getRecordsReader(), WithOffset(0))
	assert.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3, 4}, ids)
}

func TestWithOffset(t *testing.T) {
	ro := new(ReaderOptions)
	WithOffset(3)(ro)
	assert.Equal(t, 3, ro.offset)
	ro.offset = 0
	assert.Equal(t, ReaderOptions{}, *ro)
}

func TestWithLimit(t *testing.T) {
	ro := new(ReaderOptions)
	WithLimit(4)(ro)
	assert.Equal(t, 4, ro.limit)
	ro.limit = 0
	assert.Equal(t, ReaderOptions{}, *ro)
}

// errOnFirstNextReader returns a non-ErrNoMoreRecords error on first Next()
type errOnFirstNextReader struct{ called bool }

func (e *errOnFirstNextReader) Next() (Record, error) {
	if !e.called {
		e.called = true
		return nil, errors.New("next failed")
	}
	return nil, ErrNoMoreRecords
}
func (e *errOnFirstNextReader) Cursor() (string, error) { return "", nil }
func (e *errOnFirstNextReader) Close() error            { return nil }

func TestSelectAll_WithOffsetError(t *testing.T) {
	// When offset > 0 and Next returns a non-ErrNoMoreRecords error while skipping,
	// SelectAll should return that error.
	r := &errOnFirstNextReader{}
	_, err := SelectAllIDs[int](context.Background(), r, WithOffset(1))
	if err == nil || err.Error() != "next failed" {
		t.Fatalf("expected 'next failed' error, got: %v", err)
	}
}
