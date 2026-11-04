package mock_dal

import (
	"errors"
	"time"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
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
	records []record.Record
}

// Close returns nil.
func (reader *recordReader) Close() error {
	return nil
}

// Cursor clears the records, marks the reader closed, and returns an empty cursor.
func (reader *recordReader) Cursor() (string, error) {
	reader.records = nil
	reader.i = -1
	return "", nil
}

// Next returns the next record.
// It returns an error when the reader is closed or when no records remain,
// and it sleeps for the reader's delay when that delay is positive.
func (reader *recordReader) Next() (record record.Record, err error) {
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
func NewRecordsReader(delay time.Duration, records ...record.Record) dal.RecordsReader {
	return &recordReader{delay: delay, records: records}
}
