package dalgo2fs

import (
	"os"

	"github.com/dal-go/dalgo/dal"
)

func NewFileRecord(key *dal.Key) dal.Record {
	return &fileRecord{
		key: key,
	}
}

type fileData struct {
	fi os.FileInfo
	//contents []byte
}

type fileRecord struct {
	key     *dal.Key
	data    fileData
	changed bool
	err     error
}

func (f *fileRecord) Key() *dal.Key {
	return f.key
}

func (f *fileRecord) Error() error {
	return f.err
}

func (f *fileRecord) Exists() bool {
	return f.err == nil && f.data.fi != nil
}

func (f *fileRecord) SetError(err error) dal.Record {
	if err == nil {
		err = dal.ErrNoError
	}
	f.err = err
	return f
}

func (f *fileRecord) Data() any {
	return f.data
}

func (f *fileRecord) setData(data fileData) {
	f.data = data
}

func (f *fileRecord) HasChanged() bool {
	return f.changed
}

func (f *fileRecord) MarkAsChanged() {
	f.changed = true
}
