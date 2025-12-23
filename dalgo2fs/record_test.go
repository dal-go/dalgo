package dalgo2fs

import (
	"errors"
	"os"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
)

func TestNewFileRecord(t *testing.T) {
	key := dal.NewKeyWithID("test-collection", "test-id")
	record := NewFileRecord(key)
	assert.NotNil(t, record)
	assert.Equal(t, key, record.Key())
}

func TestFileRecord_Key(t *testing.T) {
	key := dal.NewKeyWithID("test-collection", "test-id")
	record := &fileRecord{key: key}
	assert.Equal(t, key, record.Key())
}

func TestFileRecord_Error(t *testing.T) {
	err := errors.New("test error")
	record := &fileRecord{err: err}
	assert.Equal(t, err, record.Error())
}

func TestFileRecord_Exists(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		record := &fileRecord{
			data: fileData{fi: &mockFileInfo{}},
		}
		assert.True(t, record.Exists())

		record.err = errors.New("some error")
		assert.False(t, record.Exists())
	})

	t.Run("not_exists", func(t *testing.T) {
		record := &fileRecord{}
		assert.False(t, record.Exists())
	})
}

func TestFileRecord_SetError(t *testing.T) {
	record := &fileRecord{}
	err := errors.New("test error")
	record.SetError(err)
	assert.Equal(t, err, record.Error())

	record.SetError(nil)
	assert.Equal(t, dal.ErrNoError, record.Error())
}

func TestFileRecord_Data(t *testing.T) {
	data := fileData{fi: &mockFileInfo{}}
	record := &fileRecord{data: data}
	assert.Equal(t, data, record.Data())
}

func TestFileRecord_SetData(t *testing.T) {
	data := fileData{fi: &mockFileInfo{}}
	record := &fileRecord{}
	record.setData(data)
	assert.Equal(t, data, record.data)
}

func TestFileRecord_HasChanged(t *testing.T) {
	record := &fileRecord{}
	assert.False(t, record.HasChanged())
	record.changed = true
	assert.True(t, record.HasChanged())
}

func TestFileRecord_MarkAsChanged(t *testing.T) {
	record := &fileRecord{}
	record.MarkAsChanged()
	assert.True(t, record.HasChanged())
}

type mockFileInfo struct {
	os.FileInfo
}
