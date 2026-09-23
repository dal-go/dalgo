package dal

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dal-go/record"
)

func TestExecuteRecordLookupsProgressAndOrder(t *testing.T) {
	rows := make([]record.Record, 100)
	for i := range rows {
		rows[i] = record.NewRecordWithData(record.NewKeyWithID("Invoice", i+1), map[string]any{"id": i + 1, "countryId": i%2 + 1})
	}
	var active, maximum atomic.Int64
	var progress []LookupProgress
	result, err := ExecuteRecordLookups(context.Background(), rows, func(ctx context.Context, row record.Record) (map[string]any, error) {
		current := active.Add(1)
		for old := maximum.Load(); current > old && !maximum.CompareAndSwap(old, current); old = maximum.Load() {
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Millisecond):
		}
		active.Add(-1)
		country := row.Data().(map[string]any)["countryId"].(int)
		return map[string]any{"population": country * 100}, nil
	}, LookupOptions{Concurrency: 4, OnProgress: func(item LookupProgress) { progress = append(progress, item) }})
	if err != nil {
		t.Fatal(err)
	}
	if maximum.Load() != 4 || result[0].Data().(map[string]any)["population"] != 100 || result[99].Data().(map[string]any)["population"] != 200 {
		t.Fatalf("unexpected lookup result or concurrency")
	}
	last := progress[len(progress)-1]
	if last.RowsLoaded != 100 || last.RequestsCompleted != 100 || last.RequestsInFlight != 0 || last.RequestsPending != 0 {
		t.Fatalf("progress: %+v", last)
	}
}

func TestExecuteRecordLookupStreamLargeRecordset(t *testing.T) {
	rows := make([]record.Record, 20_000)
	for i := range rows {
		rows[i] = record.NewRecordWithData(record.NewKeyWithID("Invoice", i+1), map[string]any{"id": i + 1})
	}
	var latest LookupProgress
	stream, err := ExecuteRecordLookupStream(context.Background(), NewRecordsReader(rows), func(_ context.Context, row record.Record) (map[string]any, error) {
		return map[string]any{"population": row.Data().(map[string]any)["id"]}, nil
	}, LookupOptions{Concurrency: 8, OnProgress: func(progress LookupProgress) { latest = progress }})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	for i := range rows {
		row, err := stream.Next()
		if err != nil {
			t.Fatal(err)
		}
		if row.Data().(map[string]any)["population"] != i+1 {
			t.Fatalf("row %d out of order", i)
		}
	}
	if _, err := stream.Next(); err != ErrNoMoreRecords {
		t.Fatalf("end: %v", err)
	}
	if latest.RowsLoaded != len(rows) || latest.RequestsCompleted != len(rows) || latest.RequestsInFlight != 0 || latest.RequestsPending != 0 {
		t.Fatalf("progress: %+v", latest)
	}
}
