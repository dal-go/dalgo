package dalgo2fs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

func TestNewDB(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		dirPath string
		wantErr bool
	}{
		{
			name:    "absolute_path",
			dirPath: cwd,
			wantErr: false,
		},
		{
			name:    "current_dir",
			dirPath: ".",
			wantErr: false,
		},
		{
			name:    "relative_path",
			dirPath: "./",
			wantErr: false,
		},
		{
			name:    "home_dir",
			dirPath: "~",
			wantErr: false,
		},
		{
			name:    "home_dir_slash",
			dirPath: "~/",
			wantErr: false,
		},
		{
			name:    "home_dir_with_subdir",
			dirPath: "~/tmp_dalgo2fs_test",
			wantErr: false,
		},
		{
			name:    "non_existent_path",
			dirPath: "/non/existent/path/dalgo2fs",
			wantErr: true,
		},
	}

	// Create a temp dir in home for testing home expansion if it doesn't exist
	testHomeSubdir := filepath.Join(home, "tmp_dalgo2fs_test")
	if err := os.MkdirAll(testHomeSubdir, 0755); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(testHomeSubdir)
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := NewDB(tt.dirPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewDB(%q) error = %v, wantErr %v", tt.dirPath, err, tt.wantErr)
				return
			}
			if !tt.wantErr && db == nil {
				t.Error("NewDB() returned nil db and no error")
			}
		})
	}
}

func TestDatabase_Methods(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dalgo2fs_test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	db, err := NewDB(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("ID", func(t *testing.T) {
		if id := db.ID(); id != "dalgo2fs" {
			t.Errorf("expected dalgo2fs, got %s", id)
		}
	})

	t.Run("RunReadonlyTransaction", func(t *testing.T) {
		err := db.RunReadonlyTransaction(context.Background(), func(ctx context.Context, tx dal.ReadTransaction) error {
			return nil
		})
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("RunReadwriteTransaction", func(t *testing.T) {
		err := db.RunReadwriteTransaction(context.Background(), func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			return nil
		})
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("Get", func(t *testing.T) {
		// Create a file to get
		fileName := "test.txt"
		filePath := filepath.Join(tmpDir, fileName)
		if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}

		key := dal.NewKeyWithID("test", fileName)
		record := NewFileRecord(key)
		err := db.Get(context.Background(), record)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !record.Exists() {
			t.Error("expected record to exist")
		}

		// Test non-existent file
		key2 := dal.NewKeyWithID("test", "non-existent.txt")
		record2 := NewFileRecord(key2)
		err = db.Get(context.Background(), record2)
		if err == nil {
			t.Error("expected error for non-existent file")
		}

		// Test non-fileRecord
		mockRecord := dal.NewRecordWithData(key, &struct{}{})
		err = db.Get(context.Background(), mockRecord)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !errors.Is(mockRecord.Error(), dal.ErrNotSupported) {
			t.Errorf("expected dal.ErrNotSupported, got %v", mockRecord.Error())
		}
	})

	t.Run("Exists", func(t *testing.T) {
		fileName := "exists.txt"
		filePath := filepath.Join(tmpDir, fileName)
		if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}

		key := dal.NewKeyWithID("test", fileName)
		exists, err := db.Exists(context.Background(), key)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !exists {
			t.Error("expected to exist")
		}

		key2 := dal.NewKeyWithID("test", "non-existent.txt")
		exists, err = db.Exists(context.Background(), key2)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if exists {
			t.Error("expected not to exist")
		}
	})

	t.Run("GetMulti", func(t *testing.T) {
		fileName1 := "multi1.txt"
		fileName2 := "multi2.txt"
		if err := os.WriteFile(filepath.Join(tmpDir, fileName1), []byte("1"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, fileName2), []byte("2"), 0644); err != nil {
			t.Fatal(err)
		}

		records := []dal.Record{
			NewFileRecord(dal.NewKeyWithID("test", fileName1)),
			NewFileRecord(dal.NewKeyWithID("test", fileName2)),
		}
		err := db.GetMulti(context.Background(), records)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		for _, r := range records {
			if !r.Exists() {
				t.Errorf("expected record %s to exist", r.Key().ID)
			}
		}

		// Test with one missing
		records = append(records, NewFileRecord(dal.NewKeyWithID("test", "missing.txt")))
		err = db.GetMulti(context.Background(), records)
		if err == nil {
			t.Error("expected error for missing file in GetMulti")
		}
	})

	t.Run("Adapter", func(t *testing.T) {
		if adapter := db.Adapter(); adapter != nil {
			t.Error("expected nil adapter")
		}
	})

	t.Run("Schema", func(t *testing.T) {
		if schema := db.Schema(); schema != nil {
			t.Error("expected nil schema")
		}
	})

	t.Run("ExecuteQueryToRecordsReader", func(t *testing.T) {
		reader, err := db.ExecuteQueryToRecordsReader(context.Background(), nil)
		if !errors.Is(err, dal.ErrNotSupported) {
			t.Errorf("expected dal.ErrNotSupported, got %v", err)
		}
		if reader != nil {
			t.Error("expected nil reader")
		}
	})

	t.Run("ExecuteQueryToRecordsetReader", func(t *testing.T) {
		reader, err := db.ExecuteQueryToRecordsetReader(context.Background(), nil, nil)
		if !errors.Is(err, dal.ErrNotSupported) {
			t.Errorf("expected dal.ErrNotSupported, got %v", err)
		}
		if reader != nil {
			t.Error("expected nil reader")
		}
	})

	t.Run("Exists_Error", func(t *testing.T) {
		key := dal.NewKeyWithID("test", "a/b/c\000") // Null character in path should cause error on some OSes
		_, err := db.Exists(context.Background(), key)
		if err == nil {
			t.Log("Warning: could not trigger stat error with null character")
		}
	})

	t.Run("NewDB_HomeDirError", func(t *testing.T) {
		// Mocking os.UserHomeDir is not possible directly, but we can set HOME to something invalid or unset it?
		// On many systems unsetting HOME makes UserHomeDir return error.
		home := os.Getenv("HOME")
		_ = os.Unsetenv("HOME")
		defer func() {
			_ = os.Setenv("HOME", home)
		}()

		_, err = NewDB("~")
		if err == nil {
			t.Log("Warning: could not trigger NewDB home dir error")
		}
	})
}
