package dalgo2memory

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errBoom is the callback failure these tests use to abort a transaction.
var errBoom = errors.New("boom")

func thingKey(id string) *record.Key {
	return record.NewKeyWithID("things", id)
}

func thingRecord(id string, n int) record.Record {
	return record.NewRecordWithData(thingKey(id), map[string]any{"n": n})
}

// TestTransactionRollback_DiscardsWrites is the regression test for the
// in-memory adapter's atomicity gap: Firestore discards every write of a
// transaction whose callback returns an error, and before this the default
// (whole-database lock) mode applied writes to the storage engines as they
// were made, so a failed transaction left its partial work committed for real.
//
// It runs against BOTH transaction modes, because the optimistic mode already
// behaved correctly and must keep doing so.
func TestTransactionRollback_DiscardsWrites(t *testing.T) {
	for _, mode := range []struct {
		name string
		opts []Option
	}{
		{"default locked mode", nil},
		{"WithOptimisticConcurrency", []Option{WithOptimisticConcurrency()}},
	} {
		t.Run(mode.name, func(t *testing.T) {
			ctx := context.Background()
			db := newDatabase(mode.opts...)

			err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
				if err := tx.Set(ctx, thingRecord("t1", 1)); err != nil {
					return err
				}
				if err := tx.Set(ctx, thingRecord("t2", 2)); err != nil {
					return err
				}
				return errBoom
			})
			require.ErrorIs(t, err, errBoom)

			for _, id := range []string{"t1", "t2"} {
				exists, existsErr := db.Exists(ctx, thingKey(id))
				require.NoError(t, existsErr)
				assert.False(t, exists,
					"record %q written by a FAILED transaction must not survive it", id)
			}
		})
	}
}

// TestTransactionRollback_DiscardsDeletes proves the rollback covers deletes,
// not only stores: a record removed inside a transaction that then fails must
// still be there afterwards.
func TestTransactionRollback_DiscardsDeletes(t *testing.T) {
	for _, mode := range []struct {
		name string
		opts []Option
	}{
		{"default locked mode", nil},
		{"WithOptimisticConcurrency", []Option{WithOptimisticConcurrency()}},
	} {
		t.Run(mode.name, func(t *testing.T) {
			ctx := context.Background()
			db := newDatabase(mode.opts...)
			require.NoError(t, db.Set(ctx, thingRecord("keep", 1)))

			err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
				if err := tx.Delete(ctx, thingKey("keep")); err != nil {
					return err
				}
				return errBoom
			})
			require.ErrorIs(t, err, errBoom)

			exists, existsErr := db.Exists(ctx, thingKey("keep"))
			require.NoError(t, existsErr)
			assert.True(t, exists,
				"a record deleted by a FAILED transaction must still exist afterwards")
		})
	}
}

// TestTransactionCommit_AppliesAllWrites is the other half of atomicity: a
// transaction that returns nil must apply every write it made. Without this,
// "discard on error" could be satisfied by never writing anything at all.
func TestTransactionCommit_AppliesAllWrites(t *testing.T) {
	for _, mode := range []struct {
		name string
		opts []Option
	}{
		{"default locked mode", nil},
		{"WithOptimisticConcurrency", []Option{WithOptimisticConcurrency()}},
	} {
		t.Run(mode.name, func(t *testing.T) {
			ctx := context.Background()
			db := newDatabase(mode.opts...)
			require.NoError(t, db.Set(ctx, thingRecord("gone", 9)))

			err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
				if err := tx.Set(ctx, thingRecord("t1", 1)); err != nil {
					return err
				}
				if err := tx.Insert(ctx, thingRecord("t2", 2)); err != nil {
					return err
				}
				return tx.Delete(ctx, thingKey("gone"))
			})
			require.NoError(t, err)

			for _, id := range []string{"t1", "t2"} {
				exists, existsErr := db.Exists(ctx, thingKey(id))
				require.NoError(t, existsErr)
				assert.True(t, exists, "record %q written by a COMMITTED transaction must survive", id)
			}
			exists, existsErr := db.Exists(ctx, thingKey("gone"))
			require.NoError(t, existsErr)
			assert.False(t, exists, "record deleted by a COMMITTED transaction must be gone")
		})
	}
}

// TestTransactionRollback_UpdateIsDiscarded proves an Update's merged result is
// buffered like any other write rather than mutating stored data in place.
func TestTransactionRollback_UpdateIsDiscarded(t *testing.T) {
	ctx := context.Background()
	db := newDatabase()
	require.NoError(t, db.Set(ctx, thingRecord("t1", 1)))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		if err := tx.Update(ctx, thingKey("t1"), []update.Update{
			update.ByFieldName("n", 42),
		}); err != nil {
			return err
		}
		return errBoom
	})
	require.ErrorIs(t, err, errBoom)

	var got map[string]any
	rec := record.NewRecordWithData(thingKey("t1"), &got)
	require.NoError(t, db.Get(ctx, rec))
	assert.EqualValues(t, 1, got["n"], "an Update in a FAILED transaction must not reach storage")
}

// TestQueryInsideReadwriteTransaction_StillWorks guards the default mode's
// query support, which the buffering change must not regress: a query issued
// before the transaction's first write reads committed storage, which is
// exactly correct, because the default ordering rule means no write can
// precede it.
func TestQueryInsideReadwriteTransaction_StillWorks(t *testing.T) {
	ctx := context.Background()
	db := newDatabase()
	require.NoError(t, db.Set(ctx, thingRecord("seed", 1)))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		q := dal.From(dal.NewRootCollectionRef("things", "")).NewQuery().SelectKeysOnly(reflect.String)
		reader, qErr := tx.ExecuteQueryToRecordsReader(ctx, q)
		if qErr != nil {
			return qErr
		}
		records, readErr := dal.ReadAllToRecords(ctx, reader)
		if readErr != nil {
			return readErr
		}
		assert.Len(t, records, 1, "the query should see the seeded record")
		// Writing afterwards is fine; only reading after a write is not.
		return tx.Set(ctx, thingRecord("added", 2))
	})
	require.NoError(t, err)
}

// TestQueryAfterWrite_RefusedRatherThanStale covers the one configuration in
// which a query can legitimately follow a write —
// WithInterleavedReadsAndWritesInTransaction. A query scans committed storage
// and so cannot see the transaction's buffered writes; it must say so rather
// than quietly return a result that omits them.
func TestQueryAfterWrite_RefusedRatherThanStale(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithInterleavedReadsAndWritesInTransaction())
	require.NoError(t, db.Set(ctx, thingRecord("seed", 1)))

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		if err := tx.Set(ctx, thingRecord("added", 2)); err != nil {
			return err
		}
		q := dal.From(dal.NewRootCollectionRef("things", "")).NewQuery().SelectKeysOnly(reflect.String)
		_, qErr := tx.ExecuteQueryToRecordsReader(ctx, q)
		return qErr
	})
	require.ErrorIs(t, err, dal.ErrNotSupported)
	assert.Contains(t, err.Error(), "cannot see that transaction's own uncommitted writes")
}

// TestInterleavedReadsSeeOwnWrites proves the opt-out still behaves like a
// SQL-style session transaction: with the ordering rule off, a read by key
// after a write returns the written value from the transaction's own pending
// view, even though nothing has reached the shared store yet.
func TestInterleavedReadsSeeOwnWrites(t *testing.T) {
	ctx := context.Background()
	db := newDatabase(WithInterleavedReadsAndWritesInTransaction())

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		if err := tx.Set(ctx, thingRecord("t1", 7)); err != nil {
			return err
		}
		var got map[string]any
		rec := record.NewRecordWithData(thingKey("t1"), &got)
		if err := tx.Get(ctx, rec); err != nil {
			return err
		}
		assert.EqualValues(t, 7, got["n"], "a read after a write must see this transaction's own write")
		return nil
	})
	require.NoError(t, err)
}

// genThing is the record type the top-level generated-insert tests below store.
type genThing struct {
	Name string `json:"name"`
}

// TestTopLevelInsert_WithGenerator covers session.Insert's NON-transactional
// generated-id path.
//
// It exists because of the buffering change: an Insert issued inside a
// read-write transaction now routes through the pending buffer
// (optimisticState.insert), so the transaction-based generator tests in
// generated_insert_test.go no longer reach the immediate engine path at all.
// That path is still live for a top-level call on the backend, so it keeps its
// own coverage here rather than being deleted as newly-unreachable — it is not
// unreachable, only reached from somewhere else now.
func TestTopLevelInsert_WithGenerator(t *testing.T) {
	ctx := context.Background()
	db := newDatabase()

	rec := record.NewRecordWithIncompleteKey("things", reflect.String, &genThing{Name: "generated"})
	require.NoError(t, db.Insert(ctx, rec, dal.WithRandomStringKey(16, 5)))

	id, ok := rec.Key().ID.(string)
	require.True(t, ok, "generator must assign a string id")
	require.NotEmpty(t, id, "generated id must not be empty")

	out := &genThing{}
	require.NoError(t, db.Get(ctx, record.NewRecordWithData(record.NewKeyWithID("things", id), out)))
	assert.Equal(t, "generated", out.Name)
}

// TestTopLevelInsert_GeneratorCollision covers the same non-transactional path's
// collision branch: a deterministic generator that keeps producing an id which
// is already taken must exhaust its attempts rather than overwrite the record
// sitting there.
func TestTopLevelInsert_GeneratorCollision(t *testing.T) {
	ctx := context.Background()
	db := newDatabase()

	// A day-accuracy timestamp generator deterministically yields the same id
	// throughout a single test run, so the second insert collides every attempt.
	gen := dal.WithTimeStampStringID(dal.TimeStampAccuracyDay, 10, 5)

	first := record.NewRecordWithIncompleteKey("days", reflect.String, &genThing{Name: "first"})
	require.NoError(t, db.Insert(ctx, first, gen))
	firstID, ok := first.Key().ID.(string)
	require.True(t, ok)

	second := record.NewRecordWithIncompleteKey("days", reflect.String, &genThing{Name: "second"})
	err := db.Insert(ctx, second, gen)
	require.Error(t, err)
	assert.ErrorIs(t, err, dal.ErrExceedsMaxNumberOfAttempts)

	out := &genThing{}
	require.NoError(t, db.Get(ctx, record.NewRecordWithData(record.NewKeyWithID("days", firstID), out)))
	assert.Equal(t, "first", out.Name, "the colliding insert must not overwrite the record already there")
}

// TestReadAfterWriteStillRejectedByDefault re-asserts that buffering did not
// weaken the v0.67.0 ordering rule.
func TestReadAfterWriteStillRejectedByDefault(t *testing.T) {
	ctx := context.Background()
	db := newDatabase()

	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		if err := tx.Set(ctx, thingRecord("t1", 1)); err != nil {
			return err
		}
		_, existsErr := tx.Exists(ctx, thingKey("t1"))
		return existsErr
	})
	require.ErrorIs(t, err, ErrReadAfterWriteInTransaction)
}

// TestReadAfterWriteRejection_PoisonsCommitEvenIfSwallowed is the regression
// test for the fidelity gap this covers: the real Firestore Go client
// (cloud.google.com/go/firestore, transaction.go's readAfterWrite field)
// records a rejected read-after-write and fails the transaction's commit
// even when the callback swallows the read's error and returns nil. Before
// this fix, dalgo2memory only surfaced the violation through the read call's
// own return value — a callback that ignored it and returned nil got its
// buffered writes committed anyway, which a real Firestore-backed caller
// could never observe.
//
// It runs against BOTH transaction modes: the flag lives on transactionState,
// which both runLockedReadwriteTransaction and runOptimisticReadwriteTransaction
// share, and each has its own commit-refusal check to cover.
func TestReadAfterWriteRejection_PoisonsCommitEvenIfSwallowed(t *testing.T) {
	for _, mode := range []struct {
		name string
		opts []Option
	}{
		{"default locked mode", nil},
		{"WithOptimisticConcurrency", []Option{WithOptimisticConcurrency()}},
	} {
		t.Run(mode.name, func(t *testing.T) {
			ctx := context.Background()
			db := newDatabase(mode.opts...)

			err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
				if err := tx.Set(ctx, thingRecord("t1", 1)); err != nil {
					return err
				}
				// Deliberately swallow the read-after-write error and return nil, as
				// a careless caller might.
				_, _ = tx.Exists(ctx, thingKey("t1"))
				return nil
			})
			require.Error(t, err, "a swallowed read-after-write rejection must still fail the commit")
			assert.ErrorIs(t, err, ErrReadAfterWriteInTransaction)

			exists, existsErr := db.Exists(ctx, thingKey("t1"))
			require.NoError(t, existsErr)
			assert.False(t, exists,
				"a write buffered before a swallowed read-after-write rejection must not be committed")
		})
	}
}
