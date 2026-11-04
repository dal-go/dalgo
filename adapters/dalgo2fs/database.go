package dalgo2fs

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
)

// NewDB opens a filesystem-backed database rooted at an existing directory dirPath.
// A path of "~" or a path starting with "~/" is expanded with the user's home directory.
// The backend is wrapped with dal.NewDB.
func NewDB(dirPath string) (db dal.DB, err error) {
	if strings.HasPrefix(dirPath, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		if dirPath == "~" {
			dirPath = home
		} else if strings.HasPrefix(dirPath, "~/") {
			dirPath = filepath.Join(home, dirPath[2:])
		}
	}
	var fsFb database
	if fsFb.dir, err = os.Stat(dirPath); err != nil {
		return
	}
	fsFb.path = dirPath
	return dal.NewDB(&fsFb), nil
}

var _ dal.Backend = (*database)(nil)

type database struct {
	// dal.NoConcurrency: file-backed store concurrency is unproven;
	// the conservative default is correct until stress-tested.
	dal.NoConcurrency
	path string
	dir  os.FileInfo
}

// ID returns the backend name "dalgo2fs".
func (d database) ID() string {
	return "dalgo2fs"
}

// Adapter returns nil.
func (d database) Adapter() dal.Adapter {
	return nil
}

// Schema returns nil.
func (d database) Schema() dal.Schema {
	return nil
}

// RunReadonlyTransaction calls f with an empty transaction. Options are ignored.
func (d database) RunReadonlyTransaction(ctx context.Context, f dal.ROTxWorker, _ ...dal.TransactionOption) error {
	tx := transaction{}
	return f(ctx, tx)
}

// RunReadwriteTransaction calls f with an empty transaction. Options are ignored.
func (d database) RunReadwriteTransaction(ctx context.Context, f dal.RWTxWorker, _ ...dal.TransactionOption) error {
	tx := transaction{}
	return f(ctx, tx)
}

// Get stats the file named by the record id under the database directory.
// A stat error is returned. A *fileRecord is filled with that file's os.FileInfo; any other record is marked dal.ErrNotSupported.
func (d database) Get(_ context.Context, record record.Record) error {
	name := filepath.Join(d.path, record.Key().ID.(string))
	fi, err := os.Stat(name)
	if err != nil {
		return err
	}
	if fr, ok := record.(*fileRecord); ok {
		fr.setData(fileData{fi: fi})
	} else {
		record.SetError(dal.ErrNotSupported)
	}
	return nil
}

// Exists reports whether the file named by the key id is present under the database directory.
// A missing file is false with a nil error; any other stat error is returned.
func (d database) Exists(_ context.Context, key *record.Key) (bool, error) {
	name := filepath.Join(d.path, key.ID.(string))
	_, err := os.Stat(name)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// GetMulti calls Get for each record and returns the first error.
func (d database) GetMulti(ctx context.Context, records []record.Record) error {
	for _, record := range records {
		if err := d.Get(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteQueryToRecordsReader returns dal.ErrNotSupported.
func (d database) ExecuteQueryToRecordsReader(_ context.Context, _ dal.Query) (dal.RecordsReader, error) {
	return nil, dal.ErrNotSupported
}

// ExecuteQueryToRecordsetReader returns dal.ErrNotSupported.
func (d database) ExecuteQueryToRecordsetReader(_ context.Context, _ dal.Query, _ ...recordset.Option) (dal.RecordsetReader, error) {
	return nil, dal.ErrNotSupported
}
