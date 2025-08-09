package dal

import (
	"errors"
	"math"
)

type ReaderOption = func(ro *readerOptions)

type ReaderOptions interface {
	Limit() int
	Offset() int
}

type readerOptions struct {
	limit  int
	offset int
}

func WithLimit(limit int) func(ro *readerOptions) {
	return func(ro *readerOptions) {
		ro.limit = limit
	}
}

func WithOffset(offset int) func(ro *readerOptions) {
	return func(ro *readerOptions) {
		ro.offset = offset
	}
}

func newReaderOptions(options ...ReaderOption) *readerOptions {
	ro := &readerOptions{}
	for _, o := range options {
		o(ro)
	}
	return ro
}

// SelectAll is a helper method that for a given reader returns all items as a slice.
func SelectAll[T any](reader Reader, getItem func(r Record) T, options ...ReaderOption) (items []T, err error) {
	if reader == nil {
		panic("reader is a required parameter, got nil")
	}
	defer func() {
		_ = reader.Close()
	}()
	ro := newReaderOptions(options...)
	limit := ro.limit
	if limit <= 0 {
		items = make([]T, 0)
		limit = math.MaxInt
	} else {
		items = make([]T, 0, limit)
	}
	for ; limit > 0; limit-- {
		var r Record
		if r, err = reader.Next(); err != nil {
			if errors.Is(err, ErrNoMoreRecords) {
				err = nil
			}
			return
		}
		item := getItem(r)
		items = append(items, item)
	}
	return
}

// SelectAllIDs is a helper method that for a given reader returns all IDs as a strongly typed slice.
func SelectAllIDs[T comparable](reader Reader, options ...ReaderOption) (ids []T, err error) {
	return SelectAll[T](reader, func(r Record) T {
		return r.Key().ID.(T)
	}, options...)
}

// SelectAllRecords	is a helper method that for a given reader returns all records as a slice.
func SelectAllRecords(reader Reader, options ...ReaderOption) (records []Record, err error) {
	return SelectAll[Record](reader, func(r Record) Record {
		return r
	}, options...)
}
