package dal

import "github.com/dal-go/record"

var _ RecordsReader = (*EmptyReader)(nil)

// EmptyReader is a records reader that contains no records.
type EmptyReader struct{}

// Next returns ErrNoMoreRecords.
func (e EmptyReader) Next() (record.Record, error) {
	return nil, ErrNoMoreRecords
}

// Cursor returns ErrNotSupported.
func (e EmptyReader) Cursor() (string, error) {
	return "", ErrNotSupported
}

// Close returns nil.
func (e EmptyReader) Close() error {
	return nil
}
