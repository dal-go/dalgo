package dal

import (
	"context"
	"errors"
)

type Selector interface {
	Select(c context.Context, query Query) (Reader, error)
	SelectAll(ctx context.Context, query Query) ([]Record, error)
	SelectAllIDs(c context.Context, query Query) (ids []any, err error)
	SelectAllStrIDs(ctx context.Context, query Query) ([]string, error)
	SelectAllIntIDs(ctx context.Context, query Query) ([]int, error)
	SelectAllInt64IDs(ctx context.Context, query Query) ([]int64, error)
}

var _ Selector = (*selector)(nil)

type selector struct {
	getReader func(c context.Context, query Query) (Reader, error)
}

func (s selector) Select(c context.Context, query Query) (Reader, error) {
	return s.getReader(c, query)
}

func (s selector) SelectAll(c context.Context, query Query) (records []Record, err error) {
	var reader Reader
	if reader, err = s.getReader(c, query); err != nil {
		return
	}
	for i := 0; i <= query.Limit; i++ {
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

func (s selector) SelectAllIDs(c context.Context, query Query) (ids []any, err error) {
	return ids, s.selectAllIDsWorker(c, query, func(id any) error {
		ids = append(ids, id)
		return nil
	})
}

func (s selector) SelectAllStrIDs(c context.Context, query Query) (ids []string, err error) {
	return ids, s.selectAllIDsWorker(c, query, func(id any) error {
		ids = append(ids, id.(string))
		return nil
	})
}

func (s selector) SelectAllIntIDs(c context.Context, query Query) (ids []int, err error) {
	return ids, s.selectAllIDsWorker(c, query, func(id any) error {
		ids = append(ids, id.(int))
		return nil
	})
}

func (s selector) SelectAllInt64IDs(c context.Context, query Query) (ids []int64, err error) {
	return ids, s.selectAllIDsWorker(c, query, func(id any) error {
		ids = append(ids, id.(int64))
		return nil
	})
}

func NewSelector(getReader func(c context.Context, query Query) (Reader, error)) Selector {
	if getReader == nil {
		panic("getReader is a required parameter, got nil")
	}
	return &selector{getReader: getReader}
}

func (s selector) selectAllIDsWorker(c context.Context, query Query, addID func(id any) error) (err error) {
	var reader Reader
	if reader, err = s.getReader(c, query); err != nil {
		return err
	}
	for i := 0; i <= query.Limit; i++ {
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
