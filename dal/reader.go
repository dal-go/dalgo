package dal

import "github.com/dal-go/dalgo/recordset"

type Reader interface {
	// Cursor points to a position in the result set. This can be used for pagination.
	Cursor() (string, error)

	// Close closes the reader
	Close() error
}

// RecordsReader reads records one by one into Record
type RecordsReader interface {
	Reader
	// Next returns the next record for a query.
	// If no more records, a nil record and ErrNoMoreRecords are returned.
	Next() (Record, error)
}

// RecordsetReader reads records one by one into *recordset.Recordset
type RecordsetReader interface {
	Reader
	Next() (row recordset.Row, rs *recordset.Recordset, err error)
}
