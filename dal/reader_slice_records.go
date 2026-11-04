package dal

import (
	"errors"
	"strconv"

	"github.com/dal-go/record"
)

var _ RecordsReader = (*recordsReader)(nil)

// NewRecordsReader returns a records reader positioned before the first of records.
func NewRecordsReader(records []record.Record) RecordsReader {
	return &recordsReader{records: records, current: -1}
}

type recordsReader struct {
	current int
	records []record.Record
}

// Next returns the next record.
// It returns ErrNoMoreRecords after the last record, and an error when records is nil.
func (r *recordsReader) Next() (record record.Record, err error) {
	if r.records == nil {
		return nil, errors.New("if no records use EmptyReader")
	}
	r.current++
	if r.current >= len(r.records) {
		return nil, ErrNoMoreRecords
	}
	return r.records[r.current], nil
}

// ErrReaderClosed is returned by Cursor when the current index is past the last record.
var ErrReaderClosed = errors.New("reader closed")

// ErrReaderNotStarted is returned by Cursor before Next has been called.
var ErrReaderNotStarted = errors.New("reader not started")

// Cursor returns the current record index.
// It returns ErrReaderNotStarted before the first Next and ErrReaderClosed once the index is past the end.
func (r *recordsReader) Cursor() (string, error) {
	if r.current >= len(r.records) {
		return "", ErrReaderClosed
	}
	if r.current < 0 {
		return "", ErrReaderNotStarted
	}
	return strconv.Itoa(r.current), nil
}

// Close clears the records and returns nil.
func (r *recordsReader) Close() error {
	r.records = nil
	return nil
}
