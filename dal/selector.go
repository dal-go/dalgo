package dal

import (
	"context"
	"errors"
)

// QueryExecutor links Query definition to a specific session (or transaction).
type QueryExecutor interface {

	// ExecuteQuery creates a Selector for the given query.
	ExecuteQuery(c context.Context, query Query, f func(c context.Context, selector Selector) error) (err error)
}

// Selector is a query executor that returns a reader + helper methods.
type Selector interface {

	// SelectReader returns a reader for the given query to read records 1 by 1 sequentially.
	// The Reader.Next() method returns ErrNoMoreRecords when there are no more records.
	SelectReader(c context.Context) (Reader, error)

	// SelectAllRecords is a helper method that returns all records for the given query.
	// It reads reader created by SelectReader until it returns ErrNoMoreRecords.
	SelectAllRecords(ctx context.Context) ([]Record, error)

	// SelectAllIDs returns all IDs for the given query without reading the full record.
	SelectAllIDs(c context.Context) (ids []any, err error)

	// SelectAllStrIDs is a helper method that returns all IDs as strings for the given query.
	SelectAllStrIDs(ctx context.Context) ([]string, error)

	// SelectAllIntIDs is a helper method that returns all IDs as integers for the given query.
	SelectAllIntIDs(ctx context.Context) ([]int, error)

	// SelectAllInt64IDs is a helper method that returns all IDs as int64 for the given query.
	SelectAllInt64IDs(ctx context.Context) ([]int64, error)
}

var _ Selector = (*selector)(nil)

type selector struct {
	query     Query
	getReader func(c context.Context, query Query) (Reader, error)
}

func (s selector) SelectReader(c context.Context) (Reader, error) {
	return s.getReader(c, s.query)
}

func (s selector) SelectAllRecords(c context.Context) (records []Record, err error) {
	var reader Reader
	if reader, err = s.getReader(c, s.query); err != nil {
		return
	}
	for i := 0; s.query.Limit <= 0 || i <= s.query.Limit; i++ {
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

func (s selector) SelectAllIDs(c context.Context) (ids []any, err error) {
	return ids, s.selectAllIDsWorker(c, s.query, func(id any) error {
		ids = append(ids, id)
		return nil
	})
}

func (s selector) SelectAllStrIDs(c context.Context) (ids []string, err error) {
	return ids, s.selectAllIDsWorker(c, s.query, func(id any) error {
		ids = append(ids, id.(string))
		return nil
	})
}

func (s selector) SelectAllIntIDs(c context.Context) (ids []int, err error) {
	return ids, s.selectAllIDsWorker(c, s.query, func(id any) error {
		ids = append(ids, id.(int))
		return nil
	})
}

func (s selector) SelectAllInt64IDs(c context.Context) (ids []int64, err error) {
	return ids, s.selectAllIDsWorker(c, s.query, func(id any) error {
		ids = append(ids, id.(int64))
		return nil
	})
}

func NewSelector(query Query, getReader func(c context.Context, query Query) (Reader, error)) Selector {
	if getReader == nil {
		panic("getReader is a required parameter, got nil")
	}
	return &selector{query: query, getReader: getReader}
}

func (s selector) selectAllIDsWorker(c context.Context, query Query, addID func(id any) error) (err error) {
	var reader Reader
	if reader, err = s.getReader(c, query); err != nil {
		return err
	}
	for i := 0; query.Limit <= 0 || i <= query.Limit; i++ {
		var record Record
		if record, err = reader.Next(); err != nil {
			if errors.Is(err, ErrNoMoreRecords) {
				err = nil
			}
			return err
		}
		if err = addID(record.Key().ID); err != nil {
			return err
		}
	}
	return err
}
