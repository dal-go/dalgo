package dalgo2memory

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests pin the snapshot-read semantics of optimistic mode: every value
// a transaction observes belongs to the single committed state that existed
// at its first observation. They are the regression suite for a measured
// fidelity bug: before this mechanism, optimistic mode was read-committed —
// a transaction could observe A from one committed state and B from a later
// one, a combination that never existed and that real Firestore never shows.
//
// External commits are simulated with top-level (non-transactional) writes on
// the same database: on an optimistic database each one is its own single-key
// commit (see bumpConflictVersion), and being top-level it cannot deadlock
// against the transaction under test.

// readThingN reads "things/<id>" inside the transaction and returns its "n".
func readThingN(ctx context.Context, tx dal.ReadTransaction, id string) (float64, error) {
	var data map[string]any
	rec := record.NewRecordWithData(thingKey(id), &data)
	if err := tx.Get(ctx, rec); err != nil {
		return 0, err
	}
	n, _ := data["n"].(float64)
	return n, nil
}

// TestSnapshotReads_FracturedViewAbortsAtRead is the core regression: after
// the transaction has observed key A, an external commit rewrites A and B;
// reading B must fail with ErrTransactionConflict AT THE READ — never return
// B's new value, which would fracture the view (A=1 and B=2 never coexisted).
// A read of an UNCHANGED key between the two stays fine: the live store is
// still the snapshot for everything the external commit did not touch.
func TestSnapshotReads_FracturedViewAbortsAtRead(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))
	require.NoError(t, db.Set(ctx, thingRecord("B", 1)))
	require.NoError(t, db.Set(ctx, thingRecord("C", 1)))

	var readErr error
	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		a, err := readThingN(ctx, tx, "A") // pins the snapshot
		require.NoError(t, err)
		require.EqualValues(t, 1, a)

		c, err := readThingN(ctx, tx, "C") // unchanged key: still consistent
		require.NoError(t, err)
		require.EqualValues(t, 1, c)

		// External commit rewrites BOTH keys after the snapshot was pinned.
		require.NoError(t, db.Set(ctx, thingRecord("A", 2)))
		require.NoError(t, db.Set(ctx, thingRecord("B", 2)))

		_, readErr = readThingN(ctx, tx, "B")
		return readErr
	})

	require.Error(t, readErr, "reading B after A's snapshot was overwritten must fail at the read")
	assert.True(t, IsTransactionConflict(readErr), "the read error must be retryable: %v", readErr)
	assert.True(t, IsTransactionConflict(err))
}

// TestSnapshotReads_SwallowedConflictPoisonsCommit: a callback that swallows
// the snapshot-conflict read error and returns nil must still fail to commit,
// and none of its buffered writes may become visible — matching the real
// Firestore client, which fails the commit of a transaction whose read came
// back ABORTED regardless of what the callback returned. A later read in the
// same transaction (of any key) also keeps failing: the transaction is dead.
func TestSnapshotReads_SwallowedConflictPoisonsCommit(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))
	require.NoError(t, db.Set(ctx, thingRecord("B", 1)))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		if _, err := readThingN(ctx, tx, "A"); err != nil { // pins the snapshot
			return err
		}
		require.NoError(t, db.Set(ctx, thingRecord("B", 2)))

		_, readErr := readThingN(ctx, tx, "B")
		require.True(t, IsTransactionConflict(readErr))
		// The poisoned transaction fails every subsequent observation too
		// (this second read exercises touch's poisoned fast-path). It runs
		// BEFORE the Set below: after a write, the default ordering rule
		// would reject the read as read-after-write before the snapshot
		// machinery is even consulted.
		_, again := readThingN(ctx, tx, "B")
		require.True(t, IsTransactionConflict(again))
		// Swallow the conflicts and keep going, as buggy caller code might.
		if err := tx.Set(ctx, thingRecord("C", 1)); err != nil {
			return err
		}
		return nil
	})

	require.Error(t, err, "a transaction that observed a snapshot conflict must not commit")
	assert.True(t, IsTransactionConflict(err))
	exists, existsErr := db.Exists(ctx, thingKey("C"))
	require.NoError(t, existsErr)
	assert.False(t, exists, "writes of a poisoned transaction must be discarded")
}

// TestSnapshotReads_UnrelatedNewerKeyAlsoAborts pins the read-time semantics
// deliberately: the transaction observed A, an external commit then rewrote
// only B, and the transaction's first read of B still aborts. Serving B's new
// value would be provably safe here ONLY under a sliding revalidation scheme;
// this implementation reads at a fixed sequence instead — the same shape as
// Firestore's fixed read time — trading a spurious abort (absorbed by the
// caller's retry) for an O(1) check and semantics that need one sentence to
// state.
func TestSnapshotReads_UnrelatedNewerKeyAlsoAborts(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))
	require.NoError(t, db.Set(ctx, thingRecord("B", 1)))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		if _, err := readThingN(ctx, tx, "A"); err != nil { // pins the snapshot
			return err
		}
		require.NoError(t, db.Set(ctx, thingRecord("B", 2)))
		_, readErr := readThingN(ctx, tx, "B")
		return readErr
	})
	assert.True(t, IsTransactionConflict(err))
}

// TestSnapshotReads_BlindWriteToNewerKeyCommits: blind writes observe nothing,
// so writing a key that was committed after the snapshot neither aborts at
// the write nor at commit — Firestore's blind writes likewise do not conflict
// on the written key's prior state. The last writer simply wins.
func TestSnapshotReads_BlindWriteToNewerKeyCommits(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))
	require.NoError(t, db.Set(ctx, thingRecord("B", 1)))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		if _, err := readThingN(ctx, tx, "A"); err != nil { // pins the snapshot
			return err
		}
		require.NoError(t, db.Set(ctx, thingRecord("B", 2)))
		return tx.Set(ctx, thingRecord("B", 3)) // blind write to the newer key
	})
	require.NoError(t, err, "a blind write to a key committed after the snapshot must not conflict")

	var got map[string]any
	rec := record.NewRecordWithData(thingKey("B"), &got)
	require.NoError(t, db.Get(ctx, rec))
	assert.EqualValues(t, 3, got["n"])
}

// TestSnapshotReads_WriteWriteRaceStillConflicts: the blind-write exemption
// above must not weaken write-write conflict detection. A key this
// transaction blind-wrote and another transaction then committed over is
// still caught at commit by the baseline validation.
func TestSnapshotReads_WriteWriteRaceStillConflicts(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		if err := tx.Set(ctx, thingRecord("A", 10)); err != nil { // blind: baseline taken here
			return err
		}
		require.NoError(t, db.Set(ctx, thingRecord("A", 2))) // external commit wins the race
		return nil
	})
	assert.True(t, IsTransactionConflict(err),
		"an external commit between a blind write and its commit must still conflict")
}

// TestSnapshotReads_PinsAtFirstReadNotAtStart: the snapshot is pinned at the
// first observation, not when the transaction begins — a commit that lands
// between the two is simply part of the state the transaction reads, exactly
// as Firestore's effective read time is its first read's.
func TestSnapshotReads_PinsAtFirstReadNotAtStart(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		// The transaction exists but has observed nothing yet.
		require.NoError(t, db.Set(ctx, thingRecord("A", 2)))
		a, err := readThingN(ctx, tx, "A")
		if err != nil {
			return err
		}
		assert.EqualValues(t, 2, a, "the first read pins the snapshot, so it sees the latest commit")
		return nil
	})
	require.NoError(t, err)
}

// TestSnapshotReads_RereadStaysOnSnapshot: once a key is observed, re-reading
// it after an external commit returns the SNAPSHOT value (the cached entry),
// never the newer one — and the transaction then fails at commit via baseline
// validation rather than ever showing the torn value.
func TestSnapshotReads_RereadStaysOnSnapshot(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		if _, err := readThingN(ctx, tx, "A"); err != nil {
			return err
		}
		require.NoError(t, db.Set(ctx, thingRecord("A", 2)))
		a, err := readThingN(ctx, tx, "A") // re-read: served from the snapshot
		if err != nil {
			return err
		}
		assert.EqualValues(t, 1, a, "a re-read must stay on the transaction's snapshot")
		return nil
	})
	assert.True(t, IsTransactionConflict(err),
		"the transaction read a key another commit overwrote, so its commit must conflict")
}
