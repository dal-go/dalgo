package dalgo2memory

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests pin the bounded auto-retry runOptimisticReadwriteTransaction
// applies to ErrTransactionConflict, matching the real Firestore Go client's
// RunTransaction (see that function's doc comment for the exact mapping to
// cloud.google.com/go/firestore.DefaultTransactionMaxAttempts and to
// dalgo2firestore's own Attempts()->MaxAttempts wiring). As in
// snapshot_test.go and txquery_test.go, external commits are simulated with
// top-level (non-transactional) writes on the same database, each its own
// single-key commit that cannot deadlock against the transaction under test.

// TestOptimisticConcurrencyRetry_ConflictThenSucceeds is the headline
// required case: the callback's first attempt is invalidated by a
// concurrent-looking external commit, and its second attempt — driven only
// on the first attempt via the callbackRuns counter, so it is deterministic
// rather than a race — reads the winning value fresh and commits cleanly.
func TestOptimisticConcurrencyRetry_ConflictThenSucceeds(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))

	var callbackRuns int
	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		callbackRuns++
		n, err := readThingN(ctx, tx, "A")
		if err != nil {
			return err
		}
		if callbackRuns == 1 {
			// Only on the first attempt: an external commit lands between
			// this attempt's read and its own commit, invalidating it. A
			// second attempt must not repeat this — it should read the
			// winning value (99) fresh and succeed.
			require.NoError(t, db.Set(ctx, thingRecord("A", 99)))
		}
		return tx.Set(ctx, thingRecord("A", int(n)+1))
	})

	require.NoError(t, err, "the second attempt must succeed once nothing external races it")
	require.Equal(t, 2, callbackRuns, "the callback must run exactly twice: attempt 1 conflicts, attempt 2 succeeds")

	var got map[string]any
	require.NoError(t, db.Get(ctx, record.NewRecordWithData(thingKey("A"), &got)))
	assert.EqualValues(t, 100, got["n"], "attempt 2 must have read the winning external value (99) and added 1 to it")
}

// TestOptimisticConcurrencyRetry_PersistentConflictEscalatesAndCompletes
// checks the other side of the bound, whose contract changed when
// final-attempt escalation landed: a conflict recurring on every optimistic
// attempt no longer exhausts into a raw error — the LAST attempt runs under
// the whole-database lock and cannot conflict, so the transaction completes,
// matching real Firestore's lock queue where contended writers finish rather
// than abort each other forever. The callback runs exactly once per attempt.
//
// The external fracture-commit is guarded to the optimistic attempts only:
// on the escalated attempt this callback must not perform a top-level write
// on the same database, since the escalated attempt holds db.mu for the
// callback's whole duration — the identical (and pre-existing) hazard of the
// single-writer mode, documented at runEscalatedReadwriteTransaction.
func TestOptimisticConcurrencyRetry_PersistentConflictEscalatesAndCompletes(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))

	var callbackRuns int
	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		callbackRuns++
		if _, err := readThingN(ctx, tx, "A"); err != nil {
			return err
		}
		if callbackRuns < defaultTransactionMaxAttempts {
			// Fracture every OPTIMISTIC attempt's snapshot after its read: no
			// optimistic attempt can ever commit.
			require.NoError(t, db.Set(ctx, thingRecord("A", callbackRuns+100)))
		}
		return tx.Set(ctx, thingRecord("A", 999))
	})

	require.NoError(t, err,
		"the escalated final attempt cannot conflict, so a persistently contended transaction completes")
	require.Equal(t, defaultTransactionMaxAttempts, callbackRuns,
		"the callback must run exactly once per attempt: every optimistic attempt, then the escalated one")

	var got map[string]any
	require.NoError(t, db.Get(ctx, record.NewRecordWithData(thingKey("A"), &got)))
	assert.EqualValues(t, 999, got["n"], "the escalated attempt's write must be the one that landed")
}

// TestOptimisticConcurrencyRetry_TxWithAttempts1DisablesRetry checks that
// dal.TxWithAttempts(1) really means exactly one attempt: a conflict that
// would otherwise be retried under the default budget is instead returned
// immediately, after exactly one callback run.
func TestOptimisticConcurrencyRetry_TxWithAttempts1DisablesRetry(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))

	var callbackRuns int
	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		callbackRuns++
		if _, err := readThingN(ctx, tx, "A"); err != nil {
			return err
		}
		require.NoError(t, db.Set(ctx, thingRecord("A", 2)))
		return nil
	}, dal.TxWithAttempts(1))

	require.Equal(t, 1, callbackRuns, "TxWithAttempts(1) must disable retrying entirely")
	require.Error(t, err)
	assert.True(t, IsTransactionConflict(err))
}

// TestOptimisticConcurrencyRetry_TxWithAttemptsHonorsExplicitCount checks
// that an explicit attempt count other than 1 or the default is honored
// exactly: a persistent conflict run under dal.TxWithAttempts(3) exhausts
// precisely 3 attempts, neither more (the default) nor fewer (1).
func TestOptimisticConcurrencyRetry_TxWithAttemptsHonorsExplicitCount(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))

	var callbackRuns int
	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		callbackRuns++
		if _, err := readThingN(ctx, tx, "A"); err != nil {
			return err
		}
		if callbackRuns < 3 {
			// Fracture only the optimistic attempts (1 and 2); the escalated
			// third attempt holds db.mu, where a top-level write would
			// deadlock — see runEscalatedReadwriteTransaction.
			require.NoError(t, db.Set(ctx, thingRecord("A", callbackRuns+100)))
		}
		return nil
	}, dal.TxWithAttempts(3))

	require.Equal(t, 3, callbackRuns,
		"TxWithAttempts(3) must run exactly 3 attempts: 2 optimistic, then the escalated final one — not the default 5, not a single attempt")
	require.NoError(t, err, "the escalated final attempt completes the transaction")
}

// TestOptimisticConcurrencyRetry_ReadAfterWriteNotRetried is the required
// negative case for the retryable/non-retryable line: ErrReadAfterWriteInTransaction
// signals a code bug in the callback (a read that follows the callback's own
// write), not contention with another transaction, so it must never be
// retried — the callback runs exactly once even though the default budget
// would allow up to 5.
func TestOptimisticConcurrencyRetry_ReadAfterWriteNotRetried(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency()) // noReadsAfterWritesInTransaction defaults to true

	var callbackRuns int
	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		callbackRuns++
		if err := tx.Set(ctx, thingRecord("t1", 1)); err != nil {
			return err
		}
		_, existsErr := tx.Exists(ctx, thingKey("t1"))
		return existsErr
	})

	require.ErrorIs(t, err, ErrReadAfterWriteInTransaction)
	require.Equal(t, 1, callbackRuns, "a read-after-write ordering bug must not be retried, even under the default attempt budget")
}

// TestLockedModeIgnoresAttemptsOptionSilently is the required negative case
// for the OTHER transaction mode: a database created WITHOUT
// WithOptimisticConcurrency never produces ErrTransactionConflict (its
// versions/collectionSeq tables stay empty — see
// runLockedReadwriteTransaction's doc comment), so RunReadwriteTransaction's
// default (locked) path must behave exactly as it did before this feature
// existed, regardless of what dal.TransactionOptions.Attempts() the caller
// asks for: an ordinary callback error is returned immediately, with the
// callback having run exactly once.
func TestLockedModeIgnoresAttemptsOptionSilently(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithSingleWriterTransactions()) // single-writer mode: conflicts cannot occur

	var callbackRuns int
	err := db.RunReadwriteTransaction(ctx, func(context.Context, dal.ReadwriteTransaction) error {
		callbackRuns++
		return errBoom
	}, dal.TxWithAttempts(5))

	require.ErrorIs(t, err, errBoom)
	require.Equal(t, 1, callbackRuns, "the locked mode must not retry regardless of the requested attempt count")
}

// TestEscalatedAttempt_FailureModes covers the escalated final attempt's two
// non-commit exits, which are otherwise shadowed by escalation's cannot-
// conflict guarantee: a callback error still rolls the attempt back, and a
// swallowed read-after-write rejection still poisons the commit — the same
// contracts the optimistic and single-writer runners honor.
func TestEscalatedAttempt_FailureModes(t *testing.T) {
	ctx := context.Background()

	t.Run("callback error discards writes", func(t *testing.T) {
		db := newDatabase(WithOptimisticConcurrency())
		require.NoError(t, db.Set(ctx, thingRecord("A", 1)))
		var runs int
		err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			runs++
			if _, err := readThingN(ctx, tx, "A"); err != nil {
				return err
			}
			if runs == 1 {
				require.NoError(t, db.Set(ctx, thingRecord("A", 2))) // fracture attempt 1
			}
			if err := tx.Set(ctx, thingRecord("C", 1)); err != nil {
				return err
			}
			if runs == 2 {
				return errBoom // fail the ESCALATED attempt itself
			}
			return nil
		}, dal.TxWithAttempts(2))
		require.ErrorIs(t, err, errBoom)
		require.Equal(t, 2, runs)
		exists, existsErr := db.Exists(ctx, thingKey("C"))
		require.NoError(t, existsErr)
		assert.False(t, exists, "the escalated attempt's writes must be discarded on callback error")
	})

	t.Run("swallowed read-after-write poisons the escalated commit", func(t *testing.T) {
		db := newDatabase(WithOptimisticConcurrency())
		require.NoError(t, db.Set(ctx, thingRecord("A", 1)))
		var runs int
		err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			runs++
			if _, err := readThingN(ctx, tx, "A"); err != nil {
				return err
			}
			if runs == 1 {
				require.NoError(t, db.Set(ctx, thingRecord("A", 2))) // fracture attempt 1
			}
			if err := tx.Set(ctx, thingRecord("C", 1)); err != nil {
				return err
			}
			if runs == 2 {
				_, rawErr := readThingN(ctx, tx, "A") // read after write: rejected
				require.ErrorIs(t, rawErr, ErrReadAfterWriteInTransaction)
				// Swallow it, as buggy caller code might.
			}
			return nil
		}, dal.TxWithAttempts(2))
		require.ErrorIs(t, err, ErrReadAfterWriteInTransaction,
			"the escalated attempt must refuse to commit after a swallowed ordering rejection")
		require.Equal(t, 2, runs, "a read-after-write rejection is a code bug, never retried")
		exists, existsErr := db.Exists(ctx, thingKey("C"))
		require.NoError(t, existsErr)
		assert.False(t, exists)
	})
}
