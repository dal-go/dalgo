package dalgo2fs

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
)

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
	return &fsFb, nil
}

type database struct {
	path string
	dir  os.FileInfo
}

func (d database) ID() string {
	return "dalgo2fs"
}

func (d database) Adapter() dal.Adapter {
	return nil
}

func (d database) Schema() dal.Schema {
	return nil
}

func (d database) RunReadonlyTransaction(ctx context.Context, f dal.ROTxWorker, _ ...dal.TransactionOption) error {
	tx := transaction{}
	return f(ctx, tx)
}

func (d database) RunReadwriteTransaction(ctx context.Context, f dal.RWTxWorker, _ ...dal.TransactionOption) error {
	tx := transaction{}
	return f(ctx, tx)
}

func (d database) Get(_ context.Context, record dal.Record) error {
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

func (d database) Exists(_ context.Context, key *dal.Key) (bool, error) {
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

func (d database) GetMulti(ctx context.Context, records []dal.Record) error {
	for _, record := range records {
		if err := d.Get(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (d database) GetRecordsReader(_ context.Context, _ dal.Query) (dal.RecordsReader, error) {
	return nil, dal.ErrNotSupported
}

func (d database) GetRecordsetReader(_ context.Context, _ dal.Query, _ recordset.Recordset) (dal.RecordsetReader, error) {
	return nil, dal.ErrNotSupported
}
