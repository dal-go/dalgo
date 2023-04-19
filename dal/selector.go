package dal

import (
	"context"
	"errors"
)

// QueryExecutor is a query executor that returns a reader + related helper methods.
type QueryExecutor interface {

	// QueryReader returns a reader for the given query to read records 1 by 1 sequentially.
	// The Reader.Next() method returns ErrNoMoreRecords when there are no more records.
	QueryReader(c context.Context, query Query) (Reader, error)

	// QueryAllRecords is a helper method that returns all records for the given query.
	// It reads reader created by QueryReader until it returns ErrNoMoreRecords.
	// If you are interested only in IDs, use like:
	//
	//		reader, err := selector.SelectReader(ctx)
	//      // handle err
	//		var ids []int
	//		ids, err = dal.SelectAllIDs[int](reader)
	QueryAllRecords(ctx context.Context, query Query) (records []Record, err error)
}

var _ QueryExecutor = (*selector)(nil)

type selector struct {
	query     Query
	getReader func(c context.Context, query Query) (Reader, error)
}

func (s selector) QueryReader(c context.Context, query Query) (Reader, error) {
	return s.getReader(c, query)
}

// QueryAllRecords is a helper method that for a given reader returns all records as a slice.
func (s selector) QueryAllRecords(c context.Context, query Query) (records []Record, err error) {
	var reader Reader
	if reader, err = s.getReader(c, query); err != nil {
		return
	}
	return SelectAllRecords(reader, query.Limit)
}

type ReaderProvider = func(c context.Context, query Query) (reader Reader, err error)

func NewQueryExecutor(getReader ReaderProvider) QueryExecutor {
	if getReader == nil {
		panic("getReader is a required parameter, got nil")
	}
	return &selector{getReader: getReader}
}

func SelectAllIDs[T comparable](reader Reader, limit int) (ids []T, err error) {
	for i := 0; limit <= 0 || i < limit; i++ {
		var record Record
		if record, err = reader.Next(); err != nil {
			if errors.Is(err, ErrNoMoreRecords) {
				err = nil
			}
			return
		}
		ids = append(ids, record.Key().ID.(T))
	}
	return
}

func SelectAllRecords(reader Reader, limit int) (records []Record, err error) {
	for i := 0; limit <= 0 || i < limit; i++ {
		var record Record
		if record, err = reader.Next(); err != nil {
			if errors.Is(err, ErrNoMoreRecords) {
				err = nil
			}
			return
		}
		records = append(records, record)
	}
	return
}
