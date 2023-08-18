package dal

import (
	"context"
	"errors"
	"math"
)

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

var _ Reader = (*RecordsReader)(nil)

type RecordsReader struct {
	current int
	records []Record
}

func (r *RecordsReader) Next() (record Record, err error) {
	if r.current >= len(r.records) {
		return nil, ErrNoMoreRecords
	}
	record = r.records[r.current]
	r.current++
	return record, nil
}

func (r *RecordsReader) Cursor() (string, error) {
	return "", ErrNotSupported
}

func (r *RecordsReader) Close() error {
	r.records = nil
	return nil
}
