package dal

import (
	"errors"
	"fmt"
	"io"
)

// ReaderOption configures how SelectAll reads from the Reader (e.g., limit, offset).
type ReaderOption = func(ro *ReaderOptions)

type ReaderOptions struct {
	offset int
	limit  int
}

// Offset specifies how many records to skip, if 0 - no records are skipped
func (ro *ReaderOptions) Offset() int {
	return ro.offset
}

// Limit specifies the maximum number of records to read, if 0 - unlimited
func (ro *ReaderOptions) Limit() int {
	return ro.limit
}

// WithLimit sets the maximum number of items to read.
// If limit <= 0, SelectAll reads until ErrNoMoreRecords.
func WithLimit(limit int) ReaderOption {
	return func(ro *ReaderOptions) {
		ro.limit = limit
	}
}

// WithOffset skips the first N records before collecting results in SelectAll.
// If offset <= 0, no records are skipped.
func WithOffset(offset int) ReaderOption {
	return func(ro *ReaderOptions) {
		ro.offset = offset
	}
}

func newReaderOptions(options ...ReaderOption) *ReaderOptions {
	ro := &ReaderOptions{}
	for _, o := range options {
		o(ro)
	}
	return ro
}

// SelectAll reads records from the provided Reader and converts each Record to T using getItem.
// Behavior and caveats:
// - Panics if reader is nil (existing behavior).
// - Respects WithOffset by discarding the first offset records.
// - If WithLimit <= 0, reads until Reader.Next() returns ErrNoMoreRecords.
// - Ensures reader.Close() is called; if Close returns an error and no prior error occurred, that error is returned.
// - Any panic inside getItem will propagate to the caller.
func SelectAll[T any](reader Reader, getItem func(r Record) T, options ...ReaderOption) (items []T, err error) {
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
		if _, e := reader.Next(); e != nil {
			if errors.Is(e, ErrNoMoreRecords) {
				// Nothing to read after skipping: return empty slice and nil error.
				return items, nil
			}
			return items, e
		}
		offset--
	}

	limit := ro.limit
	if limit > 0 {
		items = make([]T, 0, limit)
	} else {
		// Unlimited: no preallocation to avoid huge capacities
		items = make([]T, 0)
	}

	if limit <= 0 {
		// Unlimited read until no more records.
		for {
			r, e := reader.Next()
			if e != nil {
				if errors.Is(e, io.EOF) {
					return items, nil
				}
				return items, e
			}
			items = append(items, getItem(r))
		}
	} else {
		for i := 0; i < limit; i++ {
			r, e := reader.Next()
			if e != nil {
				if errors.Is(e, ErrNoMoreRecords) {
					return items, nil
				}
				return items, e
			}
			items = append(items, getItem(r))
		}
		return items, nil
	}
}

// SelectAllIDs is a helper method that for a given reader returns all IDs as a strongly typed slice.
// Note: This will panic at runtime if the underlying ID types are not assignable to T.
func SelectAllIDs[T comparable](reader Reader, options ...ReaderOption) (ids []T, err error) {
	return SelectAll[T](reader, func(r Record) T {
		return r.Key().ID.(T)
	}, options...)
}

// ReadAllRecords is a helper method that for a given reader returns all records as a slice.
func ReadAllRecords(reader Reader, options ...ReaderOption) (records []Record, err error) {
	return SelectAll[Record](reader, func(r Record) Record {
		return r
	}, options...)
}
