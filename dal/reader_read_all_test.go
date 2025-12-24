package dal

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/recordset"
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
		}}, wantIds: []int(nil), wantErr: nil},
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
				if err == nil && tt.name != "empty_reader" {
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

func (m mockDB) ExecuteQueryToRecordsReader(ctx context.Context, query Query) (RecordsReader, error) {
	return m.executeQueryToRecordsReader(ctx, query)
}

func (m mockDB) ExecuteQueryToRecordsetReader(ctx context.Context, query Query, options ...recordset.Option) (RecordsetReader, error) {
	return m.executeQueryToRecordsetReader(ctx, query, options...)
}

type mockDB struct {
	DB
	executeQueryToRecordsReader   func(ctx context.Context, query Query) (RecordsReader, error)
	executeQueryToRecordsetReader func(ctx context.Context, query Query, options ...recordset.Option) (RecordsetReader, error)
}

func TestExecuteQueryAndReadAllToRecords(t *testing.T) {
	ctx := context.Background()
	query := (Query)(nil)

	t.Run("success", func(t *testing.T) {
		db := mockDB{
			executeQueryToRecordsReader: func(ctx context.Context, query Query) (RecordsReader, error) {
				return NewRecordsReader([]Record{
					NewRecord(NewKeyWithID("test", 1)),
				}), nil
			},
		}
		records, err := ExecuteQueryAndReadAllToRecords(ctx, query, db)
		assert.NoError(t, err)
		assert.Len(t, records, 1)
	})

	t.Run("db_error", func(t *testing.T) {
		db := mockDB{
			executeQueryToRecordsReader: func(ctx context.Context, query Query) (RecordsReader, error) {
				return nil, errors.New("db error")
			},
		}
		records, err := ExecuteQueryAndReadAllToRecords(ctx, query, db)
		assert.Error(t, err)
		assert.Equal(t, "db error", err.Error())
		assert.Nil(t, records)
	})

	t.Run("reader_error", func(t *testing.T) {
		db := mockDB{
			executeQueryToRecordsReader: func(ctx context.Context, query Query) (RecordsReader, error) {
				return &errOnFirstNextReader{}, nil
			},
		}
		records, err := ExecuteQueryAndReadAllToRecords(ctx, query, db)
		assert.Error(t, err)
		assert.Equal(t, "next failed", err.Error())
		assert.Empty(t, records)
	})

	t.Run("close_error", func(t *testing.T) {
		db := mockDB{
			executeQueryToRecordsReader: func(ctx context.Context, query Query) (RecordsReader, error) {
				return &errOnCloseReader{}, nil
			},
		}
		records, err := ExecuteQueryAndReadAllToRecords(ctx, query, db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to close reader: close failed")
		assert.Empty(t, records)
	})
}

func TestExecuteQueryAndReadAllToRecordset(t *testing.T) {
	ctx := context.Background()
	q := mockQuery{}

	t.Run("success", func(t *testing.T) {
		rs := &mockRecordset{}
		reader := &mockRecordsetReader{rs: rs, count: 2}
		db := mockDB{
			executeQueryToRecordsetReader: func(ctx context.Context, query Query, options ...recordset.Option) (RecordsetReader, error) {
				return reader, nil
			},
		}
		gotRs, err := ExecuteQueryAndReadAllToRecordset(ctx, q, db)
		assert.NoError(t, err)
		assert.Equal(t, rs, gotRs)
		assert.True(t, reader.closed)
	})

	t.Run("db_error", func(t *testing.T) {
		db := mockDB{
			executeQueryToRecordsetReader: func(ctx context.Context, query Query, options ...recordset.Option) (RecordsetReader, error) {
				return nil, errors.New("db error")
			},
		}
		gotRs, err := ExecuteQueryAndReadAllToRecordset(ctx, q, db)
		assert.Error(t, err)
		assert.Equal(t, "failed to get the recordset reader: db error", err.Error())
		assert.Nil(t, gotRs)
	})

	t.Run("reader_error", func(t *testing.T) {
		rs := &mockRecordset{}
		reader := &mockRecordsetReader{rs: rs, errToReturn: errors.New("reader error")}
		db := mockDB{
			executeQueryToRecordsetReader: func(ctx context.Context, query Query, options ...recordset.Option) (RecordsetReader, error) {
				return reader, nil
			},
		}
		gotRs, err := ExecuteQueryAndReadAllToRecordset(ctx, q, db)
		assert.Error(t, err)
		assert.Equal(t, "failed to get next record: reader error", err.Error())
		assert.Equal(t, rs, gotRs)
		assert.True(t, reader.closed)
	})

	t.Run("with_offset_and_limit", func(t *testing.T) {
		rs := &mockRecordset{}
		reader := &mockRecordsetReader{rs: rs, count: 5}
		db := mockDB{
			executeQueryToRecordsetReader: func(ctx context.Context, query Query, options ...recordset.Option) (RecordsetReader, error) {
				return reader, nil
			},
		}
		gotRs, err := ExecuteQueryAndReadAllToRecordset(ctx, q, db, WithOffset(2), WithLimit(2))
		assert.True(t, errors.Is(err, ErrLimitReached))
		assert.Equal(t, rs, gotRs)
		assert.Equal(t, 4, reader.nextCalled) // 2 for offset + 2 for limit
		assert.True(t, reader.closed)
	})

	t.Run("limit_reached", func(t *testing.T) {
		rs := &mockRecordset{}
		reader := &mockRecordsetReader{rs: rs, count: 5}
		db := mockDB{
			executeQueryToRecordsetReader: func(ctx context.Context, query Query, options ...recordset.Option) (RecordsetReader, error) {
				return reader, nil
			},
		}
		gotRs, err := ExecuteQueryAndReadAllToRecordset(ctx, q, db, WithLimit(3))
		assert.True(t, errors.Is(err, ErrLimitReached))
		assert.Equal(t, rs, gotRs)
		assert.Equal(t, 3, reader.nextCalled)
		assert.True(t, reader.closed)
	})

	t.Run("offset_exceeds", func(t *testing.T) {
		rs := &mockRecordset{}
		reader := &mockRecordsetReader{rs: rs, count: 2}
		db := mockDB{
			executeQueryToRecordsetReader: func(ctx context.Context, query Query, options ...recordset.Option) (RecordsetReader, error) {
				return reader, nil
			},
		}
		gotRs, err := ExecuteQueryAndReadAllToRecordset(ctx, q, db, WithOffset(5))
		assert.NoError(t, err)
		assert.Equal(t, rs, gotRs)
		assert.Equal(t, 3, reader.nextCalled) // returns ErrNoMoreRecords on 3rd call
		assert.True(t, reader.closed)
	})
}

type mockQuery struct {
	Query
	recordsetOptions []recordset.Option
}

func (m mockQuery) GetRecordsetReader(ctx context.Context, qe QueryExecutor) (RecordsetReader, error) {
	return qe.ExecuteQueryToRecordsetReader(ctx, m, m.recordsetOptions...)
}

type mockRecordset struct {
	recordset.Recordset
}

type mockRecordsetReader struct {
	rs          recordset.Recordset
	count       int
	nextCalled  int
	errToReturn error
	closed      bool
	//err         int // not used as int, used as toggle or count for error
}

func (m *mockRecordsetReader) Recordset() recordset.Recordset {
	return m.rs
}

func (m *mockRecordsetReader) Next() (recordset.Row, recordset.Recordset, error) {
	m.nextCalled++
	if m.errToReturn != nil {
		return nil, nil, m.errToReturn
	}
	if m.nextCalled > m.count {
		return nil, nil, ErrNoMoreRecords
	}
	return &mockRow{}, m.rs, nil
}

func (m *mockRecordsetReader) Cursor() (string, error) {
	return "", nil
}

func (m *mockRecordsetReader) Close() error {
	m.closed = true
	return nil
}

type mockRow struct {
	recordset.Row
}

type errOnCloseReader struct{ EmptyReader }

func (e *errOnCloseReader) Close() error {
	return errors.New("close failed")
}
