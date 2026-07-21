package dal

import "github.com/dal-go/record"

var _ RecordsReader = (*EmptyReader)(nil)

type EmptyReader struct{}

func (e EmptyReader) Next() (record.Record, error) {
	return nil, ErrNoMoreRecords
}

func (e EmptyReader) Cursor() (string, error) {
	return "", ErrNotSupported
}

func (e EmptyReader) Close() error {
	return nil
}
