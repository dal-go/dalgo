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

// TestOptimisticConcurrencyRetry_PersistentConflictExhaustsDefaultAttempts
// checks the other side of the bound: a conflict that recurs on every single
// attempt (the external commit here races EVERY attempt's read, not just the
// first) must not retry forever — it exhausts defaultTransactionMaxAttempts
// and returns a final error that still satisfies IsTransactionConflict, so a
// caller that gives up can still tell why.
func TestOptimisticConcurrencyRetry_PersistentConflictExhaustsDefaultAttempts(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithOptimisticConcurrency())
	require.NoError(t, db.Set(ctx, thingRecord("A", 1)))

	var callbackRuns int
	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		callbackRuns++
		if _, err := readThingN(ctx, tx, "A"); err != nil {
			return err
		}
		// Unlike the test above, this external commit runs on EVERY
		// attempt, after that attempt's own read: no attempt can ever
		// succeed.
		require.NoError(t, db.Set(ctx, thingRecord("A", callbackRuns+100)))
		return nil
	})

	require.Equal(t, defaultTransactionMaxAttempts, callbackRuns,
		"the callback must run exactly once per attempt, up to the default budget, no more")
	require.Error(t, err)
	assert.True(t, IsTransactionConflict(err), "the exhausted retry must still report a classifiable conflict: %v", err)
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
		require.NoError(t, db.Set(ctx, thingRecord("A", callbackRuns+100)))
		return nil
	}, dal.TxWithAttempts(3))

	require.Equal(t, 3, callbackRuns, "TxWithAttempts(3) must run exactly 3 attempts, not the default 5 or a single attempt")
	require.Error(t, err)
	assert.True(t, IsTransactionConflict(err))
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
