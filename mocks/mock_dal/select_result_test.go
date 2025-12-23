package mock_dal

import (
	"errors"
	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"reflect"
	"testing"
	"time"
)

func TestNewRecordsReader(t *testing.T) {
	record := dal.NewRecordWithIncompleteKey("TestCollection", reflect.String, &struct{}{})
	reader := NewRecordsReader(0, record)
	assert.NotNil(t, reader)
}

func TestNewSelectResult(t *testing.T) {
	t.Run("with_nil_reader", func(t *testing.T) {
		result := NewSelectResult(nil, nil)
		assert.NotNil(t, result)
		assert.Nil(t, result.Reader)
		assert.Nil(t, result.Err)
	})

	t.Run("with_error", func(t *testing.T) {
		testErr := errors.New("test error")
		result := NewSelectResult(nil, testErr)
		assert.NotNil(t, result)
		assert.Nil(t, result.Reader)
		assert.Equal(t, testErr, result.Err)
	})

	t.Run("with_reader", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockReader := NewMockReader(ctrl)
		result := NewSelectResult(mockReader, nil)
		assert.NotNil(t, result)
		assert.Equal(t, mockReader, result.Reader)
		assert.Nil(t, result.Err)
	})
}

func TestRecordReader_Methods(t *testing.T) {
	t.Run("close", func(t *testing.T) {
		record := dal.NewRecord(dal.NewKeyWithID("test", "id1"))
		reader := NewRecordsReader(0, record)

		err := reader.Close()
		assert.NoError(t, err)
	})

	t.Run("cursor", func(t *testing.T) {
		record := dal.NewRecord(dal.NewKeyWithID("test", "id1"))
		reader := NewRecordsReader(0, record)

		cursor, err := reader.Cursor()
		assert.Equal(t, "", cursor)
		assert.NoError(t, err)
	})

	t.Run("next_with_records", func(t *testing.T) {
		record1 := dal.NewRecord(dal.NewKeyWithID("test", "id1"))
		record2 := dal.NewRecord(dal.NewKeyWithID("test", "id2"))
		reader := NewRecordsReader(0, record1, record2)

		// First record
		result, err := reader.Next()
		assert.Equal(t, record1, result)
		assert.NoError(t, err)

		// Second record
		result, err = reader.Next()
		assert.Equal(t, record2, result)
		assert.NoError(t, err)

		// No more records
		result, err = reader.Next()
		assert.Nil(t, result)
		assert.Error(t, err)
	})

	t.Run("next_after_cursor", func(t *testing.T) {
		record := dal.NewRecord(dal.NewKeyWithID("test", "id1"))
		reader := NewRecordsReader(0, record)

		// Call cursor to close reader
		_, _ = reader.Cursor()

		// Next should return error
		result, err := reader.Next()
		assert.Nil(t, result)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "reader is closed")
	})

	t.Run("next_with_delay", func(t *testing.T) {
		record := dal.NewRecord(dal.NewKeyWithID("test", "id-delayed"))
		reader := NewRecordsReader(1*time.Millisecond, record)

		result, err := reader.Next()
		assert.Equal(t, record, result)
		assert.NoError(t, err)
	})
}
