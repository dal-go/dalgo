package dal

import (
	"context"

	"github.com/dal-go/dalgo/recordset"
)

// QueryExecutor is a query executor that returns a reader and have few helper methods.
type QueryExecutor interface {

	// ExecuteQueryToRecordsReader returns a reader for the given query to read records 1 by 1 sequentially.
	// The RecordsReader.Next() method returns ErrNoMoreRecords when there are no more records.
	ExecuteQueryToRecordsReader(ctx context.Context, query Query) (RecordsReader, error)

	// ExecuteQueryToRecordsetReader returns a RecordsetReader for the given query, allowing sequential read of records into the provided recordset.
	ExecuteQueryToRecordsetReader(ctx context.Context, query Query, options ...recordset.Option) (RecordsetReader, error)
}

var _ QueryExecutor = (DB)(nil)
var _ QueryExecutor = (ReadSession)(nil)
var _ QueryExecutor = (ReadTransaction)(nil)
