package dal

// Reader reads records one by one
type Reader interface {

	// Next returns the next record for a query.
	// If no more records, a nil record and ErrNoMoreRecords are returned.
	Next() (Record, error)

	// Cursor points to a position in the result set. This can be used for pagination.
	Cursor() (string, error)

	// Close closes the reader
	Close() error
}
