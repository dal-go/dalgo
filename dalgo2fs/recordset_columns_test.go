package dalgo2fs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewFileNameColumn(t *testing.T) {
	col := NewFileNameColumn()
	assert.NotNil(t, col)
	assert.Equal(t, ColumnFileName, col.Name())
	assert.Equal(t, "", col.DefaultValue())
}

func TestNewFileExtColumn(t *testing.T) {
	col := NewFileExtColumn()
	assert.NotNil(t, col)
	assert.Equal(t, ColumnFileExt, col.Name())
	assert.Equal(t, "", col.DefaultValue())
}

func TestNewFileSizeColumn(t *testing.T) {
	col := NewFileSizeColumn()
	assert.NotNil(t, col)
	assert.Equal(t, ColumnFileSize, col.Name())
	assert.Equal(t, int64(0), col.DefaultValue())
}

func TestNewFileModifiedColumn(t *testing.T) {
	col := NewFileModifiedColumn()
	assert.NotNil(t, col)
	assert.Equal(t, ColumnFileModified, col.Name())
	assert.Equal(t, time.Time{}, col.DefaultValue())
}

func TestNewFileInfoColumns(t *testing.T) {
	cols := NewFileInfoColumns()
	assert.Len(t, cols, 4)

	assert.Equal(t, ColumnFileName, cols[0].Name())
	assert.Equal(t, ColumnFileSize, cols[1].Name())
	assert.Equal(t, ColumnFileExt, cols[2].Name())
	assert.Equal(t, ColumnFileModified, cols[3].Name())

	for _, col := range cols {
		assert.NotNil(t, col)
	}
}

func TestFileColumns_Methods(t *testing.T) {
	col := NewFileExtColumn()
	assert.Equal(t, "STRING", col.DbType())
	_ = col.Add(".txt")
	assert.Equal(t, ".txt", col.Values()[0])
}
