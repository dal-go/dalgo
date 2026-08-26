package dalgo2memory

import (
	"context"
	"sync"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests pin the FLIP itself: a plain NewDB() — no options — now behaves
// as the Firestore profile end to end. Every capability asserted here is
// already proven in depth elsewhere (snapshot_test.go, txquery_test.go,
// retry_test.go) against WithOptimisticConcurrency, which is now an affirming
// no-op; what THESE tests prove is that the bare default gets the same
// behavior, so no consumer has to pass an option to test against something
// Firestore-shaped.

// TestDefault_SnapshotReadsAndRetry: on a bare newDatabase(), a fractured
// read aborts the attempt AND the default auto-retry re-runs the callback to
// success — the two halves of the profile working together with no options.
func TestDefault_SnapshotReadsAndRetry(t *testing.T) {
	ctx := context.Background()
	db := newDatabase() // no options: the whole point
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))
	require.NoError(t, db.Set(ctx, thingRecord("B", 1)))

	attempts := 0
	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		attempts++
		var a map[string]any
		if err := tx.Get(ctx, record.NewRecordWithData(thingKey("A"), &a)); err != nil {
			return err
		}
		if attempts == 1 {
			// Fracture the first attempt's snapshot with external commits.
			require.NoError(t, db.Set(ctx, thingRecord("A", 2)))
			require.NoError(t, db.Set(ctx, thingRecord("B", 2)))
		}
		var b map[string]any
		if err := tx.Get(ctx, record.NewRecordWithData(thingKey("B"), &b)); err != nil {
			return err
		}
		assert.Equal(t, a["n"], b["n"], "the callback must never observe a fractured view")
		return nil
	})
	require.NoError(t, err, "the default must auto-retry the aborted first attempt")
	assert.Equal(t, 2, attempts, "attempt 1 aborts on the fractured read; attempt 2 succeeds")
}

// TestDefault_TransactionalQueryWithPhantomProtection: a bare newDatabase()
// supports queries inside read-write transactions AND conflicts on a phantom
// insert — with retry disabled to observe the conflict itself.
func TestDefault_TransactionalQueryWithPhantomProtection(t *testing.T) {
	ctx := context.Background()
	db := newDatabase()
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		n, err := queryThingIDs(ctx, tx)
		if err != nil {
			return err
		}
		assert.Equal(t, 1, n)
		require.NoError(t, db.Set(ctx, thingRecord("B", 1))) // the phantom
		return nil
	}, dal.TxWithAttempts(1))
	assert.True(t, IsTransactionConflict(err),
		"a phantom insert into a queried collection must conflict on the bare default: %v", err)
}

// TestDefault_AtomicityUnchanged: the flip must not disturb what #121
// established — a failed callback discards its writes on the bare default.
func TestDefault_AtomicityUnchanged(t *testing.T) {
	ctx := context.Background()
	db := newDatabase()

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		if err := tx.Set(ctx, thingRecord("X", 1)); err != nil {
			return err
		}
		return errBoom
	})
	require.ErrorIs(t, err, errBoom)
	exists, existsErr := db.Exists(ctx, thingKey("X"))
	require.NoError(t, existsErr)
	assert.False(t, exists, "a failed transaction's writes must be discarded on the bare default")
}

// TestDefault_HotKeyContentionAllComplete pins the escalation progress
// guarantee: N workers doing read-modify-write on ONE hot key must ALL
// eventually complete under the bare default, the way real Firestore's
// server-side lock queue lets contended writers finish rather than abort
// each other forever. Before final-attempt escalation this shape could
// exhaust the optimistic attempt budget and surface a raw conflict that
// production never sees (measured in a fleet sweep: 8 workers on one
// counter document).
func TestDefault_HotKeyContentionAllComplete(t *testing.T) {
	ctx := context.Background()
	db := newDatabase()
	require.NoError(t, db.Set(ctx, thingRecord("counter", 0)))

	const workers = 12
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
				var data map[string]any
				rec := record.NewRecordWithData(thingKey("counter"), &data)
				if err := tx.Get(ctx, rec); err != nil {
					return err
				}
				n, _ := data["n"].(float64)
				return tx.Set(ctx, record.NewRecordWithData(thingKey("counter"), map[string]any{"n": n + 1}))
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err, "every contended transaction must eventually complete, as against real Firestore")
	}

	var final map[string]any
	require.NoError(t, db.Get(ctx, record.NewRecordWithData(thingKey("counter"), &final)))
	assert.EqualValues(t, workers, final["n"],
		"every increment must be applied exactly once: no lost updates, no double-commits")
}

// TestTxWithAttempts1_NeverEscalates pins the other half of the escalation
// contract: a single-attempt transaction is one PURE optimistic shot, so a
// conflict still surfaces — this is what keeps every conflict-observation
// test in this package meaningful.
func TestTxWithAttempts1_NeverEscalates(t *testing.T) {
	ctx := context.Background()
	db := newDatabase()
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		var data map[string]any
		if err := tx.Get(ctx, record.NewRecordWithData(thingKey("A"), &data)); err != nil {
			return err
		}
		require.NoError(t, db.Set(ctx, thingRecord("A", 2))) // external commit
		return tx.Set(ctx, thingRecord("A", 3))
	}, dal.TxWithAttempts(1))
	assert.True(t, IsTransactionConflict(err),
		"a one-attempt transaction must surface the conflict, never escalate past it")
}
