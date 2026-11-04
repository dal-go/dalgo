package dalgo2fs

import (
	"os"

	"github.com/dal-go/record"
)

// NewFileRecord returns an empty file record for key.
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

// Key returns the key given to NewFileRecord.
func (f *fileRecord) Key() *record.Key {
	return f.key
}

func (f *fileRecord) Error() error {
	return f.err
}

// Exists reports whether the record holds file info and has no error.
func (f *fileRecord) Exists() bool {
	return f.err == nil && f.data.fi != nil
}

// SetError stores err on the record and returns the same record.
// A nil err is stored as record.ErrNoError.
func (f *fileRecord) SetError(err error) record.Record {
	if err == nil {
		err = record.ErrNoError
	}
	f.err = err
	return f
}

// Data returns the file metadata stored on the record.
func (f *fileRecord) Data() any {
	return f.data
}

func (f *fileRecord) setData(data fileData) {
	f.data = data
}

// HasChanged reports whether MarkAsChanged has been called.
func (f *fileRecord) HasChanged() bool {
	return f.changed
}

// MarkAsChanged records that the record has changed.
func (f *fileRecord) MarkAsChanged() {
	f.changed = true
}
