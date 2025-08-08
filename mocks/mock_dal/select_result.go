package mock_dal

import (
	"errors"
	"github.com/dal-go/dalgo/dal"
	"time"
)

// SelectResult is a helper class that can be used in test definitions (TT)
type SelectResult struct {
	Reader dal.Reader
	Err    error
}

// NewSelectResult creates new SelectResult
func NewSelectResult(reader dal.Reader, err error) SelectResult {
	return SelectResult{Reader: reader, Err: err}
}

var _ dal.Reader = (*recordReader)(nil)

type recordReader struct {
	i       int
	delay   time.Duration
	records []dal.Record
}

func (reader *recordReader) Close() error {
	return nil
}

func (reader *recordReader) Cursor() (string, error) {
	reader.records = nil
	reader.i = -1
	return "", nil
}

func (reader *recordReader) Next() (record dal.Record, err error) {
	if reader.i == -1 {
		return nil, errors.New("reader is closed")
	}
	if reader.i >= len(reader.records) {
		return nil, dal.ErrNoMoreRecords
	}
	if reader.delay > 0 {
		time.Sleep(reader.delay)
	}
	record = reader.records[reader.i]
	reader.i++
	return record, nil
}

// NewRecordsReader creates a reader that returns given records
func NewRecordsReader(delay time.Duration, records ...dal.Record) dal.Reader {
	return &recordReader{delay: delay, records: records}
}
