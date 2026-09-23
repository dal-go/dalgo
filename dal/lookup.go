package dal

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/dal-go/record"
)

const lookupStreamBatchSize = 512

// ExecuteRecordLookupStream enriches a source reader in bounded batches. Its
// output remains in source order while requests within each batch run in parallel.
func ExecuteRecordLookupStream(ctx context.Context, source RecordsReader, lookup LookupValue, options LookupOptions) (RecordsReader, error) {
	if source == nil || lookup == nil {
		return nil, fmt.Errorf("lookup stream requires a source and lookup function")
	}
	return &recordLookupStream{ctx: ctx, source: source, lookup: lookup, options: options}, nil
}

type recordLookupStream struct {
	ctx               context.Context
	source            RecordsReader
	lookup            LookupValue
	options           LookupOptions
	pending           []record.Record
	loaded, completed int
	done              bool
}

func (r *recordLookupStream) Cursor() (string, error) { return "", nil }
func (r *recordLookupStream) Close() error            { return r.source.Close() }

func (r *recordLookupStream) Next() (record.Record, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	for len(r.pending) == 0 {
		if r.done {
			return nil, ErrNoMoreRecords
		}
		batch := make([]record.Record, 0, lookupStreamBatchSize)
		for len(batch) < lookupStreamBatchSize {
			row, err := r.source.Next()
			if err == ErrNoMoreRecords {
				r.done = true
				break
			}
			if err != nil {
				return nil, err
			}
			batch = append(batch, row)
		}
		if len(batch) == 0 {
			return nil, ErrNoMoreRecords
		}
		r.loaded += len(batch)
		priorCompleted := r.completed
		options := r.options
		options.OnProgress = func(progress LookupProgress) {
			if r.options.OnProgress != nil {
				progress.RowsLoaded = r.loaded
				progress.RequestsCompleted += priorCompleted
				r.options.OnProgress(progress)
			}
		}
		rows, err := ExecuteRecordLookups(r.ctx, batch, r.lookup, options)
		if err != nil {
			return nil, err
		}
		r.completed += len(rows)
		r.pending = rows
	}
	row := r.pending[0]
	r.pending = r.pending[1:]
	return row, nil
}

// LookupProgress gives a live accounting of a per-record lookup queue.
type LookupProgress struct {
	RowsLoaded        int
	RequestsCompleted int
	RequestsInFlight  int
	RequestsPending   int
}

type LookupOptions struct {
	Concurrency int
	OnProgress  func(LookupProgress)
}

// LookupValue returns fields to append to one source record. Callers supply
// the HTTP transport and its authorization policy; DALgo bounds concurrency
// and keeps the result in source order.
type LookupValue func(context.Context, record.Record) (map[string]any, error)

func ExecuteRecordLookups(ctx context.Context, rows []record.Record, lookup LookupValue, options LookupOptions) ([]record.Record, error) {
	if lookup == nil {
		return nil, fmt.Errorf("lookup function is required")
	}
	concurrency := options.Concurrency
	if concurrency == 0 {
		concurrency = 8
	}
	if concurrency < 1 || concurrency > 64 {
		return nil, fmt.Errorf("lookup concurrency must be 1..64")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	out := make([]record.Record, len(rows))
	var next atomic.Int64
	completed, inFlight := 0, 0
	var reportMu sync.Mutex
	report := func(completedDelta, inFlightDelta int) {
		reportMu.Lock()
		defer reportMu.Unlock()
		completed += completedDelta
		inFlight += inFlightDelta
		if options.OnProgress != nil {
			options.OnProgress(LookupProgress{RowsLoaded: len(rows), RequestsCompleted: completed, RequestsInFlight: inFlight, RequestsPending: len(rows) - completed - inFlight})
		}
	}
	report(0, 0)
	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once
	for worker := 0; worker < concurrency && worker < len(rows); worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				index := int(next.Add(1) - 1)
				if index >= len(rows) {
					return
				}
				report(0, 1)
				fields, err := lookup(ctx, rows[index])
				if err != nil {
					errOnce.Do(func() { firstErr = err; cancel() })
					report(0, -1)
					return
				}
				data, ok := rows[index].Data().(map[string]any)
				if !ok {
					errOnce.Do(func() { firstErr = fmt.Errorf("lookup source row %d has non-object data", index); cancel() })
					report(0, -1)
					return
				}
				merged := make(map[string]any, len(data)+len(fields))
				for k, v := range data {
					merged[k] = v
				}
				for k, v := range fields {
					merged[k] = v
				}
				out[index] = record.NewRecordWithData(rows[index].Key(), merged)
				report(1, -1)
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
