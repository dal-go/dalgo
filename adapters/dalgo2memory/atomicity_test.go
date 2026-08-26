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
