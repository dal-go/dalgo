package dal

import (
	"context"
	"errors"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestReadAll(t *testing.T) {
	for _, tt := range []struct {
		name                 string
		reader               Reader
		shouldPanic          bool
		expectedRecordsCount int
		expectedErrTexts     []string
	}{
		{name: "nil_reader_no_limit", reader: nil, shouldPanic: true},
		{name: "empty_reader", reader: &EmptyReader{}, expectedRecordsCount: 0},
		{name: "2records", reader: NewRecordsReader([]Record{
			NewRecord(NewKeyWithID("collection", 1)),
			NewRecord(NewKeyWithID("collection", 2)),
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
			records, err := ReadAll(ctx, tt.reader, -1)
			if tt.expectedErrTexts == nil {
				assert.Nil(t, err)
				assert.Equal(t, tt.expectedRecordsCount, len(records))
			} else {
				assert.Nil(t, records)
				assert.NotNil(t, err)
				for _, expectedErrText := range tt.expectedErrTexts {
					assert.Contains(t, err.Error(), expectedErrText)
				}
			}
		})
	}
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
			reader           Reader
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

func TestEmptyReader(t *testing.T) {
	t.Run("Close", func(t *testing.T) {
		reader := &EmptyReader{}
		err := reader.Close()
		assert.Nil(t, err)
	})
	t.Run("Cursor", func(t *testing.T) {
		reader := &EmptyReader{}
		cursor, err := reader.Cursor()
		assert.Equal(t, ErrNotSupported, err)
		assert.Equal(t, "", cursor)
	})
	t.Run("Next", func(t *testing.T) {
		reader := &EmptyReader{}
		record, err := reader.Next()
		assert.Equal(t, ErrNoMoreRecords, err)
		assert.Nil(t, record)
	})
}
