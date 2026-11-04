package dal

import (
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
)

// Reader is a positioned result that exposes a cursor and can be closed.
type Reader interface {
	// Cursor points to a position in the result set. This can be used for pagination.
	Cursor() (string, error)

	// Close closes the reader
	Close() error
}

// RecordsReader reads records one by one into record.Record
type RecordsReader interface {
	Reader
	// Next returns the next record for a query.
	// If no more records, a nil record and ErrNoMoreRecords are returned.
	Next() (record.Record, error)
}

// RecordsetReader reads records one by one into recordset.Recordset
type RecordsetReader interface {
	Reader
	// Recordset returns the recordset being read.
	Recordset() recordset.Recordset
	// Next returns the next row and the recordset.
	Next() (row recordset.Row, rs recordset.Recordset, err error)
}
