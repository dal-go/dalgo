package dal

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// SelectAll reads records from the provided Reader and converts each Record to T using getItem.
// Behavior and caveats:
// - Panics if reader is nil (existing behavior).
// - Respects WithOffset by discarding the first offset records.
// - If WithLimit <= 0, reads until Reader.Next() returns ErrNoMoreRecords.
// - Ensures reader.Close() is called; if Close returns an error and no prior error occurred, that error is returned.
// - Any panic inside getItem will propagate to the caller.
func SelectAll[T any](ctx context.Context, reader Reader, getItem func(r Record) T, options ...ReaderOption) (items []T, err error) {
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
				return items, ctx.Err()
			default:
			}
		}
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
			if ctx != nil {
				select {
				case <-ctx.Done():
					return items, ctx.Err()
				default:
				}
			}
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
			if ctx != nil {
				select {
				case <-ctx.Done():
					return items, ctx.Err()
				default:
				}
			}
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
func SelectAllIDs[T comparable](ctx context.Context, reader Reader, options ...ReaderOption) (ids []T, err error) {
	return SelectAll[T](ctx, reader, func(r Record) T {
		return r.Key().ID.(T)
	}, options...)
}

// ReadAllToRecords is a helper method that for a given reader returns all records as a slice.
func ReadAllToRecords(ctx context.Context, reader Reader, options ...ReaderOption) (records []Record, err error) {
	return SelectAll[Record](ctx, reader, func(r Record) Record {
		return r
	}, options...)
}
