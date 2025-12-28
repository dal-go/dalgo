package dalgo2fs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/stretchr/testify/assert"
)

func TestNewDirReader(t *testing.T) {
	fileNameCol := NewFileNameColumn()
	fileSizeCol := NewFileSizeColumn()
	fileExtCol := NewFileExtColumn()
	isDirCol := NewIsDirColumn()

	reader, err := NewDirReader("./test-fs-db",
		recordset.UntypedCol(fileNameCol),
		recordset.UntypedCol(isDirCol),
		recordset.UntypedCol(fileExtCol),
		recordset.UntypedCol(fileSizeCol),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = reader.Close()
	}()
	for {
		_, _, err = reader.Next()
		if err != nil {
			if errors.Is(err, dal.ErrNoMoreRecords) {
				break
			}
			t.Fatal(err)
		}
	}
	rs := reader.Recordset()
	assert.NotNil(t, rs)
	assert.Greater(t, rs.RowsCount(), 0)
	row0 := rs.GetRow(0)
	assert.NotNil(t, row0)
	fileNames := fileNameCol.Values()
	assert.Equal(t, []string{
		"README.md",
		"anna.json",
		"bob.json",
		"jack.yaml",
	}, fileNames)
	fileExtensions := fileExtCol.Values()
	assert.Equal(t, []string{
		".md",
		".json",
		".json",
		".yaml",
	}, fileExtensions)
}

func TestNewDirReaderWithInvalidPath(t *testing.T) {
	reader, err := NewDirReader("/nonexistent/path")
	assert.Nil(t, reader)
	assert.Error(t, err)
}

func TestDirReader_Cursor(t *testing.T) {
	reader, _ := NewDirReader("./test-fs-db")
	cursor, err := reader.Cursor()
	assert.Equal(t, "", cursor)
	assert.Equal(t, dal.ErrNotSupported, err)
}

func TestDirReader_Next_Error(t *testing.T) {
	// To test error in Next when fi, err = dirEntry.Info() fails.
	// We can't easily mock os.DirEntry, so we might need to skip or find a creative way.
	// But we can test other columns.
	t.Run("ModifiedColumn", func(t *testing.T) {
		modifiedCol := NewFileModifiedColumn()
		reader, err := NewDirReader("./test-fs-db", recordset.UntypedCol(modifiedCol))
		assert.NoError(t, err)
		_, _, err = reader.Next()
		assert.NoError(t, err)
	})
}

func TestDirReader_Next_EOF(t *testing.T) {
	reader, _ := NewDirReader("./test-fs-db")
	// Drain it
	for {
		_, _, err := reader.Next()
		if err != nil {
			break
		}
	}
	_, _, err := reader.Next()
	assert.Equal(t, dal.ErrNoMoreRecords, err)
}

func TestDirReader_Next_SkipDirs(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "dalgo2fs_skip_dirs")
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	_ = os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("test"), 0644)

	reader, _ := NewDirReader(tmpDir)
	row, _, err := reader.Next()
	assert.NoError(t, err)
	assert.NotNil(t, row)

	_, _, err = reader.Next()
	assert.Equal(t, dal.ErrNoMoreRecords, err)

	t.Run("LastEntryIsDir", func(t *testing.T) {
		tmpDir2, _ := os.MkdirTemp("", "dalgo2fs_last_dir")
		defer func() {
			_ = os.RemoveAll(tmpDir2)
		}()
		_ = os.Mkdir(filepath.Join(tmpDir2, "zzz_subdir"), 0755)
		reader2, _ := NewDirReader(tmpDir2)
		_, _, err := reader2.Next()
		assert.Equal(t, dal.ErrNoMoreRecords, err)
	})

	t.Run("FilteredFileInfoColumns", func(t *testing.T) {
		tmpDir3, _ := os.MkdirTemp("", "dalgo2fs_filtered_cols")
		defer func() {
			_ = os.RemoveAll(tmpDir3)
		}()
		_ = os.WriteFile(filepath.Join(tmpDir3, "file.txt"), []byte("test"), 0644)

		// Use a subset of columns to trigger more branches in Next
		reader3, _ := NewDirReader(tmpDir3,
			recordset.UntypedCol(NewFileNameColumn()),
			recordset.UntypedCol(NewFileSizeColumn()),
			recordset.UntypedCol(NewFileModifiedColumn()),
		)
		row, _, err := reader3.Next()
		assert.NoError(t, err)
		assert.NotNil(t, row)
	})
}
