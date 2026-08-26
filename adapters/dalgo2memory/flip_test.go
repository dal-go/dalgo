package dalgo2memory

import (
	"context"
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
