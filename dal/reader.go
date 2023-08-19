package dal

import (
	"context"
	"errors"
	"math"
	"strconv"
)

// Reader reads records one by one
type Reader interface {

	// Next returns the next record for a query.
	// If no more records a nil record and ErrNoMoreRecords are returned.
	Next() (Record, error)

	// Cursor points to a position in the result set. This can be used for pagination.
	Cursor() (string, error)

	// Close closes the reader
	Close() error
}

// ReadAll reads all records from a reader
func ReadAll(_ context.Context, reader Reader, limit int) (records []Record, err error) {
	var record Record
	if limit <= 0 {
		limit = math.MaxInt64
	}
	for i := 0; i < limit; i++ {
		if record, err = reader.Next(); err != nil {
			if errors.Is(err, ErrNoMoreRecords) {
				err = nil
				break
			}
			return records, err
		}
		records = append(records, record)
	}
	return records, err
}

var _ Reader = (*EmptyReader)(nil)

type EmptyReader struct{}

func (e EmptyReader) Next() (Record, error) {
	return nil, ErrNoMoreRecords
}

func (e EmptyReader) Cursor() (string, error) {
	return "", ErrNotSupported
}

func (e EmptyReader) Close() error {
	return nil
}

var _ Reader = (*recordsReader)(nil)

func NewRecordsReader(records []Record) Reader {
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
