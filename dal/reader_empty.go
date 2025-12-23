package dal

var _ RecordsReader = (*EmptyReader)(nil)

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
