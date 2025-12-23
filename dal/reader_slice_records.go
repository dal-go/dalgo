package dal

import (
	"errors"
	"strconv"
)

var _ RecordsReader = (*recordsReader)(nil)

func NewRecordsReader(records []Record) RecordsReader {
	return &recordsReader{records: records, current: -1}
}

type recordsReader struct {
	current int
	records []Record
}

func (r *recordsReader) Next() (record Record, err error) {
	if r.records == nil {
		return nil, errors.New("if no records use EmptyReader")
	}
	r.current++
	if r.current >= len(r.records) {
		return nil, ErrNoMoreRecords
	}
	return r.records[r.current], nil
}

var ErrReaderClosed = errors.New("reader closed")
var ErrReaderNotStarted = errors.New("reader not started")

func (r *recordsReader) Cursor() (string, error) {
	if r.current >= len(r.records) {
		return "", ErrReaderClosed
	}
	if r.current < 0 {
		return "", ErrReaderNotStarted
	}
	return strconv.Itoa(r.current), nil
}

func (r *recordsReader) Close() error {
	r.records = nil
	return nil
}
