package dalgo2memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/require"
)

// bothTransactionModes is the two RunReadwriteTransaction implementations
// this package ships: the default whole-database-lock mode and the opt-in
// WithOptimisticConcurrency mode. Firestore parity — and, independently, the
// two different wrong behaviors each mode has today (deadlock vs. silent
// success, see ErrNestedTransaction's doc comment) — must hold for both.
var bothTransactionModes = []struct {
	name string
	opts []Option
}{
	{"default (contending) mode", nil},
	{"single-writer mode", []Option{WithSingleWriterTransactions()}},
	{"WithOptimisticConcurrency", []Option{WithOptimisticConcurrency()}},
}

// runWithTimeout runs f in a goroutine and fails the test if it has not
// returned within the deadline. Every nested-transaction test in this file
// uses it instead of calling the outer RunReadwriteTransaction directly,
// because the whole point of ErrNestedTransaction is to replace a deadlock
// (see TestDefaultTransactionsRemainSerialized for how real the default
// mode's whole-database lock is) with a prompt error — a test that merely
// calls the outer transaction synchronously would hang forever, not fail,
// if that guard ever regressed.
func runWithTimeout(t *testing.T, f func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- f() }()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("transaction did not return: a nested transaction call likely deadlocked")
		return nil // unreachable: t.Fatal stops this goroutine
	}
}

// TestNestedReadwriteTransaction_Rejected is PR 5's headline case: a
// RunReadwriteTransaction call made from inside another RunReadwriteTransaction
// callback must fail with ErrNestedTransaction, in both transaction modes, and
// must do so promptly rather than deadlocking. Before this guard, the default
// locked mode deadlocked here (nested call blocks forever re-acquiring db.mu)
// and the optimistic mode let the nested call run to completion as an
// independent transaction — see ErrNestedTransaction's doc comment.
func TestNestedReadwriteTransaction_Rejected(t *testing.T) {
	for _, mode := range bothTransactionModes {
		t.Run(mode.name, func(t *testing.T) {
			ctx := context.Background()
			db := newDatabase(mode.opts...)
			nestedRan := false

			err := runWithTimeout(t, func() error {
				return db.RunReadwriteTransaction(ctx, func(ctx context.Context, _ dal.ReadwriteTransaction) error {
					return db.RunReadwriteTransaction(ctx, func(context.Context, dal.ReadwriteTransaction) error {
						nestedRan = true
						return nil
					})
				})
			})

			require.ErrorIs(t, err, ErrNestedTransaction)
			require.False(t, nestedRan, "the nested callback must never run: the guard rejects before entering it")
		})
	}
}

// TestNestedReadonlyInReadwriteTransaction_Rejected is the ro-in-rw case the
// PR brief asked to resolve by checking the real Firestore client: Firestore's
// Client.RunTransaction stamps and checks the same transactionInProgressKey
// for BOTH RunReadwriteTransaction and RunReadonlyTransaction (ReadOnly() is
// just an option on the same RunTransaction call), and the check runs before
// either side's read-only flag is even inspected — so a read-only transaction
// started inside a running read-write transaction is rejected exactly like a
// read-write one would be. dalgo2memory mirrors that: RunReadonlyTransaction
// checks the same marker RunReadwriteTransaction sets.
func TestNestedReadonlyInReadwriteTransaction_Rejected(t *testing.T) {
	for _, mode := range bothTransactionModes {
		t.Run(mode.name, func(t *testing.T) {
			ctx := context.Background()
			db := newDatabase(mode.opts...)
			nestedRan := false

			err := runWithTimeout(t, func() error {
				return db.RunReadwriteTransaction(ctx, func(ctx context.Context, _ dal.ReadwriteTransaction) error {
					return db.RunReadonlyTransaction(ctx, func(context.Context, dal.ReadTransaction) error {
						nestedRan = true
						return nil
					})
				})
			})

			require.ErrorIs(t, err, ErrNestedTransaction)
			require.False(t, nestedRan)
		})
	}
}

// TestNestedTransactionInReadonlyTransaction_Rejected completes the symmetry:
// a read-only OUTER transaction also guards against a nested transaction of
// either kind, matching the fact that Firestore's transactionInProgressKey
// check does not care whether the OUTER transaction was read-only either.
func TestNestedTransactionInReadonlyTransaction_Rejected(t *testing.T) {
	ctx := context.Background()
	db := newDatabase()

	t.Run("read-write nested in read-only", func(t *testing.T) {
		nestedRan := false
		err := runWithTimeout(t, func() error {
			return db.RunReadonlyTransaction(ctx, func(ctx context.Context, _ dal.ReadTransaction) error {
				return db.RunReadwriteTransaction(ctx, func(context.Context, dal.ReadwriteTransaction) error {
					nestedRan = true
					return nil
				})
			})
		})
		require.ErrorIs(t, err, ErrNestedTransaction)
		require.False(t, nestedRan)
	})

	t.Run("read-only nested in read-only", func(t *testing.T) {
		nestedRan := false
		err := runWithTimeout(t, func() error {
			return db.RunReadonlyTransaction(ctx, func(ctx context.Context, _ dal.ReadTransaction) error {
				return db.RunReadonlyTransaction(ctx, func(context.Context, dal.ReadTransaction) error {
					nestedRan = true
					return nil
				})
			})
		})
		require.ErrorIs(t, err, ErrNestedTransaction)
		require.False(t, nestedRan)
	})
}

// TestNestedReadwriteTransaction_OuterTreatsAsFatal_DiscardsWrites covers the
// brief's first outer-transaction-unaffected case: a callback that propagates
// the nested-transaction error aborts the outer transaction exactly like any
// other callback error, discarding its own writes.
func TestNestedReadwriteTransaction_OuterTreatsAsFatal_DiscardsWrites(t *testing.T) {
	for _, mode := range bothTransactionModes {
		t.Run(mode.name, func(t *testing.T) {
			ctx := context.Background()
			db := newDatabase(mode.opts...)

			err := runWithTimeout(t, func() error {
				return db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
					if err := tx.Set(ctx, thingRecord("t1", 1)); err != nil {
						return err
					}
					// Treat the nested attempt as fatal: propagate it.
					return db.RunReadwriteTransaction(ctx, func(context.Context, dal.ReadwriteTransaction) error {
						return nil
					})
				})
			})

			require.ErrorIs(t, err, ErrNestedTransaction)
			exists, existsErr := db.Exists(ctx, thingKey("t1"))
			require.NoError(t, existsErr)
			require.False(t, exists, "outer transaction's write must be discarded when its callback treats the nested-transaction error as fatal")
		})
	}
}

// TestNestedReadwriteTransaction_OuterSwallows_StillCommits covers the
// brief's second outer-transaction-unaffected case: this is NOT poisoning.
// ErrNestedTransaction is returned directly by the nested call, before any
// transaction state for that nested attempt is created — unlike
// ErrReadAfterWriteInTransaction, nothing records the rejection on the outer
// transaction's own state (compare transactionState.readAfterWriteRejected).
// This matches the real Firestore client: errNestedTransaction is returned by
// Client.RunTransaction itself, with no effect on the outer *Transaction's
// fields, so a callback that ignores it and returns nil commits normally.
func TestNestedReadwriteTransaction_OuterSwallows_StillCommits(t *testing.T) {
	for _, mode := range bothTransactionModes {
		t.Run(mode.name, func(t *testing.T) {
			ctx := context.Background()
			db := newDatabase(mode.opts...)

			err := runWithTimeout(t, func() error {
				return db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
					if err := tx.Set(ctx, thingRecord("t1", 1)); err != nil {
						return err
					}
					nestedErr := db.RunReadwriteTransaction(ctx, func(context.Context, dal.ReadwriteTransaction) error {
						return nil
					})
					if !errors.Is(nestedErr, ErrNestedTransaction) {
						t.Fatalf("expected ErrNestedTransaction from the nested call, got %v", nestedErr)
					}
					// Swallow it and keep going: per the doc comment above, this must
					// not poison the outer transaction's eventual commit.
					return nil
				})
			})

			require.NoError(t, err, "the outer transaction must still commit: a swallowed ErrNestedTransaction does not poison it")
			exists, existsErr := db.Exists(ctx, thingKey("t1"))
			require.NoError(t, existsErr)
			require.True(t, exists, "outer transaction's own work must still commit when the callback swallows the nested-transaction error")
		})
	}
}

// TestReadwriteTransaction_GetNonTransactionalContextEscapesGuard covers the
// sanctioned escape ErrNestedTransaction's doc comment describes:
// dal.GetNonTransactionalContext(ctx) returns a context from before dalgo
// wrapped it with the transaction, so starting a new transaction with that
// context is not "nested" as far as this guard can see.
//
// This only demonstrates the guard, not a deadlock-free general escape: it
// runs against a WithOptimisticConcurrency database, because the default
// locked mode's runLockedReadwriteTransaction holds db.mu for the whole
// outer callback regardless of what the inner call's ctx carries, so a
// second RunReadwriteTransaction call on the same *database while the outer
// one is still running would deadlock on that pre-existing, unrelated lock —
// see the last paragraph of ErrNestedTransaction's doc comment.
func TestReadwriteTransaction_GetNonTransactionalContextEscapesGuard(t *testing.T) {
	db := newDatabase(WithOptimisticConcurrency())
	// A caller (or an upstream decorator such as access.NewDB) wraps the
	// context with dal.NewContextWithTransaction before handing it to this
	// backend; the transaction value itself is irrelevant to this guard, only
	// the context chain it establishes is, so nil is fine here (dal's own
	// tests use the same nil transaction for this purpose).
	ctx := dal.NewContextWithTransaction(context.Background(), nil)

	independentRan := false
	err := runWithTimeout(t, func() error {
		return db.RunReadwriteTransaction(ctx, func(ctx context.Context, _ dal.ReadwriteTransaction) error {
			independentCtx := dal.GetNonTransactionalContext(ctx)
			return db.RunReadwriteTransaction(independentCtx, func(context.Context, dal.ReadwriteTransaction) error {
				independentRan = true
				return nil
			})
		})
	})

	require.NoError(t, err, "dal.GetNonTransactionalContext(ctx) must escape the nested-transaction guard: it is dal core's sanctioned way to start an independent transaction from inside a callback")
	require.True(t, independentRan, "the independent transaction started via the escape hatch must actually run")
}
