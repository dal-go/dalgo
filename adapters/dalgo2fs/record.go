package dalgo2fs

import (
	"os"

	"github.com/dal-go/record"
)

func NewFileRecord(key *record.Key) record.Record {
	return &fileRecord{
		key: key,
	}
}

type fileData struct {
	fi os.FileInfo
	//contents []byte
}

type fileRecord struct {
	key     *record.Key
	data    fileData
	changed bool
	err     error
}

func (f *fileRecord) Key() *record.Key {
	return f.key
}

func (f *fileRecord) Error() error {
	return f.err
}

func (f *fileRecord) Exists() bool {
	return f.err == nil && f.data.fi != nil
}

func (f *fileRecord) SetError(err error) record.Record {
	if err == nil {
		err = record.ErrNoError
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
