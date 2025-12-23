package dal

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/dal-go/dalgo/recordset"
)

// SelectAll reads records from the provided RecordsReader and converts each Record to T using getItem.
// Behavior and caveats:
// - Panics if reader is nil (existing behavior).
// - Respects WithOffset by discarding the first offset records.
// - If WithLimit <= 0, reads until RecordsReader.Next() returns ErrNoMoreRecords.
// - Ensures reader.Close() is called; if Close returns an error and no prior error occurred, that error is returned.
// - Any panic inside getItem will propagate to the caller.
func SelectAll(ctx context.Context, reader RecordsReader, addItem func(r Record), options ...ReaderOption) (err error) {
	if reader == nil {
		panic("reader is a required parameter, got nil")
	}
	// Ensure Close is called and its error is propagated if no earlier error.
	defer func() {
		if closeErr := reader.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close reader: %w", closeErr)
		}
	}()

	ro := newReaderOptions(options...)

	// Skip offset records if requested.
	offset := ro.offset
	for offset > 0 {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
		if _, e := reader.Next(); e != nil {
			if errors.Is(e, ErrNoMoreRecords) {
				// Nothing to read after skipping: return empty slice and nil error.
				return nil
			}
			return e
		}
		offset--
	}

	if ro.limit <= 0 {
		// Unlimited read until no more records.
		for {
			if ctx != nil {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
			}
			r, e := reader.Next()
			if e != nil {
				if errors.Is(e, io.EOF) {
					return nil
				}
				return e
			}
			addItem(r)
		}
	} else {
		for i := 0; i < ro.limit; i++ {
			if ctx != nil {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
			}
			r, e := reader.Next()
			if e != nil {
				if errors.Is(e, ErrNoMoreRecords) {
					return nil
				}
				return e
			}
			addItem(r)
		}
		return nil
	}
}

// SelectAllIDs is a helper method that for a given reader returns all IDs as a strongly typed slice.
// Note: This will panic at runtime if the underlying ID types are not assignable to T.
func SelectAllIDs[T comparable](ctx context.Context, reader RecordsReader, options ...ReaderOption) (ids []T, err error) {
	return ids, SelectAll(
		ctx,
		reader,
		func(r Record) {
			ids = append(ids, r.Key().ID.(T))
		},
		options...,
	)
}

// ReadAllToRecords is a helper method that for a given reader returns all records as a slice.
func ReadAllToRecords(ctx context.Context, reader RecordsReader, options ...ReaderOption) (records []Record, err error) {
	return records, SelectAll(
		ctx,
		reader,
		func(r Record) {
			records = append(records, r)
		},
		options...,
	)
}

func ExecuteQueryAndReadAllToRecords(ctx context.Context, query Query, qe QueryExecutor, options ...ReaderOption) (records []Record, err error) {
	var reader RecordsReader
	if reader, err = qe.GetRecordsReader(ctx, query); err != nil {
		return nil, err
	}
	return ReadAllToRecords(ctx, reader, options...)
}

func ExecuteQueryAndReadAllToRecordset(ctx context.Context, query Query, qe QueryExecutor, options ...ReaderOption) (rs recordset.Recordset, err error) {
	var reader RecordsetReader
	if reader, err = query.GetRecordsetReader(ctx, qe); err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close reader: %w", closeErr)
		}
	}()

	rs = reader.Recordset()
	ro := newReaderOptions(options...)

	// Skip offset records if requested.
	offset := ro.offset
	for offset > 0 {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return rs, ctx.Err()
			default:
			}
		}
		if _, _, e := reader.Next(); e != nil {
			if errors.Is(e, ErrNoMoreRecords) {
				return rs, nil
			}
			return rs, e
		}
		offset--
	}

	if ro.limit <= 0 {
		for {
			if ctx != nil {
				select {
				case <-ctx.Done():
					return rs, ctx.Err()
				default:
				}
			}
			if _, _, e := reader.Next(); e != nil {
				if errors.Is(e, ErrNoMoreRecords) || errors.Is(e, io.EOF) {
					return rs, nil
				}
				return rs, e
			}
		}
	} else {
		rowsCount := 0
		for {
			if ctx != nil {
				select {
				case <-ctx.Done():
					return rs, ctx.Err()
				default:
				}
			}
			if _, _, e := reader.Next(); e != nil {
				if errors.Is(e, ErrNoMoreRecords) || errors.Is(e, io.EOF) {
					return rs, nil
				}
				return rs, e
			}
			rowsCount++
			if rowsCount >= ro.limit {
				return rs, ErrLimitReached
			}
		}
	}
}
